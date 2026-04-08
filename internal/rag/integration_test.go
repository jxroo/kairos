package rag

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/jxroo/kairos/internal/config"
	"github.com/jxroo/kairos/internal/memory"
)

func setupIntegration(t *testing.T) (*Indexer, *RAGSearchService, *Store, *Progress, string) {
	t.Helper()
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "integration.db")
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

	blevePath := filepath.Join(tmpDir, "bleve")
	bleveIdx, err := OpenOrCreateBleve(blevePath)
	if err != nil {
		t.Fatalf("OpenOrCreateBleve() error: %v", err)
	}
	t.Cleanup(func() { bleveIdx.Close() })

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
	indexer := NewIndexer(store, embedder, vecIndex, bleveIdx, registry, chunker, progress, cfg, zap.NewNop())
	searchSvc := NewRAGSearchService(store, embedder, vecIndex, bleveIdx, zap.NewNop())

	return indexer, searchSvc, store, progress, tmpDir
}

func TestIntegrationIndexAndSearch(t *testing.T) {
	indexer, searchSvc, _, _, _ := setupIntegration(t)
	ctx := context.Background()

	// Create test files.
	dir := t.TempDir()
	files := map[string]string{
		"kubernetes.md":  "# Kubernetes\n\nDeployment strategies for production clusters.\nUse rolling updates and blue-green deployments.",
		"python.txt":     "Python machine learning libraries: scikit-learn, tensorflow, pytorch.",
		"main.go":        "package main\n\nfunc main() {\n\tfmt.Println(\"Hello\")\n}",
		"notes.md":       "# Meeting Notes\n\nDiscussed project timeline and milestones.",
	}

	for name, content := range files {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
		if err := indexer.IndexFile(ctx, path); err != nil {
			t.Fatalf("IndexFile(%s) error: %v", name, err)
		}
	}

	// Search for kubernetes content.
	results, err := searchSvc.Search(ctx, RAGSearchQuery{
		Query: "kubernetes deployment",
		Limit: 5,
	})
	if err != nil {
		t.Fatalf("Search() error: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected results for kubernetes query")
	}

	// Top result should reference the kubernetes file.
	if results[0].Document.Filename != "kubernetes.md" {
		t.Errorf("expected kubernetes.md as top result, got %q", results[0].Document.Filename)
	}

	// Results should have source info.
	if results[0].Chunk.StartLine < 1 {
		t.Error("expected positive StartLine")
	}
	if results[0].Document.Path == "" {
		t.Error("expected non-empty document path")
	}
}

func TestIntegrationModifyFile(t *testing.T) {
	indexer, searchSvc, _, _, _ := setupIntegration(t)
	ctx := context.Background()

	dir := t.TempDir()
	path := filepath.Join(dir, "evolving.md")

	// Original content.
	if err := os.WriteFile(path, []byte("Original content about databases"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := indexer.IndexFile(ctx, path); err != nil {
		t.Fatalf("IndexFile() error: %v", err)
	}

	// Modify content.
	if err := os.WriteFile(path, []byte("Updated content about cloud computing and AWS"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := indexer.IndexFile(ctx, path); err != nil {
		t.Fatalf("re-IndexFile() error: %v", err)
	}

	// Search should find updated content.
	results, err := searchSvc.Search(ctx, RAGSearchQuery{
		Query: "cloud computing AWS",
		Limit: 5,
	})
	if err != nil {
		t.Fatalf("Search() error: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected results after modification")
	}
}

func TestIntegrationDeleteFile(t *testing.T) {
	indexer, searchSvc, store, _, _ := setupIntegration(t)
	ctx := context.Background()

	dir := t.TempDir()
	path := filepath.Join(dir, "delete.md")
	if err := os.WriteFile(path, []byte("Content to be deleted entirely"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := indexer.IndexFile(ctx, path); err != nil {
		t.Fatalf("IndexFile() error: %v", err)
	}

	// Verify indexed.
	doc, err := store.GetDocumentByPath(ctx, path)
	if err != nil {
		t.Fatalf("GetDocumentByPath() error: %v", err)
	}
	if doc.Status != StatusIndexed {
		t.Errorf("expected indexed status, got %q", doc.Status)
	}

	// Remove.
	if err := indexer.RemoveFile(ctx, path); err != nil {
		t.Fatalf("RemoveFile() error: %v", err)
	}

	// Document should be gone.
	docs, err := store.ListDocuments(ctx)
	if err != nil {
		t.Fatalf("ListDocuments() error: %v", err)
	}
	if len(docs) != 0 {
		t.Errorf("expected 0 documents after removal, got %d", len(docs))
	}

	// Search should return nothing.
	results, err := searchSvc.Search(ctx, RAGSearchQuery{
		Query: "deleted content",
		Limit: 5,
	})
	if err != nil {
		t.Fatalf("Search() error: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results after deletion, got %d", len(results))
	}
}

func TestIntegrationProgressTracking(t *testing.T) {
	indexer, _, _, progress, _ := setupIntegration(t)
	ctx := context.Background()

	dir := t.TempDir()
	for i := 0; i < 5; i++ {
		name := filepath.Join(dir, "file"+string(rune('a'+i))+".md")
		if err := os.WriteFile(name, []byte("content "+string(rune('a'+i))), 0644); err != nil {
			t.Fatal(err)
		}
	}

	progress.Begin("indexing", 5)
	for i := 0; i < 5; i++ {
		name := filepath.Join(dir, "file"+string(rune('a'+i))+".md")
		if err := indexer.BatchIndexFile(ctx, name); err != nil {
			t.Fatalf("BatchIndexFile() error: %v", err)
		}
	}

	status := progress.Status()
	if status.IndexedFiles != 5 {
		t.Errorf("expected 5 indexed files, got %d", status.IndexedFiles)
	}
	if status.Percent != 100 {
		t.Errorf("expected 100 percent, got %d", status.Percent)
	}
	if status.State != "idle" {
		t.Errorf("expected idle state after batch, got %q", status.State)
	}

	if err := os.WriteFile(filepath.Join(dir, "later.md"), []byte("later content"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := indexer.IndexFile(ctx, filepath.Join(dir, "later.md")); err != nil {
		t.Fatalf("IndexFile() error: %v", err)
	}

	status = progress.Status()
	if status.IndexedFiles != 5 {
		t.Errorf("expected indexed files to stay at 5 after non-batch indexing, got %d", status.IndexedFiles)
	}
}

func TestIntegrationWatcherFlow(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "watcher.db")
	store, err := NewStore(dbPath, zap.NewNop())
	if err != nil {
		t.Fatalf("NewStore() error: %v", err)
	}
	defer store.Close()

	embedder := memory.NewFallbackEmbedder()
	vecIndex, err := memory.NewFallbackIndex("")
	if err != nil {
		t.Fatalf("NewFallbackIndex() error: %v", err)
	}

	blevePath := filepath.Join(tmpDir, "bleve_w")
	bleveIdx, err := OpenOrCreateBleve(blevePath)
	if err != nil {
		t.Fatalf("OpenOrCreateBleve() error: %v", err)
	}
	defer bleveIdx.Close()

	watchDir := filepath.Join(tmpDir, "watched")
	if err := os.MkdirAll(watchDir, 0755); err != nil {
		t.Fatal(err)
	}

	cfg := &config.RAGConfig{
		Enabled:      true,
		WatchPaths:   []string{watchDir},
		Extensions:   []string{".md"},
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

	watcher := NewWatcher(indexer, cfg, progress, zap.NewNop())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := watcher.Start(ctx); err != nil {
		t.Fatalf("Start() error: %v", err)
	}
	defer watcher.Stop()

	// Create a file in the watched directory.
	path := filepath.Join(watchDir, "watched.md")
	if err := os.WriteFile(path, []byte("Watched file about networking"), 0644); err != nil {
		t.Fatal(err)
	}

	// Wait for watcher + debounce.
	time.Sleep(1 * time.Second)

	// Search should find the file.
	results, err := searchSvc.Search(ctx, RAGSearchQuery{
		Query: "networking",
		Limit: 5,
	})
	if err != nil {
		t.Fatalf("Search() error: %v", err)
	}
	if len(results) == 0 {
		t.Error("expected watcher to index the file and make it searchable")
	}
}

func TestIntegrationRebuildAllRemovesStaleDocuments(t *testing.T) {
	indexer, searchSvc, store, _, _ := setupIntegration(t)
	ctx := context.Background()

	dir := t.TempDir()
	oldPath := filepath.Join(dir, "old.md")
	if err := os.WriteFile(oldPath, []byte("legacy notes about postgres"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := indexer.IndexFile(ctx, oldPath); err != nil {
		t.Fatalf("IndexFile() error: %v", err)
	}

	if err := os.Remove(oldPath); err != nil {
		t.Fatal(err)
	}
	newPath := filepath.Join(dir, "new.md")
	if err := os.WriteFile(newPath, []byte("fresh kubernetes deployment notes"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := indexer.RebuildAll(ctx, []string{dir}); err != nil {
		t.Fatalf("RebuildAll() error: %v", err)
	}

	if _, err := store.GetDocumentByPath(ctx, oldPath); err == nil {
		t.Fatal("expected stale document record to be removed after rebuild")
	}

	results, err := searchSvc.Search(ctx, RAGSearchQuery{
		Query: "kubernetes deployment",
		Limit: 5,
	})
	if err != nil {
		t.Fatalf("Search() error: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected rebuilt corpus to be searchable")
	}
	if results[0].Document.Path != newPath {
		t.Errorf("expected rebuilt result from %q, got %q", newPath, results[0].Document.Path)
	}
}

func TestIntegrationCorpusOf100Files(t *testing.T) {
	indexer, searchSvc, _, _, _ := setupIntegration(t)
	ctx := context.Background()

	dir := t.TempDir()
	for i := 0; i < 97; i++ {
		path := filepath.Join(dir, fmt.Sprintf("filler-%03d.txt", i))
		content := fmt.Sprintf("General project notes %d about unrelated planning topics.", i)
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
		if err := indexer.IndexFile(ctx, path); err != nil {
			t.Fatalf("IndexFile(%s) error: %v", path, err)
		}
	}

	targetMD := filepath.Join(dir, "kubernetes.md")
	if err := os.WriteFile(targetMD, []byte("# Kubernetes\n\nDeployment strategies for production clusters and blue green rollout."), 0644); err != nil {
		t.Fatal(err)
	}
	if err := indexer.IndexFile(ctx, targetMD); err != nil {
		t.Fatalf("IndexFile(%s) error: %v", targetMD, err)
	}

	targetPDF := filepath.Join(dir, "runbook.pdf")
	if err := writeSimplePDF(targetPDF, "Kubernetes deployment rollback runbook"); err != nil {
		t.Fatalf("writeSimplePDF() error: %v", err)
	}
	if err := indexer.IndexFile(ctx, targetPDF); err != nil {
		t.Fatalf("IndexFile(%s) error: %v", targetPDF, err)
	}

	targetGo := filepath.Join(dir, "deploy.go")
	if err := os.WriteFile(targetGo, []byte("package main\n\n// kubernetes deployment helper\nfunc deploy() {}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := indexer.IndexFile(ctx, targetGo); err != nil {
		t.Fatalf("IndexFile(%s) error: %v", targetGo, err)
	}

	results, err := searchSvc.Search(ctx, RAGSearchQuery{
		Query: "blue green rollout",
		Limit: 10,
	})
	if err != nil {
		t.Fatalf("Search() error: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected results from 100-file corpus")
	}
	if results[0].Document.Filename != "kubernetes.md" {
		t.Errorf("expected kubernetes.md as top result, got %q", results[0].Document.Filename)
	}
}
