package rag

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"go.uber.org/zap"

	"github.com/jxroo/kairos/internal/config"
	"github.com/jxroo/kairos/internal/memory"
)

func setupSearchTest(t *testing.T) (*RAGSearchService, *Indexer) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "search.db")
	store, err := NewStore(dbPath, zap.NewNop())
	if err != nil {
		t.Fatalf("NewStore() error: %v", err)
	}
	t.Cleanup(func() { store.Close() })

	embedder := memory.NewFallbackEmbedder()
	vecIndex, err := memory.NewFallbackIndex("")
	if err != nil {
		t.Fatalf("NewFallbackIndex() error: %v", err)
	}

	blevePath := filepath.Join(t.TempDir(), "bleve_idx")
	bleveIdx, err := OpenOrCreateBleve(blevePath)
	if err != nil {
		t.Fatalf("OpenOrCreateBleve() error: %v", err)
	}
	t.Cleanup(func() { bleveIdx.Close() })

	cfg := &config.RAGConfig{
		Enabled:      true,
		Extensions:   []string{".md", ".txt", ".go"},
		IgnoreDirs:   []string{".git"},
		ChunkSize:    512,
		ChunkOverlap: 64,
		MaxFileSize:  10485760,
	}

	registry := DefaultRegistry()
	chunker := NewChunker(cfg.ChunkSize, cfg.ChunkOverlap)
	progress := NewProgress()
	indexer := NewIndexer(store, embedder, vecIndex, bleveIdx, registry, chunker, progress, cfg, zap.NewNop())
	searchSvc := NewRAGSearchService(store, embedder, vecIndex, bleveIdx, zap.NewNop())

	return searchSvc, indexer
}

func TestSearchVectorOnly(t *testing.T) {
	svc, idx := setupSearchTest(t)
	ctx := context.Background()

	dir := t.TempDir()
	path := filepath.Join(dir, "kubernetes.md")
	if err := os.WriteFile(path, []byte("Kubernetes deployment guide for production clusters"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := idx.IndexFile(ctx, path); err != nil {
		t.Fatalf("IndexFile() error: %v", err)
	}

	results, err := svc.Search(ctx, RAGSearchQuery{
		Query: "kubernetes deployment",
		Limit: 5,
	})
	if err != nil {
		t.Fatalf("Search() error: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected at least 1 result")
	}
	if results[0].Document.Path != path {
		t.Errorf("expected path %q, got %q", path, results[0].Document.Path)
	}
}

func TestSearchKeywordOnly(t *testing.T) {
	svc, idx := setupSearchTest(t)
	ctx := context.Background()

	dir := t.TempDir()
	// Index a document with specific keywords.
	path := filepath.Join(dir, "database.md")
	if err := os.WriteFile(path, []byte("PostgreSQL database optimization and query tuning"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := idx.IndexFile(ctx, path); err != nil {
		t.Fatalf("IndexFile() error: %v", err)
	}

	results, err := svc.Search(ctx, RAGSearchQuery{
		Query: "PostgreSQL optimization",
		Limit: 5,
	})
	if err != nil {
		t.Fatalf("Search() error: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected at least 1 result")
	}
}

func TestSearchFileTypeFilter(t *testing.T) {
	svc, idx := setupSearchTest(t)
	ctx := context.Background()

	dir := t.TempDir()
	mdPath := filepath.Join(dir, "doc.md")
	if err := os.WriteFile(mdPath, []byte("Markdown content about testing"), 0644); err != nil {
		t.Fatal(err)
	}
	txtPath := filepath.Join(dir, "doc.txt")
	if err := os.WriteFile(txtPath, []byte("Text content about testing"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := idx.IndexFile(ctx, mdPath); err != nil {
		t.Fatalf("IndexFile() error: %v", err)
	}
	if err := idx.IndexFile(ctx, txtPath); err != nil {
		t.Fatalf("IndexFile() error: %v", err)
	}

	// Filter to .md only.
	results, err := svc.Search(ctx, RAGSearchQuery{
		Query:     "testing",
		Limit:     10,
		FileTypes: []string{".md"},
	})
	if err != nil {
		t.Fatalf("Search() error: %v", err)
	}

	for _, r := range results {
		if r.Document.Extension != ".md" {
			t.Errorf("expected .md extension, got %q", r.Document.Extension)
		}
	}
}

func TestSearchEmptyCorpus(t *testing.T) {
	svc, _ := setupSearchTest(t)
	ctx := context.Background()

	results, err := svc.Search(ctx, RAGSearchQuery{
		Query: "anything",
		Limit: 5,
	})
	if err != nil {
		t.Fatalf("Search() error: %v", err)
	}
	if results != nil {
		t.Errorf("expected nil results for empty corpus, got %d", len(results))
	}
}

func TestSearchRRFOrdering(t *testing.T) {
	svc, idx := setupSearchTest(t)
	ctx := context.Background()

	dir := t.TempDir()

	// Doc that should match both vector and keyword.
	path1 := filepath.Join(dir, "both.md")
	if err := os.WriteFile(path1, []byte("Docker container orchestration with Kubernetes clusters"), 0644); err != nil {
		t.Fatal(err)
	}

	// Doc that matches only loosely.
	path2 := filepath.Join(dir, "loose.md")
	if err := os.WriteFile(path2, []byte("Unrelated content about cooking recipes and food"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := idx.IndexFile(ctx, path1); err != nil {
		t.Fatalf("IndexFile() error: %v", err)
	}
	if err := idx.IndexFile(ctx, path2); err != nil {
		t.Fatalf("IndexFile() error: %v", err)
	}

	results, err := svc.Search(ctx, RAGSearchQuery{
		Query: "Docker Kubernetes orchestration",
		Limit: 10,
	})
	if err != nil {
		t.Fatalf("Search() error: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected results")
	}

	// First result should be the matching doc.
	if results[0].Document.Path != path1 {
		t.Errorf("expected %q as top result, got %q", path1, results[0].Document.Path)
	}

	// Scores should be descending.
	for i := 1; i < len(results); i++ {
		if results[i].FinalScore > results[i-1].FinalScore {
			t.Errorf("results not in descending score order at index %d", i)
		}
	}
}
