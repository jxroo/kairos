package tools

import (
	"context"
	"fmt"
	"os/exec"
	"time"

	"go.uber.org/zap"
)

func shellDef() ToolDefinition {
	return ToolDefinition{
		Name:        "shell",
		Description: "Execute a shell command with explicit argument list (no shell interpolation)",
		InputSchema: map[string]Param{
			"command":     {Type: "string", Description: "Command to execute", Required: true},
			"args":        {Type: "array", Description: "Command arguments"},
			"working_dir": {Type: "string", Description: "Working directory"},
		},
		Permissions: []Permission{{Resource: "shell", Allow: true}},
		Builtin:     true,
	}
}

func shellHandler(perms *PermissionChecker, logger *zap.Logger) ToolHandler {
	return func(ctx context.Context, args map[string]any) (*ToolResult, error) {
		if err := perms.Check("shell", "shell", ""); err != nil {
			return &ToolResult{Content: "permission denied: shell access not allowed", IsError: true}, nil
		}

		command, _ := args["command"].(string)
		if command == "" {
			return &ToolResult{Content: "command is required", IsError: true}, nil
		}

		var cmdArgs []string
		if rawArgs, ok := args["args"].([]any); ok {
			for _, a := range rawArgs {
				cmdArgs = append(cmdArgs, fmt.Sprint(a))
			}
		}

		timeout := 30 * time.Second
		ctx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()

		cmd := exec.CommandContext(ctx, command, cmdArgs...)
		if dir, ok := args["working_dir"].(string); ok && dir != "" {
			cmd.Dir = dir
		}

		out, err := cmd.CombinedOutput()
		if err != nil {
			return &ToolResult{
				Content: fmt.Sprintf("exit error: %v\n%s", err, string(out)),
				IsError: true,
			}, nil
		}

		return &ToolResult{Content: string(out)}, nil
	}
}
