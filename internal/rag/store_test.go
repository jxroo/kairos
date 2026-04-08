package rag

import (
	"context"
	"path/filepath"
	"testing"

	"go.uber.org/zap"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	s, err := NewStore(dbPath, zap.NewNop())
	if err != nil {
		t.Fatalf("NewStore() error: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestCreateAndGetDocument(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	doc := &Document{
		Path:      "/tmp/test.md",
		Filename:  "test.md",
		Extension: ".md",
		SizeBytes: 1024,
		FileHash:  "abc123",
		Status:    StatusPending,
	}

	if err := s.CreateDocument(ctx, doc); err != nil {
		t.Fatalf("CreateDocument() error: %v", err)
	}
	if doc.ID == "" {
		t.Error("expected non-empty ID")
	}

	got, err := s.GetDocumentByPath(ctx, "/tmp/test.md")
	if err != nil {
		t.Fatalf("GetDocumentByPath() error: %v", err)
	}
	if got.ID != doc.ID {
		t.Errorf("expected ID %q, got %q", doc.ID, got.ID)
	}
	if got.Path != "/tmp/test.md" {
		t.Errorf("expected path /tmp/test.md, got %q", got.Path)
	}
	if got.FileHash != "abc123" {
		t.Errorf("expected hash abc123, got %q", got.FileHash)
	}
}

func TestGetDocumentByID(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	doc := &Document{
		Path:      "/tmp/test.txt",
		Filename:  "test.txt",
		Extension: ".txt",
		SizeBytes: 256,
		FileHash:  "def456",
		Status:    StatusPending,
	}
	if err := s.CreateDocument(ctx, doc); err != nil {
		t.Fatalf("CreateDocument() error: %v", err)
	}

	got, err := s.GetDocument(ctx, doc.ID)
	if err != nil {
		t.Fatalf("GetDocument() error: %v", err)
	}
	if got.Filename != "test.txt" {
		t.Errorf("expected filename test.txt, got %q", got.Filename)
	}
}

func TestUpdateDocumentStatus(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	doc := &Document{
		Path:      "/tmp/status.md",
		Filename:  "status.md",
		Extension: ".md",
		SizeBytes: 100,
		FileHash:  "hash1",
		Status:    StatusPending,
	}
	if err := s.CreateDocument(ctx, doc); err != nil {
		t.Fatalf("CreateDocument() error: %v", err)
	}

	if err := s.UpdateDocumentStatus(ctx, doc.ID, StatusIndexed, ""); err != nil {
		t.Fatalf("UpdateDocumentStatus() error: %v", err)
	}

	got, err := s.GetDocument(ctx, doc.ID)
	if err != nil {
		t.Fatalf("GetDocument() error: %v", err)
	}
	if got.Status != StatusIndexed {
		t.Errorf("expected status indexed, got %q", got.Status)
	}
	if got.IndexedAt == nil {
		t.Error("expected non-nil IndexedAt")
	}
}

func TestUpdateDocumentStatusError(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	doc := &Document{
		Path:      "/tmp/err.md",
		Filename:  "err.md",
		Extension: ".md",
		SizeBytes: 50,
		FileHash:  "hash2",
		Status:    StatusPending,
	}
	if err := s.CreateDocument(ctx, doc); err != nil {
		t.Fatalf("CreateDocument() error: %v", err)
	}

	if err := s.UpdateDocumentStatus(ctx, doc.ID, StatusError, "parse failed"); err != nil {
		t.Fatalf("UpdateDocumentStatus() error: %v", err)
	}

	got, err := s.GetDocument(ctx, doc.ID)
	if err != nil {
		t.Fatalf("GetDocument() error: %v", err)
	}
	if got.Status != StatusError {
		t.Errorf("expected status error, got %q", got.Status)
	}
	if got.ErrorMsg != "parse failed" {
		t.Errorf("expected error_msg 'parse failed', got %q", got.ErrorMsg)
	}
}

func TestDeleteDocumentCascade(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	doc := &Document{
		Path:      "/tmp/cascade.md",
		Filename:  "cascade.md",
		Extension: ".md",
		SizeBytes: 200,
		FileHash:  "hash3",
		Status:    StatusPending,
	}
	if err := s.CreateDocument(ctx, doc); err != nil {
		t.Fatalf("CreateDocument() error: %v", err)
	}

	chunks := []Chunk{
		{DocumentID: doc.ID, ChunkIndex: 0, Content: "chunk 0", StartLine: 1, EndLine: 5},
		{DocumentID: doc.ID, ChunkIndex: 1, Content: "chunk 1", StartLine: 6, EndLine: 10},
	}
	if err := s.CreateChunks(ctx, chunks); err != nil {
		t.Fatalf("CreateChunks() error: %v", err)
	}

	// Verify chunks exist.
	got, err := s.GetChunksByDocumentID(ctx, doc.ID)
	if err != nil {
		t.Fatalf("GetChunksByDocumentID() error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 chunks, got %d", len(got))
	}

	// Delete document — chunks should cascade.
	if err := s.DeleteDocument(ctx, doc.ID); err != nil {
		t.Fatalf("DeleteDocument() error: %v", err)
	}

	got, err = s.GetChunksByDocumentID(ctx, doc.ID)
	if err != nil {
		t.Fatalf("GetChunksByDocumentID() after delete error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected 0 chunks after cascade, got %d", len(got))
	}
}

func TestCreateAndGetChunks(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	doc := &Document{
		Path:      "/tmp/chunks.md",
		Filename:  "chunks.md",
		Extension: ".md",
		SizeBytes: 500,
		FileHash:  "hash4",
		Status:    StatusPending,
	}
	if err := s.CreateDocument(ctx, doc); err != nil {
		t.Fatalf("CreateDocument() error: %v", err)
	}

	chunks := []Chunk{
		{DocumentID: doc.ID, ChunkIndex: 0, Content: "first chunk", StartLine: 1, EndLine: 3},
		{DocumentID: doc.ID, ChunkIndex: 1, Content: "second chunk", StartLine: 4, EndLine: 6},
		{DocumentID: doc.ID, ChunkIndex: 2, Content: "third chunk", StartLine: 7, EndLine: 9},
	}
	if err := s.CreateChunks(ctx, chunks); err != nil {
		t.Fatalf("CreateChunks() error: %v", err)
	}

	// Verify IDs were assigned.
	for i, c := range chunks {
		if c.ID == "" {
			t.Errorf("chunk %d: expected non-empty ID", i)
		}
	}

	// Get by document ID.
	got, err := s.GetChunksByDocumentID(ctx, doc.ID)
	if err != nil {
		t.Fatalf("GetChunksByDocumentID() error: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("expected 3 chunks, got %d", len(got))
	}
	if got[0].Content != "first chunk" {
		t.Errorf("expected first chunk content, got %q", got[0].Content)
	}

	// Get single chunk.
	single, err := s.GetChunk(ctx, chunks[1].ID)
	if err != nil {
		t.Fatalf("GetChunk() error: %v", err)
	}
	if single.Content != "second chunk" {
		t.Errorf("expected 'second chunk', got %q", single.Content)
	}
}

func TestDeleteChunksByDocumentID(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	doc := &Document{
		Path:      "/tmp/delchunks.md",
		Filename:  "delchunks.md",
		Extension: ".md",
		SizeBytes: 100,
		FileHash:  "hash5",
		Status:    StatusPending,
	}
	if err := s.CreateDocument(ctx, doc); err != nil {
		t.Fatalf("CreateDocument() error: %v", err)
	}

	chunks := []Chunk{
		{DocumentID: doc.ID, ChunkIndex: 0, Content: "c0", StartLine: 1, EndLine: 1},
	}
	if err := s.CreateChunks(ctx, chunks); err != nil {
		t.Fatalf("CreateChunks() error: %v", err)
	}

	if err := s.DeleteChunksByDocumentID(ctx, doc.ID); err != nil {
		t.Fatalf("DeleteChunksByDocumentID() error: %v", err)
	}

	got, err := s.GetChunksByDocumentID(ctx, doc.ID)
	if err != nil {
		t.Fatalf("GetChunksByDocumentID() error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected 0 chunks, got %d", len(got))
	}
}

func TestGetDocumentStats(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	docs := []Document{
		{Path: "/a.md", Filename: "a.md", Extension: ".md", SizeBytes: 10, FileHash: "h1", Status: StatusPending},
		{Path: "/b.md", Filename: "b.md", Extension: ".md", SizeBytes: 20, FileHash: "h2", Status: StatusPending},
		{Path: "/c.md", Filename: "c.md", Extension: ".md", SizeBytes: 30, FileHash: "h3", Status: StatusIndexed},
	}
	for i := range docs {
		if err := s.CreateDocument(ctx, &docs[i]); err != nil {
			t.Fatalf("CreateDocument() error: %v", err)
		}
	}
	// Update one to indexed.
	if err := s.UpdateDocumentStatus(ctx, docs[2].ID, StatusIndexed, ""); err != nil {
		t.Fatalf("UpdateDocumentStatus() error: %v", err)
	}

	stats, err := s.GetDocumentStats(ctx)
	if err != nil {
		t.Fatalf("GetDocumentStats() error: %v", err)
	}

	if stats[StatusPending] != 2 {
		t.Errorf("expected 2 pending, got %d", stats[StatusPending])
	}
	if stats[StatusIndexed] != 1 {
		t.Errorf("expected 1 indexed, got %d", stats[StatusIndexed])
	}
}

func TestListDocuments(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	for _, name := range []string{"b.md", "a.md", "c.md"} {
		doc := &Document{
			Path:      "/tmp/" + name,
			Filename:  name,
			Extension: ".md",
			SizeBytes: 10,
			FileHash:  "h-" + name,
			Status:    StatusPending,
		}
		if err := s.CreateDocument(ctx, doc); err != nil {
			t.Fatalf("CreateDocument() error: %v", err)
		}
	}

	docs, err := s.ListDocuments(ctx)
	if err != nil {
		t.Fatalf("ListDocuments() error: %v", err)
	}
	if len(docs) != 3 {
		t.Fatalf("expected 3 documents, got %d", len(docs))
	}
	// Should be ordered by path.
	if docs[0].Filename != "a.md" {
		t.Errorf("expected first doc a.md, got %q", docs[0].Filename)
	}
}

func TestChunkIDsByDocumentID(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	doc := &Document{
		Path:      "/tmp/ids.md",
		Filename:  "ids.md",
		Extension: ".md",
		SizeBytes: 100,
		FileHash:  "hash6",
		Status:    StatusPending,
	}
	if err := s.CreateDocument(ctx, doc); err != nil {
		t.Fatalf("CreateDocument() error: %v", err)
	}

	chunks := []Chunk{
		{DocumentID: doc.ID, ChunkIndex: 0, Content: "a", StartLine: 1, EndLine: 1},
		{DocumentID: doc.ID, ChunkIndex: 1, Content: "b", StartLine: 2, EndLine: 2},
	}
	if err := s.CreateChunks(ctx, chunks); err != nil {
		t.Fatalf("CreateChunks() error: %v", err)
	}

	ids, err := s.ChunkIDsByDocumentID(ctx, doc.ID)
	if err != nil {
		t.Fatalf("ChunkIDsByDocumentID() error: %v", err)
	}
	if len(ids) != 2 {
		t.Errorf("expected 2 IDs, got %d", len(ids))
	}
}
