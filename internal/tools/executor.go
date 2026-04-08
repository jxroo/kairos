package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"go.uber.org/zap"
)

// ErrMissingRequired is returned when a required argument is not provided.
var ErrMissingRequired = fmt.Errorf("missing required argument")

// Executor is the single entry point for all tool execution. It combines
// registry lookup, permission checks, timeout, handler invocation, and audit
// logging.
type Executor struct {
	registry *Registry
	perms    *PermissionChecker
	sandbox  *SandboxRunner
	audit    *AuditLogger
	timeout  time.Duration
	logger   *zap.Logger
}

// NewExecutor creates an Executor.
func NewExecutor(registry *Registry, perms *PermissionChecker, sandbox *SandboxRunner, audit *AuditLogger, timeout time.Duration, logger *zap.Logger) *Executor {
	return &Executor{
		registry: registry,
		perms:    perms,
		sandbox:  sandbox,
		audit:    audit,
		timeout:  timeout,
		logger:   logger,
	}
}

// Registry returns the underlying tool registry.
func (e *Executor) Registry() *Registry { return e.registry }

// Audit returns the underlying audit logger.
func (e *Executor) Audit() *AuditLogger { return e.audit }

// Execute runs the named tool with the given arguments. Every invocation is
// audit-logged (even failures).
func (e *Executor) Execute(ctx context.Context, toolName string, args map[string]any, caller string) (*ToolResult, error) {
	start := time.Now()

	def, handler, err := e.registry.Get(toolName)
	if err != nil {
		e.logAudit(ctx, toolName, args, nil, err, start, caller)
		return nil, err
	}

	// Validate required params.
	for name, param := range def.InputSchema {
		if param.Required {
			if _, ok := args[name]; !ok {
				err := fmt.Errorf("%w: %s", ErrMissingRequired, name)
				e.logAudit(ctx, toolName, args, nil, err, start, caller)
				return nil, err
			}
		}
	}

	// Check permissions declared by the tool.
	for _, perm := range def.Permissions {
		if checkErr := e.perms.Check(toolName, perm.Resource, ""); checkErr != nil {
			result := &ToolResult{Content: fmt.Sprintf("permission denied: %s", perm.Resource), IsError: true}
			e.logAudit(ctx, toolName, args, result, nil, start, caller)
			return result, nil
		}
	}

	// Apply timeout.
	timeout := e.timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	result, err := handler(ctx, args)
	e.logAudit(ctx, toolName, args, result, err, start, caller)
	return result, err
}

func (e *Executor) logAudit(ctx context.Context, toolName string, args map[string]any, result *ToolResult, execErr error, start time.Time, caller string) {
	if e.audit == nil {
		return
	}

	argsJSON, _ := json.Marshal(args)
	entry := AuditEntry{
		ToolName:   toolName,
		Arguments:  string(argsJSON),
		DurationMs: time.Since(start).Milliseconds(),
		Caller:     caller,
	}

	if execErr != nil {
		entry.IsError = true
		entry.Result = execErr.Error()
	} else if result != nil {
		entry.IsError = result.IsError
		entry.Result = result.Content
	}

	if logErr := e.audit.Log(ctx, entry); logErr != nil {
		e.logger.Error("audit log failed", zap.Error(logErr))
	}
}
