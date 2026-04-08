package tools

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jxroo/kairos/internal/config"
	"github.com/jxroo/kairos/internal/memory"
	"go.uber.org/zap"

	_ "github.com/mattn/go-sqlite3"
)

func setupExecutor(t *testing.T, rules []config.PermissionRule) (*Executor, *AuditLogger) {
	t.Helper()
	dbPath := t.TempDir() + "/test.db"
	store, err := memory.NewStore(dbPath, zap.NewNop())
	if err != nil {
		t.Fatalf("creating store: %v", err)
	}
	t.Cleanup(func() { store.Close() })

	registry := NewRegistry()
	perms := NewPermissionChecker(rules, zap.NewNop())
	audit := NewAuditLogger(store.DB(), zap.NewNop())
	sandbox := NewSandboxRunner(perms, 5*time.Second, zap.NewNop())
	exec := NewExecutor(registry, perms, sandbox, audit, 5*time.Second, zap.NewNop())
	return exec, audit
}

func TestExecutor_Success(t *testing.T) {
	exec, audit := setupExecutor(t, nil)

	exec.registry.Register(ToolDefinition{
		Name:        "echo",
		InputSchema: map[string]Param{"msg": {Type: "string", Required: true}},
	}, func(ctx context.Context, args map[string]any) (*ToolResult, error) {
		return &ToolResult{Content: args["msg"].(string)}, nil
	})

	result, err := exec.Execute(context.Background(), "echo", map[string]any{"msg": "hi"}, "test")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.Content != "hi" {
		t.Errorf("got %q, want %q", result.Content, "hi")
	}

	entries, _ := audit.List(context.Background(), 10)
	if len(entries) != 1 {
		t.Fatalf("audit entries: %d, want 1", len(entries))
	}
	if entries[0].ToolName != "echo" || entries[0].IsError {
		t.Errorf("unexpected audit entry: %+v", entries[0])
	}
}

func TestExecutor_ToolNotFound(t *testing.T) {
	exec, _ := setupExecutor(t, nil)
	_, err := exec.Execute(context.Background(), "nonexistent", nil, "test")
	if !errors.Is(err, ErrToolNotFound) {
		t.Errorf("got %v, want ErrToolNotFound", err)
	}
}

func TestExecutor_MissingRequired(t *testing.T) {
	exec, _ := setupExecutor(t, nil)
	exec.registry.Register(ToolDefinition{
		Name:        "need-arg",
		InputSchema: map[string]Param{"x": {Type: "string", Required: true}},
	}, func(ctx context.Context, args map[string]any) (*ToolResult, error) {
		return &ToolResult{Content: "ok"}, nil
	})

	_, err := exec.Execute(context.Background(), "need-arg", nil, "test")
	if !errors.Is(err, ErrMissingRequired) {
		t.Errorf("got %v, want ErrMissingRequired", err)
	}
}

func TestExecutor_PermissionDenied(t *testing.T) {
	exec, _ := setupExecutor(t, nil) // no rules = deny all
	exec.registry.Register(ToolDefinition{
		Name:        "restricted",
		Permissions: []Permission{{Resource: "shell", Allow: true}},
	}, func(ctx context.Context, args map[string]any) (*ToolResult, error) {
		return &ToolResult{Content: "ran"}, nil
	})

	result, err := exec.Execute(context.Background(), "restricted", map[string]any{}, "test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError for permission denied")
	}
}

func TestExecutor_Timeout(t *testing.T) {
	exec, _ := setupExecutor(t, nil)
	exec.timeout = 100 * time.Millisecond

	exec.registry.Register(ToolDefinition{Name: "slow"}, func(ctx context.Context, args map[string]any) (*ToolResult, error) {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(5 * time.Second):
			return &ToolResult{Content: "done"}, nil
		}
	})

	_, err := exec.Execute(context.Background(), "slow", map[string]any{}, "test")
	if err == nil {
		t.Error("expected timeout error")
	}
}

func TestExecutor_AuditOnFailure(t *testing.T) {
	exec, audit := setupExecutor(t, nil)
	_, _ = exec.Execute(context.Background(), "nonexistent", nil, "test")

	entries, _ := audit.List(context.Background(), 10)
	if len(entries) != 1 {
		t.Fatalf("audit entries: %d, want 1", len(entries))
	}
	if !entries[0].IsError {
		t.Error("expected audit entry to have IsError=true")
	}
}
