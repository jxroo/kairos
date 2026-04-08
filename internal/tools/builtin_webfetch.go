package tools

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"go.uber.org/zap"
)

func webFetchDef() ToolDefinition {
	return ToolDefinition{
		Name:        "web-fetch",
		Description: "Fetch a URL and return the response body (truncated to 100KB)",
		InputSchema: map[string]Param{
			"url":     {Type: "string", Description: "URL to fetch", Required: true},
			"method":  {Type: "string", Description: "HTTP method (GET, POST)", Default: "GET"},
			"headers": {Type: "object", Description: "HTTP headers as key-value pairs"},
		},
		Permissions: []Permission{{Resource: "network", Allow: true}},
		Builtin:     true,
	}
}

func webFetchHandler(perms *PermissionChecker, logger *zap.Logger) ToolHandler {
	return func(ctx context.Context, args map[string]any) (*ToolResult, error) {
		if err := perms.Check("web-fetch", "network", ""); err != nil {
			return &ToolResult{Content: "permission denied: network access not allowed", IsError: true}, nil
		}

		url, _ := args["url"].(string)
		if url == "" {
			return &ToolResult{Content: "url is required", IsError: true}, nil
		}

		method := "GET"
		if m, ok := args["method"].(string); ok && m != "" {
			method = strings.ToUpper(m)
		}

		ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()

		req, err := http.NewRequestWithContext(ctx, method, url, nil)
		if err != nil {
			return &ToolResult{Content: fmt.Sprintf("invalid request: %v", err), IsError: true}, nil
		}

		if hdrs, ok := args["headers"].(map[string]any); ok {
			for k, v := range hdrs {
				req.Header.Set(k, fmt.Sprint(v))
			}
		}

		client := &http.Client{Timeout: 30 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			return &ToolResult{Content: fmt.Sprintf("fetch error: %v", err), IsError: true}, nil
		}
		defer resp.Body.Close()

		body, _ := io.ReadAll(io.LimitReader(resp.Body, 100*1024))
		result := fmt.Sprintf("HTTP %d\n%s", resp.StatusCode, string(body))

		return &ToolResult{Content: result}, nil
	}
}
