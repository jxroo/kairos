package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/dop251/goja"
	"go.uber.org/zap"
)

// SandboxRunner executes JavaScript tool scripts in an isolated goja VM.
// Each invocation gets a fresh VM — no state leaks between runs.
type SandboxRunner struct {
	perms      *PermissionChecker
	timeout    time.Duration
	logger     *zap.Logger
	httpClient *http.Client
}

// isPrivateIP returns true if the IP address is in a private/reserved range.
func isPrivateIP(ip net.IP) bool {
	privateRanges := []string{
		"127.0.0.0/8",
		"10.0.0.0/8",
		"172.16.0.0/12",
		"192.168.0.0/16",
		"169.254.0.0/16",
		"::1/128",
		"fc00::/7",
	}
	for _, cidr := range privateRanges {
		_, network, err := net.ParseCIDR(cidr)
		if err != nil {
			continue
		}
		if network.Contains(ip) {
			return true
		}
	}
	return false
}

// safeSandboxClient creates an HTTP client that blocks requests to private IPs.
func safeSandboxClient() *http.Client {
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(addr)
			if err != nil {
				return nil, fmt.Errorf("invalid address %q: %w", addr, err)
			}
			ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
			if err != nil {
				return nil, fmt.Errorf("resolving %q: %w", host, err)
			}
			for _, ip := range ips {
				if isPrivateIP(ip.IP) {
					return nil, fmt.Errorf("request to private IP %s is blocked", ip.IP)
				}
			}
			return dialer.DialContext(ctx, network, net.JoinHostPort(ips[0].IP.String(), port))
		},
	}
	redirectCount := 0
	return &http.Client{
		Timeout: 30 * time.Second,
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			redirectCount++
			if redirectCount > 5 {
				return fmt.Errorf("too many redirects")
			}
			return nil
		},
	}
}

// NewSandboxRunner creates a SandboxRunner.
func NewSandboxRunner(perms *PermissionChecker, timeout time.Duration, logger *zap.Logger) *SandboxRunner {
	return &SandboxRunner{
		perms:      perms,
		timeout:    timeout,
		logger:     logger,
		httpClient: safeSandboxClient(),
	}
}

// WrapScript creates a ToolHandler that executes the given JS source.
// The script must define `function run(args)` returning a string or object.
func (sr *SandboxRunner) WrapScript(toolName, script string) ToolHandler {
	return func(ctx context.Context, args map[string]any) (*ToolResult, error) {
		timeout := sr.timeout
		if timeout == 0 {
			timeout = 30 * time.Second
		}
		ctx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()

		vm := goja.New()

		// Interrupt on context cancellation.
		go func() {
			<-ctx.Done()
			vm.Interrupt("timeout")
		}()

		// Inject kairos API object.
		kairos := vm.NewObject()
		argsVal, _ := vm.RunString("(" + toJSON(args) + ")")
		_ = kairos.Set("args", argsVal)
		_ = kairos.Set("log", func(call goja.FunctionCall) goja.Value {
			msg := call.Argument(0).String()
			sr.logger.Debug("tool script log", zap.String("tool", toolName), zap.String("msg", msg))
			return goja.Undefined()
		})

		// Permission-gated APIs.
		if sr.perms.Check(toolName, "network", "") == nil {
			_ = kairos.Set("httpGet", sr.httpGetFunc(ctx, vm))
			_ = kairos.Set("httpPost", sr.httpPostFunc(ctx, vm))
			_ = kairos.Set("httpRequest", sr.httpRequestFunc(ctx, vm))
		}
		if sr.perms.Check(toolName, "shell", "") == nil {
			_ = kairos.Set("exec", sr.execFunc(ctx, vm))
		}
		if sr.perms.Check(toolName, "filesystem", "") == nil {
			_ = kairos.Set("readFile", sr.readFileFunc(toolName, vm))
			_ = kairos.Set("writeFile", sr.writeFileFunc(toolName, vm))
			_ = kairos.Set("listDir", sr.listDirFunc(toolName, vm))
			_ = kairos.Set("stat", sr.statFunc(toolName, vm))
		}

		_ = vm.Set("kairos", kairos)

		// Execute script.
		if _, err := vm.RunString(script); err != nil {
			return &ToolResult{Content: fmt.Sprintf("script error: %v", err), IsError: true}, nil
		}

		// Call run(args).
		runFn, ok := goja.AssertFunction(vm.Get("run"))
		if !ok {
			return &ToolResult{Content: "script must define function run(args)", IsError: true}, nil
		}

		result, err := runFn(goja.Undefined(), argsVal)
		if err != nil {
			errMsg := err.Error()
			if strings.Contains(errMsg, "timeout") {
				return nil, fmt.Errorf("script execution timed out")
			}
			return &ToolResult{Content: fmt.Sprintf("script exception: %v", err), IsError: true}, nil
		}

		// Convert result to string.
		if result == nil || goja.IsUndefined(result) || goja.IsNull(result) {
			return &ToolResult{Content: ""}, nil
		}
		exported := result.Export()
		switch v := exported.(type) {
		case string:
			return &ToolResult{Content: v}, nil
		default:
			b, _ := json.Marshal(v)
			return &ToolResult{Content: string(b)}, nil
		}
	}
}

func (sr *SandboxRunner) httpGetFunc(ctx context.Context, vm *goja.Runtime) func(call goja.FunctionCall) goja.Value {
	return func(call goja.FunctionCall) goja.Value {
		url := call.Argument(0).String()
		result, err := sr.doHTTPRequest(ctx, http.MethodGet, url, "", nil)
		if err != nil {
			panic(vm.NewGoError(err))
		}
		return vm.ToValue(result["body"])
	}
}

func (sr *SandboxRunner) httpPostFunc(ctx context.Context, vm *goja.Runtime) func(call goja.FunctionCall) goja.Value {
	return func(call goja.FunctionCall) goja.Value {
		url := call.Argument(0).String()
		bodyStr := call.Argument(1).String()
		result, err := sr.doHTTPRequest(ctx, http.MethodPost, url, bodyStr, map[string]string{
			"Content-Type": "application/json",
		})
		if err != nil {
			panic(vm.NewGoError(err))
		}
		return vm.ToValue(result["body"])
	}
}

func (sr *SandboxRunner) httpRequestFunc(ctx context.Context, vm *goja.Runtime) func(call goja.FunctionCall) goja.Value {
	return func(call goja.FunctionCall) goja.Value {
		method := call.Argument(0).String()
		url := call.Argument(1).String()
		body := ""
		if arg := call.Argument(2); !goja.IsUndefined(arg) && !goja.IsNull(arg) {
			body = arg.String()
		}

		headers := map[string]string{}
		if arg := call.Argument(3); !goja.IsUndefined(arg) && !goja.IsNull(arg) {
			if exported, ok := arg.Export().(map[string]any); ok {
				for key, value := range exported {
					headers[key] = fmt.Sprint(value)
				}
			}
		}

		result, err := sr.doHTTPRequest(ctx, method, url, body, headers)
		if err != nil {
			panic(vm.NewGoError(err))
		}
		return vm.ToValue(result)
	}
}

func (sr *SandboxRunner) execFunc(ctx context.Context, vm *goja.Runtime) func(call goja.FunctionCall) goja.Value {
	return func(call goja.FunctionCall) goja.Value {
		command := call.Argument(0).String()
		var cmdArgs []string
		if arg1 := call.Argument(1); !goja.IsUndefined(arg1) && !goja.IsNull(arg1) {
			if exported, ok := arg1.Export().([]any); ok {
				for _, a := range exported {
					cmdArgs = append(cmdArgs, fmt.Sprint(a))
				}
			}
		}
		cmd := exec.CommandContext(ctx, command, cmdArgs...)
		if arg2 := call.Argument(2); !goja.IsUndefined(arg2) && !goja.IsNull(arg2) {
			if workingDir := arg2.String(); workingDir != "" {
				cmd.Dir = workingDir
			}
		}
		out, err := cmd.CombinedOutput()
		if err != nil {
			panic(vm.NewGoError(fmt.Errorf("%w\n%s", err, string(out))))
		}
		return vm.ToValue(string(out))
	}
}

func (sr *SandboxRunner) readFileFunc(toolName string, vm *goja.Runtime) func(call goja.FunctionCall) goja.Value {
	return func(call goja.FunctionCall) goja.Value {
		path := call.Argument(0).String()
		abs, _ := filepath.Abs(path)
		if err := sr.perms.Check(toolName, "filesystem", abs); err != nil {
			panic(vm.NewGoError(fmt.Errorf("readFile: %w", err)))
		}
		data, err := os.ReadFile(abs)
		if err != nil {
			panic(vm.NewGoError(err))
		}
		return vm.ToValue(string(data))
	}
}

func (sr *SandboxRunner) writeFileFunc(toolName string, vm *goja.Runtime) func(call goja.FunctionCall) goja.Value {
	return func(call goja.FunctionCall) goja.Value {
		path := call.Argument(0).String()
		data := call.Argument(1).String()
		abs, _ := filepath.Abs(path)
		if err := sr.perms.Check(toolName, "filesystem", abs); err != nil {
			panic(vm.NewGoError(fmt.Errorf("writeFile: %w", err)))
		}
		if err := os.WriteFile(abs, []byte(data), 0644); err != nil {
			panic(vm.NewGoError(err))
		}
		return goja.Undefined()
	}
}

func (sr *SandboxRunner) listDirFunc(toolName string, vm *goja.Runtime) func(call goja.FunctionCall) goja.Value {
	return func(call goja.FunctionCall) goja.Value {
		path := call.Argument(0).String()
		abs, _ := filepath.Abs(path)
		if err := sr.perms.Check(toolName, "filesystem", abs); err != nil {
			panic(vm.NewGoError(fmt.Errorf("listDir: %w", err)))
		}

		entries, err := os.ReadDir(abs)
		if err != nil {
			panic(vm.NewGoError(err))
		}

		names := make([]string, 0, len(entries))
		for _, entry := range entries {
			name := entry.Name()
			if entry.IsDir() {
				name += "/"
			}
			names = append(names, name)
		}

		return vm.ToValue(names)
	}
}

func (sr *SandboxRunner) statFunc(toolName string, vm *goja.Runtime) func(call goja.FunctionCall) goja.Value {
	return func(call goja.FunctionCall) goja.Value {
		path := call.Argument(0).String()
		abs, _ := filepath.Abs(path)
		if err := sr.perms.Check(toolName, "filesystem", abs); err != nil {
			panic(vm.NewGoError(fmt.Errorf("stat: %w", err)))
		}

		info, err := os.Stat(abs)
		if err != nil {
			panic(vm.NewGoError(err))
		}

		return vm.ToValue(map[string]any{
			"name":     info.Name(),
			"size":     info.Size(),
			"is_dir":   info.IsDir(),
			"mode":     info.Mode().String(),
			"mod_time": info.ModTime().UTC().String(),
		})
	}
}

func (sr *SandboxRunner) doHTTPRequest(ctx context.Context, method, url, body string, headers map[string]string) (map[string]any, error) {
	req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewBufferString(body))
	if err != nil {
		return nil, err
	}
	for key, value := range headers {
		req.Header.Set(key, value)
	}

	resp, err := sr.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(io.LimitReader(resp.Body, 100*1024))
	if err != nil {
		return nil, err
	}

	outHeaders := make(map[string]string, len(resp.Header))
	for key := range resp.Header {
		outHeaders[key] = resp.Header.Get(key)
	}

	return map[string]any{
		"status":  resp.StatusCode,
		"body":    string(data),
		"headers": outHeaders,
	}, nil
}

func toJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return "{}"
	}
	return string(b)
}
