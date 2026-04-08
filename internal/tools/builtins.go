package tools

import (
	"context"

	"go.uber.org/zap"
)

const calcScript = `
function run(args) {
  if (typeof args.expression !== "string" || args.expression.trim() === "") {
    throw new Error("expression is required");
  }

  var expr = args.expression.trim();
  if (!/^[0-9+\-*/().^\s]+$/.test(expr)) {
    throw new Error("expression may only contain arithmetic characters");
  }

  expr = expr.replace(/\^/g, "**");
  var value = Function("return (" + expr + ");")();
  if (typeof value !== "number" || !isFinite(value)) {
    throw new Error("result is not a finite number");
  }

  return String(value);
}
`

const shellScript = `
function run(args) {
  if (typeof args.command !== "string" || args.command === "") {
    throw new Error("command is required");
  }

  return kairos.exec(args.command, args.args || [], args.working_dir || "");
}
`

const gitScript = `
function run(args) {
  if (typeof args.repo_path !== "string" || args.repo_path === "") {
    throw new Error("repo_path is required");
  }
  if (typeof args.command !== "string" || args.command === "") {
    throw new Error("command is required");
  }

  var allowed = {log: true, status: true, diff: true, show: true, branch: true, tag: true};
  if (!allowed[args.command]) {
    throw new Error('git command "' + args.command + '" is not allowed (only: log, status, diff, show, branch, tag)');
  }

  var cmdArgs = ["-C", args.repo_path, args.command];
  if (Array.isArray(args.args)) {
    cmdArgs = cmdArgs.concat(args.args);
  }

  return kairos.exec("git", cmdArgs);
}
`

const webFetchScript = `
function run(args) {
  if (typeof args.url !== "string" || args.url === "") {
    throw new Error("url is required");
  }

  var method = "GET";
  if (typeof args.method === "string" && args.method !== "") {
    method = String(args.method).toUpperCase();
  }

  var response = kairos.httpRequest(method, args.url, "", args.headers || {});
  return "HTTP " + response.status + "\n" + response.body;
}
`

const fileOpsScript = `
function run(args) {
  if (typeof args.operation !== "string" || typeof args.path !== "string" || args.path === "") {
    throw new Error("operation and path are required");
  }

  switch (args.operation) {
    case "read":
      return kairos.readFile(args.path);
    case "list":
      return JSON.stringify(kairos.listDir(args.path));
    case "stat":
      return JSON.stringify(kairos.stat(args.path));
    default:
      throw new Error("unknown operation: " + args.operation);
  }
}
`

// RegisterBuiltins registers all built-in tools into the registry.
func RegisterBuiltins(registry *Registry, perms *PermissionChecker, sandbox *SandboxRunner, logger *zap.Logger) error {
	builtins := []struct {
		def    ToolDefinition
		script string
	}{
		{calcDef(), calcScript},
		{shellDef(), shellScript},
		{gitDef(), gitScript},
		{webFetchDef(), webFetchScript},
		{fileOpsDef(), fileOpsScript},
	}

	for _, builtin := range builtins {
		var handler ToolHandler
		if sandbox != nil {
			handler = sandbox.WrapScript(builtin.def.Name, builtin.script)
		} else {
			handler = builtinFallbackHandler(builtin.def.Name, perms, logger)
		}

		if err := registry.Register(builtin.def, handler); err != nil {
			logger.Warn("failed to register builtin", zap.String("tool", builtin.def.Name), zap.Error(err))
		}
	}
	return nil
}

func builtinFallbackHandler(name string, perms *PermissionChecker, logger *zap.Logger) ToolHandler {
	switch name {
	case "calc":
		return calcHandler()
	case "shell":
		return shellHandler(perms, logger)
	case "git":
		return gitHandler(perms, logger)
	case "web-fetch":
		return webFetchHandler(perms, logger)
	case "file-ops":
		return fileOpsHandler(perms, logger)
	default:
		return func(ctx context.Context, args map[string]any) (*ToolResult, error) {
			return &ToolResult{Content: "builtin handler not found", IsError: true}, nil
		}
	}
}
