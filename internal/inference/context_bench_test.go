package inference

import (
	"context"
	"fmt"
	"testing"

	"go.uber.org/zap"
)

func BenchmarkContextAssembly(b *testing.B) {
	memories := make([]MemoryResult, 10)
	for i := range memories {
		memories[i] = MemoryResult{
			Content:   fmt.Sprintf("memory content %d with some additional detail", i),
			Score:     0.9,
			Tags:      []string{"bench", "test"},
			CreatedAt: "2026-03-15",
		}
	}
	ragResults := make([]RAGResult, 5)
	for i := range ragResults {
		ragResults[i] = RAGResult{Content: fmt.Sprintf("document chunk %d about relevant topic", i), Source: fmt.Sprintf("file%d.md", i), Score: 0.8}
	}

	assembler := NewContextAssembler(
		&mockMemorySearcher{results: memories},
		&mockRAGSearcher{results: ragResults},
		zap.NewNop(),
	)

	messages := []Message{
		{Role: "user", Content: "What do you know about the project?"},
	}
	opts := AssembleOpts{
		SystemPrompt: "You are a helpful assistant.",
		MaxTokens:    4096,
	}

	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = assembler.Assemble(ctx, messages, opts)
	}
}
