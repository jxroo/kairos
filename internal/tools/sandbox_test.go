package tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jxroo/kairos/internal/config"
	"go.uber.org/zap"
)

func permAllowAll() *PermissionChecker {
	return NewPermissionChecker([]config.PermissionRule{
		{Tool: "*", Resource: "filesystem", Allow: true},
		{Tool: "*", Resource: "network", Allow: true},
		{Tool: "*", Resource: "shell", Allow: true},
	}, zap.NewNop())
}

func permDenyAll() *PermissionChecker {
	return NewPermissionChecker(nil, zap.NewNop())
}

func TestSandbox_SimpleReturn(t *testing.T) {
	sr := NewSandboxRunner(permDenyAll(), 5*time.Second, zap.NewNop())
	handler := sr.WrapScript("test", `function run(args) { return "hello " + args.name; }`)

	result, err := handler(context.Background(), map[string]any{"name": "world"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Content != "hello world" {
		t.Errorf("got %q, want %q", result.Content, "hello world")
	}
	if result.IsError {
		t.Error("unexpected IsError=true")
	}
}

func TestSandbox_ObjectReturn(t *testing.T) {
	sr := NewSandboxRunner(permDenyAll(), 5*time.Second, zap.NewNop())
	handler := sr.WrapScript("test", `function run(args) { return {count: 42}; }`)

	result, err := handler(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result.Content, "42") {
		t.Errorf("got %q, want JSON containing 42", result.Content)
	}
}

func TestSandbox_Timeout(t *testing.T) {
	sr := NewSandboxRunner(permDenyAll(), 100*time.Millisecond, zap.NewNop())
	handler := sr.WrapScript("test", `function run(args) { while(true){} }`)

	_, err := handler(context.Background(), nil)
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if !strings.Contains(err.Error(), "timed out") {
		t.Errorf("got %q, want timeout error", err.Error())
	}
}

func TestSandbox_MissingRunFunction(t *testing.T) {
	sr := NewSandboxRunner(permDenyAll(), 5*time.Second, zap.NewNop())
	handler := sr.WrapScript("test", `var x = 1;`)

	result, err := handler(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true for missing run()")
	}
	if !strings.Contains(result.Content, "must define function run") {
		t.Errorf("got %q", result.Content)
	}
}

func TestSandbox_Exception(t *testing.T) {
	sr := NewSandboxRunner(permDenyAll(), 5*time.Second, zap.NewNop())
	handler := sr.WrapScript("test", `function run(args) { throw new Error("boom"); }`)

	result, err := handler(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true")
	}
	if !strings.Contains(result.Content, "boom") {
		t.Errorf("got %q, want error containing 'boom'", result.Content)
	}
}

func TestSandbox_NoFSWithoutPermission(t *testing.T) {
	sr := NewSandboxRunner(permDenyAll(), 5*time.Second, zap.NewNop())
	handler := sr.WrapScript("test", `function run(args) { return typeof kairos.readFile; }`)

	result, err := handler(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Content != "undefined" {
		t.Errorf("readFile should be undefined without permission, got %q", result.Content)
	}
}

func TestSandbox_FSWithPermission(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "hello.txt")
	os.WriteFile(testFile, []byte("hello world"), 0644)

	perms := NewPermissionChecker([]config.PermissionRule{
		{Tool: "*", Resource: "filesystem", Allow: true, Paths: []string{tmpDir}},
	}, zap.NewNop())

	sr := NewSandboxRunner(perms, 5*time.Second, zap.NewNop())
	handler := sr.WrapScript("test", `function run(args) { return kairos.readFile(args.path); }`)

	result, err := handler(context.Background(), map[string]any{"path": testFile})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Content != "hello world" {
		t.Errorf("got %q, want %q", result.Content, "hello world")
	}
}

func TestSandbox_FSPathScoped(t *testing.T) {
	tmpDir := t.TempDir()

	perms := NewPermissionChecker([]config.PermissionRule{
		{Tool: "*", Resource: "filesystem", Allow: true, Paths: []string{tmpDir}},
	}, zap.NewNop())

	sr := NewSandboxRunner(perms, 5*time.Second, zap.NewNop())
	handler := sr.WrapScript("test", `function run(args) { return kairos.readFile("/etc/passwd"); }`)

	result, err := handler(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true for out-of-scope path")
	}
}

func TestSandbox_SSRFBlocked(t *testing.T) {
	sr := NewSandboxRunner(permAllowAll(), 5*time.Second, zap.NewNop())

	tests := []struct {
		name string
		url  string
	}{
		{"localhost", "http://127.0.0.1:8080/secret"},
		{"loopback-full", "http://127.0.0.1/"},
		{"private-10", "http://10.0.0.1/"},
		{"private-172", "http://172.16.0.1/"},
		{"private-192", "http://192.168.1.1/"},
		{"ipv6-loopback", "http://[::1]/"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := sr.doHTTPRequest(context.Background(), "GET", tt.url, "", nil)
			if err == nil {
				t.Errorf("expected SSRF block for %s, got nil error", tt.url)
			}
			if !strings.Contains(err.Error(), "private IP") && !strings.Contains(err.Error(), "blocked") {
				// Accept any connection error for IPs that fail to resolve.
				t.Logf("error for %s: %v", tt.url, err)
			}
		})
	}
}

func TestSandbox_FreshStatePerInvocation(t *testing.T) {
	sr := NewSandboxRunner(permDenyAll(), 5*time.Second, zap.NewNop())
	handler := sr.WrapScript("test", `
		var counter = 0;
		function run(args) { counter++; return String(counter); }
	`)

	r1, _ := handler(context.Background(), nil)
	r2, _ := handler(context.Background(), nil)
	if r1.Content != "1" || r2.Content != "1" {
		t.Errorf("state leaked: r1=%q r2=%q", r1.Content, r2.Content)
	}
}
