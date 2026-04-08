package memory

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"

	"go.uber.org/zap"
)

func setupBenchStore(b *testing.B, count int) (*Store, *SearchService) {
	b.Helper()
	dbPath := filepath.Join(b.TempDir(), "bench.db")
	logger := zap.NewNop()
	store, err := NewStore(dbPath, logger)
	if err != nil {
		b.Fatalf("failed to create store: %v", err)
	}
	b.Cleanup(func() { store.Close() })

	embedder := NewFallbackEmbedder()
	index, err := NewFallbackIndex("")
	if err != nil {
		b.Fatalf("failed to create index: %v", err)
	}
	b.Cleanup(func() { index.Close() })

	searchSvc := NewSearchService(store, embedder, index, logger)

	ctx := context.Background()
	for i := 0; i < count; i++ {
		mem, err := store.Create(ctx, CreateMemoryInput{
			Content:    fmt.Sprintf("benchmark memory entry number %d with varied content about topic %d", i, i%50),
			Importance: "normal",
		})
		if err != nil {
			b.Fatalf("failed to seed memory %d: %v", i, err)
		}
		vec, _ := embedder.Embed(ctx, mem.Content)
		_ = index.Add(mem.ID, vec)
	}

	return store, searchSvc
}

func BenchmarkMemorySearch_100(b *testing.B) {
	_, svc := setupBenchStore(b, 100)
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = svc.Search(ctx, SearchQuery{Query: "test query", Limit: 10})
	}
}

func BenchmarkMemorySearch_1000(b *testing.B) {
	_, svc := setupBenchStore(b, 1000)
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = svc.Search(ctx, SearchQuery{Query: "test query", Limit: 10})
	}
}
