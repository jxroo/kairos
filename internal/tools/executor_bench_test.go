package tools

import (
	"context"
	"testing"
	"time"

	"go.uber.org/zap"
)

func BenchmarkToolExecution_Calc(b *testing.B) {
	logger := zap.NewNop()
	registry := NewRegistry()
	perms := NewPermissionChecker(nil, logger)
	sandbox := NewSandboxRunner(perms, 5*time.Second, logger)

	// Register a simple arithmetic tool using goja.
	calcScript := `function run(args) { var a = Number(args.a); var b = Number(args.b); return String(a + b); }`
	def := ToolDefinition{
		Name:        "bench-calc",
		Description: "Benchmark calculator",
		InputSchema: map[string]Param{
			"a": {Type: "number", Required: true},
			"b": {Type: "number", Required: true},
		},
		Builtin: true,
	}
	handler := sandbox.WrapScript("bench-calc", calcScript)
	_ = registry.Register(def, handler)

	executor := NewExecutor(registry, perms, sandbox, nil, 5*time.Second, logger)

	ctx := context.Background()
	args := map[string]any{"a": 2, "b": 3}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = executor.Execute(ctx, "bench-calc", args, "bench")
	}
}
