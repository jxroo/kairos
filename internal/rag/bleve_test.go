package rag

import (
	"path/filepath"
	"testing"
)

func TestBleveOpenOrCreate(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test_bleve")

	b, err := OpenOrCreateBleve(path)
	if err != nil {
		t.Fatalf("OpenOrCreateBleve() error: %v", err)
	}
	// Must close before re-opening — bbolt uses exclusive file lock.
	b.Close()

	// Re-open existing index.
	b2, err := OpenOrCreateBleve(path)
	if err != nil {
		t.Fatalf("re-open error: %v", err)
	}
	b2.Close()
}

func TestBleveIndexAndSearch(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "search_bleve")

	b, err := OpenOrCreateBleve(path)
	if err != nil {
		t.Fatalf("OpenOrCreateBleve() error: %v", err)
	}
	defer b.Close()

	chunk := Chunk{
		ID:         "chunk-1",
		DocumentID: "doc-1",
		Content:    "Kubernetes deployment strategies for production environments",
	}

	if err := b.IndexChunk(chunk, "/docs/k8s.md"); err != nil {
		t.Fatalf("IndexChunk() error: %v", err)
	}

	chunk2 := Chunk{
		ID:         "chunk-2",
		DocumentID: "doc-2",
		Content:    "Python machine learning libraries and frameworks",
	}
	if err := b.IndexChunk(chunk2, "/docs/ml.md"); err != nil {
		t.Fatalf("IndexChunk() error: %v", err)
	}

	hits, err := b.Search("kubernetes deployment", 10)
	if err != nil {
		t.Fatalf("Search() error: %v", err)
	}

	if len(hits) == 0 {
		t.Fatal("expected at least 1 hit")
	}
	if hits[0].ChunkID != "chunk-1" {
		t.Errorf("expected chunk-1 as top hit, got %q", hits[0].ChunkID)
	}
}

func TestBleveRemoveChunk(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "remove_bleve")

	b, err := OpenOrCreateBleve(path)
	if err != nil {
		t.Fatalf("OpenOrCreateBleve() error: %v", err)
	}
	defer b.Close()

	chunk := Chunk{
		ID:         "chunk-rm",
		DocumentID: "doc-rm",
		Content:    "removable content about databases",
	}
	if err := b.IndexChunk(chunk, "/tmp/db.md"); err != nil {
		t.Fatalf("IndexChunk() error: %v", err)
	}

	if err := b.RemoveChunk("chunk-rm"); err != nil {
		t.Fatalf("RemoveChunk() error: %v", err)
	}

	hits, err := b.Search("databases", 10)
	if err != nil {
		t.Fatalf("Search() error: %v", err)
	}
	if len(hits) != 0 {
		t.Errorf("expected 0 hits after removal, got %d", len(hits))
	}
}

func TestBleveSearchNoResults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty_bleve")

	b, err := OpenOrCreateBleve(path)
	if err != nil {
		t.Fatalf("OpenOrCreateBleve() error: %v", err)
	}
	defer b.Close()

	hits, err := b.Search("nonexistent query", 10)
	if err != nil {
		t.Fatalf("Search() error: %v", err)
	}
	if len(hits) != 0 {
		t.Errorf("expected 0 hits, got %d", len(hits))
	}
}
