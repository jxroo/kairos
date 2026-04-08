package rag

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"

	"go.uber.org/zap"

	"github.com/jxroo/kairos/internal/memory"
)

func BenchmarkRAGSearch_Hybrid(b *testing.B) {
	tmpDir := b.TempDir()
	logger := zap.NewNop()

	dbPath := filepath.Join(tmpDir, "bench.db")
	store, err := NewStore(dbPath, logger)
	if err != nil {
		b.Fatalf("failed to create RAG store: %v", err)
	}
	b.Cleanup(func() { store.Close() })

	embedder := memory.NewFallbackEmbedder()
	vecIndex, err := memory.NewFallbackIndex("")
	if err != nil {
		b.Fatalf("failed to create vector index: %v", err)
	}
	b.Cleanup(func() { vecIndex.Close() })

	blevePath := filepath.Join(tmpDir, "bleve_bench")
	bleveIdx, err := OpenOrCreateBleve(blevePath)
	if err != nil {
		b.Fatalf("failed to create bleve: %v", err)
	}
	b.Cleanup(func() { bleveIdx.Close() })

	searchSvc := NewRAGSearchService(store, embedder, vecIndex, bleveIdx, logger)

	ctx := context.Background()
	// Seed test documents and chunks.
	for i := 0; i < 100; i++ {
		doc := &Document{
			Path:      fmt.Sprintf("/docs/file%d.md", i),
			Filename:  fmt.Sprintf("file%d.md", i),
			Extension: ".md",
			SizeBytes: int64(100 + i),
			Status:    StatusIndexed,
		}
		if err := store.CreateDocument(ctx, doc); err != nil {
			b.Fatalf("failed to create doc %d: %v", i, err)
		}

		content := fmt.Sprintf("benchmark document chunk %d about topic %d with varied keywords", i, i%20)
		chunks := []Chunk{{
			DocumentID: doc.ID,
			ChunkIndex: 0,
			Content:    content,
			StartLine:  0,
			EndLine:    len(content),
		}}
		if err := store.CreateChunks(ctx, chunks); err != nil {
			b.Fatalf("failed to create chunk %d: %v", i, err)
		}
		vec, _ := embedder.Embed(ctx, content)
		_ = vecIndex.Add(chunks[0].ID, vec)
		_ = bleveIdx.IndexChunk(chunks[0], doc.Path)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = searchSvc.Search(ctx, RAGSearchQuery{Query: "benchmark topic", Limit: 10})
	}
}
