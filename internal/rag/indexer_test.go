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

func newTestIndexer(t *testing.T) (*Indexer, *Store, memory.VectorIndex) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := NewStore(dbPath, zap.NewNop())
	if err != nil {
		t.Fatalf("NewStore() error: %v", err)
	}
	t.Cleanup(func() { store.Close() })

	embedder := memory.NewFallbackEmbedder()
	index, err := memory.NewFallbackIndex("")
	if err != nil {
		t.Fatalf("NewFallbackIndex() error: %v", err)
	}

	cfg := &config.RAGConfig{
		Enabled:      true,
		Extensions:   []string{".md", ".txt", ".go", ".py", ".pdf"},
		IgnoreDirs:   []string{".git", "node_modules"},
		ChunkSize:    512,
		ChunkOverlap: 64,
		MaxFileSize:  10485760,
	}

	registry := DefaultRegistry()
	chunker := NewChunker(cfg.ChunkSize, cfg.ChunkOverlap)
	progress := NewProgress()

	indexer := NewIndexer(store, embedder, index, nil, registry, chunker, progress, cfg, zap.NewNop())
	return indexer, store, index
}

func TestIndexFile(t *testing.T) {
	idx, store, vecIndex := newTestIndexer(t)
	ctx := context.Background()

	// Create a test file.
	dir := t.TempDir()
	path := filepath.Join(dir, "test.md")
	content := "# Test Document\n\nThis is a test document with some content.\nMore lines here."
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	if err := idx.IndexFile(ctx, path); err != nil {
		t.Fatalf("IndexFile() error: %v", err)
	}

	// Verify document in DB.
	doc, err := store.GetDocumentByPath(ctx, path)
	if err != nil {
		t.Fatalf("GetDocumentByPath() error: %v", err)
	}
	if doc.Status != StatusIndexed {
		t.Errorf("expected status indexed, got %q", doc.Status)
	}

	// Verify chunks exist.
	chunks, err := store.GetChunksByDocumentID(ctx, doc.ID)
	if err != nil {
		t.Fatalf("GetChunksByDocumentID() error: %v", err)
	}
	if len(chunks) == 0 {
		t.Error("expected at least 1 chunk")
	}

	// Verify vectors exist.
	for _, c := range chunks {
		hits, err := vecIndex.Search(make([]float32, 384), 100)
		if err != nil {
			t.Fatalf("vector search error: %v", err)
		}
		found := false
		for _, h := range hits {
			if h.ID == c.ID {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("chunk %q not found in vector index", c.ID)
		}
	}
}

func TestIndexFileSameHash(t *testing.T) {
	idx, store, _ := newTestIndexer(t)
	ctx := context.Background()

	dir := t.TempDir()
	path := filepath.Join(dir, "same.md")
	if err := os.WriteFile(path, []byte("content"), 0644); err != nil {
		t.Fatal(err)
	}

	// Index twice.
	if err := idx.IndexFile(ctx, path); err != nil {
		t.Fatalf("first IndexFile() error: %v", err)
	}
	if err := idx.IndexFile(ctx, path); err != nil {
		t.Fatalf("second IndexFile() error: %v", err)
	}

	// Should still be one document.
	docs, err := store.ListDocuments(ctx)
	if err != nil {
		t.Fatalf("ListDocuments() error: %v", err)
	}
	if len(docs) != 1 {
		t.Errorf("expected 1 document, got %d", len(docs))
	}
}

func TestIndexFileChangedHash(t *testing.T) {
	idx, store, _ := newTestIndexer(t)
	ctx := context.Background()

	dir := t.TempDir()
	path := filepath.Join(dir, "changing.md")
	if err := os.WriteFile(path, []byte("original content"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := idx.IndexFile(ctx, path); err != nil {
		t.Fatalf("first IndexFile() error: %v", err)
	}

	// Change file.
	if err := os.WriteFile(path, []byte("updated content"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := idx.IndexFile(ctx, path); err != nil {
		t.Fatalf("second IndexFile() error: %v", err)
	}

	// Still one document with updated content.
	doc, err := store.GetDocumentByPath(ctx, path)
	if err != nil {
		t.Fatalf("GetDocumentByPath() error: %v", err)
	}
	chunks, err := store.GetChunksByDocumentID(ctx, doc.ID)
	if err != nil {
		t.Fatalf("GetChunksByDocumentID() error: %v", err)
	}
	if len(chunks) == 0 {
		t.Error("expected chunks after re-index")
	}
	if chunks[0].Content != "updated content" {
		t.Errorf("expected updated content, got %q", chunks[0].Content)
	}
}

func TestRemoveFile(t *testing.T) {
	idx, store, vecIndex := newTestIndexer(t)
	ctx := context.Background()

	dir := t.TempDir()
	path := filepath.Join(dir, "remove.md")
	if err := os.WriteFile(path, []byte("to be removed"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := idx.IndexFile(ctx, path); err != nil {
		t.Fatalf("IndexFile() error: %v", err)
	}

	// Get chunk IDs before removal.
	doc, _ := store.GetDocumentByPath(ctx, path)
	chunkIDs, _ := store.ChunkIDsByDocumentID(ctx, doc.ID)

	if err := idx.RemoveFile(ctx, path); err != nil {
		t.Fatalf("RemoveFile() error: %v", err)
	}

	// Document should be gone.
	_, err := store.GetDocumentByPath(ctx, path)
	if err == nil {
		t.Error("expected error after removal")
	}

	// Vectors should be gone.
	for _, id := range chunkIDs {
		hits, _ := vecIndex.Search(make([]float32, 384), 100)
		for _, h := range hits {
			if h.ID == id {
				t.Errorf("chunk %q still in vector index after removal", id)
			}
		}
	}
}

func TestIndexFileUnsupportedExtension(t *testing.T) {
	idx, _, _ := newTestIndexer(t)
	ctx := context.Background()

	dir := t.TempDir()
	path := filepath.Join(dir, "test.xyz")
	if err := os.WriteFile(path, []byte("data"), 0644); err != nil {
		t.Fatal(err)
	}

	err := idx.IndexFile(ctx, path)
	if err == nil {
		t.Error("expected error for unsupported extension")
	}
}

func TestIndexFileExceedsMaxSize(t *testing.T) {
	idx, _, _ := newTestIndexer(t)
	idx.cfg.MaxFileSize = 10 // 10 bytes
	ctx := context.Background()

	dir := t.TempDir()
	path := filepath.Join(dir, "big.md")
	if err := os.WriteFile(path, []byte("this content exceeds 10 bytes"), 0644); err != nil {
		t.Fatal(err)
	}

	err := idx.IndexFile(ctx, path)
	if err == nil {
		t.Error("expected error for oversized file")
	}
}

func TestBatchIndexFileTracksFailure(t *testing.T) {
	idx, _, _ := newTestIndexer(t)
	idx.cfg.MaxFileSize = 10
	ctx := context.Background()

	dir := t.TempDir()
	path := filepath.Join(dir, "big.md")
	if err := os.WriteFile(path, []byte("this content exceeds 10 bytes"), 0644); err != nil {
		t.Fatal(err)
	}

	idx.progress.Begin("indexing", 1)
	if err := idx.BatchIndexFile(ctx, path); err == nil {
		t.Fatal("expected error for oversized file")
	}

	status := idx.progress.Status()
	if status.FailedFiles != 1 {
		t.Errorf("expected 1 failed file, got %d", status.FailedFiles)
	}
	if status.Percent != 100 {
		t.Errorf("expected 100 percent, got %d", status.Percent)
	}
	if status.State != "idle" {
		t.Errorf("expected idle state, got %q", status.State)
	}
}

func TestBatchIndexFileTracksSkippedDocument(t *testing.T) {
	idx, _, _ := newTestIndexer(t)
	ctx := context.Background()

	dir := t.TempDir()
	path := filepath.Join(dir, "same.md")
	if err := os.WriteFile(path, []byte("content"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := idx.IndexFile(ctx, path); err != nil {
		t.Fatalf("IndexFile() error: %v", err)
	}

	idx.progress.Begin("rebuilding", 1)
	if err := idx.BatchIndexFile(ctx, path); err != nil {
		t.Fatalf("BatchIndexFile() error: %v", err)
	}

	status := idx.progress.Status()
	if status.IndexedFiles != 1 {
		t.Errorf("expected 1 indexed file, got %d", status.IndexedFiles)
	}
	if status.Percent != 100 {
		t.Errorf("expected 100 percent, got %d", status.Percent)
	}
	if status.State != "idle" {
		t.Errorf("expected idle state, got %q", status.State)
	}
}

func TestRemoveFileMissingDocumentNoop(t *testing.T) {
	idx, _, _ := newTestIndexer(t)
	ctx := context.Background()

	path := filepath.Join(t.TempDir(), "missing.md")
	if err := idx.RemoveFile(ctx, path); err != nil {
		t.Fatalf("RemoveFile() should be a no-op, got error: %v", err)
	}
}

func TestIndexerCanIndex(t *testing.T) {
	idx, _, _ := newTestIndexer(t)

	if !idx.CanIndex("/tmp/doc.md") {
		t.Fatal("expected .md file to be indexable")
	}
	if idx.CanIndex("/tmp/doc.csv") {
		t.Fatal("expected .csv file to be rejected")
	}
}
