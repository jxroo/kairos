package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"go.uber.org/zap"
)

func fileOpsDef() ToolDefinition {
	return ToolDefinition{
		Name:        "file-ops",
		Description: "File operations: read, list, stat. Path validated against permission rules.",
		InputSchema: map[string]Param{
			"operation": {Type: "string", Description: "Operation: read, list, stat", Required: true, Enum: []string{"read", "list", "stat"}},
			"path":      {Type: "string", Description: "File or directory path", Required: true},
		},
		Permissions: []Permission{{Resource: "filesystem", Allow: true}},
		Builtin:     true,
	}
}

func fileOpsHandler(perms *PermissionChecker, logger *zap.Logger) ToolHandler {
	return func(ctx context.Context, args map[string]any) (*ToolResult, error) {
		op, _ := args["operation"].(string)
		path, _ := args["path"].(string)

		if op == "" || path == "" {
			return &ToolResult{Content: "operation and path are required", IsError: true}, nil
		}

		abs, err := filepath.Abs(path)
		if err != nil {
			return &ToolResult{Content: fmt.Sprintf("invalid path: %v", err), IsError: true}, nil
		}

		if err := perms.Check("file-ops", "filesystem", abs); err != nil {
			return &ToolResult{Content: "permission denied: path not allowed", IsError: true}, nil
		}

		switch op {
		case "read":
			data, err := os.ReadFile(abs)
			if err != nil {
				return &ToolResult{Content: fmt.Sprintf("read error: %v", err), IsError: true}, nil
			}
			content := string(data)
			if len(content) > 100*1024 {
				content = content[:100*1024] + "\n...[truncated]"
			}
			return &ToolResult{Content: content}, nil

		case "list":
			entries, err := os.ReadDir(abs)
			if err != nil {
				return &ToolResult{Content: fmt.Sprintf("list error: %v", err), IsError: true}, nil
			}
			var names []string
			for _, e := range entries {
				name := e.Name()
				if e.IsDir() {
					name += "/"
				}
				names = append(names, name)
			}
			b, _ := json.Marshal(names)
			return &ToolResult{Content: string(b)}, nil

		case "stat":
			info, err := os.Stat(abs)
			if err != nil {
				return &ToolResult{Content: fmt.Sprintf("stat error: %v", err), IsError: true}, nil
			}
			stat := map[string]any{
				"name":     info.Name(),
				"size":     info.Size(),
				"is_dir":   info.IsDir(),
				"mode":     info.Mode().String(),
				"mod_time": info.ModTime().UTC().String(),
			}
			b, _ := json.Marshal(stat)
			return &ToolResult{Content: string(b)}, nil

		default:
			return &ToolResult{Content: fmt.Sprintf("unknown operation: %s", op), IsError: true}, nil
		}
	}
}
