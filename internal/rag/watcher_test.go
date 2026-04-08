package rag

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/jxroo/kairos/internal/config"
)

// mockIndexer records calls for testing.
type mockIndexer struct {
	mu           sync.Mutex
	indexed      []string
	batchIndexed []string
	removed      []string
	indexErr     error
}

func (m *mockIndexer) IndexFile(_ context.Context, path string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.indexed = append(m.indexed, path)
	return m.indexErr
}

func (m *mockIndexer) BatchIndexFile(_ context.Context, path string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.batchIndexed = append(m.batchIndexed, path)
	return m.indexErr
}

func (m *mockIndexer) CanIndex(path string) bool {
	ext := filepath.Ext(path)
	return ext == ".md" || ext == ".txt"
}

func (m *mockIndexer) RemoveFile(_ context.Context, path string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.removed = append(m.removed, path)
	return nil
}

func (m *mockIndexer) indexedPaths() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := make([]string, len(m.indexed))
	copy(cp, m.indexed)
	return cp
}

func (m *mockIndexer) batchIndexedPaths() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := make([]string, len(m.batchIndexed))
	copy(cp, m.batchIndexed)
	return cp
}

func (m *mockIndexer) removedPaths() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := make([]string, len(m.removed))
	copy(cp, m.removed)
	return cp
}

func TestWatcherInitialScan(t *testing.T) {
	dir := t.TempDir()
	// Create some files.
	for _, name := range []string{"a.md", "b.txt", "c.xyz"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("content"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	mock := &mockIndexer{}
	cfg := &config.RAGConfig{
		Enabled:    true,
		WatchPaths: []string{dir},
		Extensions: []string{".md", ".txt"},
		IgnoreDirs: []string{".git"},
	}

	w := NewWatcher(mock, cfg, NewProgress(), zap.NewNop())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := w.Start(ctx); err != nil {
		t.Fatalf("Start() error: %v", err)
	}

	// Wait for initial scan.
	time.Sleep(500 * time.Millisecond)

	indexed := mock.batchIndexedPaths()
	if len(indexed) != 2 {
		t.Errorf("expected 2 indexed files (.md, .txt), got %d: %v", len(indexed), indexed)
	}

	w.Stop()
}

func TestWatcherIgnoresGitDir(t *testing.T) {
	dir := t.TempDir()
	gitDir := filepath.Join(dir, ".git")
	if err := os.MkdirAll(gitDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(gitDir, "config.md"), []byte("git config"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "readme.md"), []byte("readme"), 0644); err != nil {
		t.Fatal(err)
	}

	mock := &mockIndexer{}
	cfg := &config.RAGConfig{
		Enabled:    true,
		WatchPaths: []string{dir},
		Extensions: []string{".md"},
		IgnoreDirs: []string{".git"},
	}

	w := NewWatcher(mock, cfg, NewProgress(), zap.NewNop())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := w.Start(ctx); err != nil {
		t.Fatalf("Start() error: %v", err)
	}

	time.Sleep(500 * time.Millisecond)

	indexed := mock.batchIndexedPaths()
	// Should only index readme.md, not .git/config.md.
	if len(indexed) != 1 {
		t.Errorf("expected 1 indexed file, got %d: %v", len(indexed), indexed)
	}

	w.Stop()
}

func TestWatcherDetectsCreate(t *testing.T) {
	dir := t.TempDir()

	mock := &mockIndexer{}
	cfg := &config.RAGConfig{
		Enabled:    true,
		WatchPaths: []string{dir},
		Extensions: []string{".md"},
		IgnoreDirs: []string{".git"},
	}

	w := NewWatcher(mock, cfg, NewProgress(), zap.NewNop())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := w.Start(ctx); err != nil {
		t.Fatalf("Start() error: %v", err)
	}

	// Wait for initial scan (empty dir).
	time.Sleep(300 * time.Millisecond)

	// Create a new file.
	newFile := filepath.Join(dir, "new.md")
	if err := os.WriteFile(newFile, []byte("new content"), 0644); err != nil {
		t.Fatal(err)
	}

	// Wait for debounce + processing.
	time.Sleep(500 * time.Millisecond)

	indexed := mock.indexedPaths()
	found := false
	for _, p := range indexed {
		if p == newFile {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected new.md to be indexed, paths: %v", indexed)
	}

	w.Stop()
}

func TestWatcherDetectsDelete(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "delete.md")
	if err := os.WriteFile(path, []byte("to delete"), 0644); err != nil {
		t.Fatal(err)
	}

	mock := &mockIndexer{}
	cfg := &config.RAGConfig{
		Enabled:    true,
		WatchPaths: []string{dir},
		Extensions: []string{".md"},
		IgnoreDirs: []string{".git"},
	}

	w := NewWatcher(mock, cfg, NewProgress(), zap.NewNop())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := w.Start(ctx); err != nil {
		t.Fatalf("Start() error: %v", err)
	}

	time.Sleep(300 * time.Millisecond)

	// Delete the file.
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}

	time.Sleep(500 * time.Millisecond)

	removed := mock.removedPaths()
	found := false
	for _, p := range removed {
		if p == path {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected delete.md to be removed, paths: %v", removed)
	}

	w.Stop()
}

func TestWatcherIgnoresUnsupportedExtension(t *testing.T) {
	dir := t.TempDir()

	mock := &mockIndexer{}
	cfg := &config.RAGConfig{
		Enabled:    true,
		WatchPaths: []string{dir},
		Extensions: []string{".md"},
		IgnoreDirs: []string{".git"},
	}

	w := NewWatcher(mock, cfg, NewProgress(), zap.NewNop())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := w.Start(ctx); err != nil {
		t.Fatalf("Start() error: %v", err)
	}

	time.Sleep(200 * time.Millisecond)

	// Create unsupported file.
	if err := os.WriteFile(filepath.Join(dir, "data.csv"), []byte("a,b,c"), 0644); err != nil {
		t.Fatal(err)
	}

	time.Sleep(500 * time.Millisecond)

	indexed := mock.indexedPaths()
	if len(indexed) != 0 {
		t.Errorf("expected 0 indexed files, got %d: %v", len(indexed), indexed)
	}

	w.Stop()
}
