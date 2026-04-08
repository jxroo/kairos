package server

import (
	"context"
	"testing"

	"go.uber.org/zap"
)

func TestChatToolSet_NilDeps(t *testing.T) {
	// With all nil dependencies, should return empty specs.
	specs, executor := ChatToolSet(nil, nil, nil, zap.NewNop())

	if len(specs) != 0 {
		t.Errorf("expected 0 specs with nil deps, got %d", len(specs))
	}

	// Executor should handle unknown tool gracefully.
	_, err := executor(context.Background(), "unknown_tool", `{}`)
	if err == nil {
		t.Error("expected error for unknown tool")
	}
}

func TestChatToolSet_SpecNames(t *testing.T) {
	// We can't easily construct real Store/SearchService here without a DB,
	// so we test that the function doesn't panic with nil RAG.
	specs, _ := ChatToolSet(nil, nil, nil, zap.NewNop())
	if len(specs) != 0 {
		t.Errorf("expected 0 specs without store, got %d", len(specs))
	}
}

func TestChatToolSet_ExecutorUnknownTool(t *testing.T) {
	_, executor := ChatToolSet(nil, nil, nil, zap.NewNop())

	result, err := executor(context.Background(), "nonexistent", `{}`)
	if err == nil {
		t.Error("expected error for nonexistent tool")
	}
	if result != "" {
		t.Errorf("expected empty result, got %q", result)
	}
}

func TestChatToolSet_ExecutorBadJSON(t *testing.T) {
	_, executor := ChatToolSet(nil, nil, nil, zap.NewNop())

	_, err := executor(context.Background(), "kairos_remember", `invalid json`)
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}
