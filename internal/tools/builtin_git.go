package tools

import (
	"context"
	"fmt"
	"os/exec"
	"time"

	"go.uber.org/zap"
)

var allowedGitCommands = map[string]bool{
	"log":    true,
	"status": true,
	"diff":   true,
	"show":   true,
	"branch": true,
	"tag":    true,
}

func gitDef() ToolDefinition {
	return ToolDefinition{
		Name:        "git",
		Description: "Execute safe git commands (log, status, diff, show, branch, tag). Rejects push/reset/force.",
		InputSchema: map[string]Param{
			"repo_path": {Type: "string", Description: "Repository path", Required: true},
			"command":   {Type: "string", Description: "Git subcommand (log, status, diff, show, branch, tag)", Required: true},
			"args":      {Type: "array", Description: "Additional arguments"},
		},
		Permissions: []Permission{{Resource: "shell", Allow: true}},
		Builtin:     true,
	}
}

func gitHandler(perms *PermissionChecker, logger *zap.Logger) ToolHandler {
	return func(ctx context.Context, args map[string]any) (*ToolResult, error) {
		if err := perms.Check("git", "shell", ""); err != nil {
			return &ToolResult{Content: "permission denied: shell access not allowed", IsError: true}, nil
		}

		repoPath, _ := args["repo_path"].(string)
		command, _ := args["command"].(string)

		if repoPath == "" || command == "" {
			return &ToolResult{Content: "repo_path and command are required", IsError: true}, nil
		}

		if !allowedGitCommands[command] {
			return &ToolResult{
				Content: fmt.Sprintf("git command %q is not allowed (only: log, status, diff, show, branch, tag)", command),
				IsError: true,
			}, nil
		}

		cmdArgs := []string{"-C", repoPath, command}
		if rawArgs, ok := args["args"].([]any); ok {
			for _, a := range rawArgs {
				cmdArgs = append(cmdArgs, fmt.Sprint(a))
			}
		}

		timeout := 30 * time.Second
		ctx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()

		cmd := exec.CommandContext(ctx, "git", cmdArgs...)
		out, err := cmd.CombinedOutput()
		if err != nil {
			return &ToolResult{
				Content: fmt.Sprintf("git error: %v\n%s", err, string(out)),
				IsError: true,
			}, nil
		}

		return &ToolResult{Content: string(out)}, nil
	}
}
