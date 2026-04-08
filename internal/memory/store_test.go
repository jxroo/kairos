package memory

import (
	"context"
	"errors"
	"database/sql"
	"path/filepath"
	"testing"

	_ "github.com/mattn/go-sqlite3"
	"go.uber.org/zap"
)

// newTestStore creates a Store backed by a temp SQLite file.
func newTestStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	logger := zap.NewNop()
	s, err := NewStore(filepath.Join(dir, "test.db"), logger)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func strPtr(s string) *string { return &s }

// TestStore_CreateAllFields verifies that Create stores every provided field.
func TestStore_CreateAllFields(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	in := CreateMemoryInput{
		Content:        "hello world",
		Importance:     "high",
		ConversationID: "conv-1",
		Source:         "chat",
		Tags:           []string{"go", "test"},
	}

	m, err := s.Create(ctx, in)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if m.ID == "" {
		t.Error("expected non-empty ID")
	}
	if m.Content != in.Content {
		t.Errorf("content: got %q, want %q", m.Content, in.Content)
	}
	if m.Importance != "high" {
		t.Errorf("importance: got %q, want high", m.Importance)
	}
	if m.ImportanceWeight != 2.0 {
		t.Errorf("importance_weight: got %f, want 2.0", m.ImportanceWeight)
	}
	if m.ConversationID != "conv-1" {
		t.Errorf("conversation_id: got %q, want conv-1", m.ConversationID)
	}
	if m.Source != "chat" {
		t.Errorf("source: got %q, want chat", m.Source)
	}
	if len(m.Tags) != 2 {
		t.Errorf("tags length: got %d, want 2", len(m.Tags))
	}
}

// TestStore_CreateDefaults verifies that Create applies sensible defaults
// when optional fields are omitted.
func TestStore_CreateDefaults(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	m, err := s.Create(ctx, CreateMemoryInput{Content: "bare minimum"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if m.Importance != "normal" {
		t.Errorf("importance default: got %q, want normal", m.Importance)
	}
	if m.ImportanceWeight != 1.0 {
		t.Errorf("importance_weight default: got %f, want 1.0", m.ImportanceWeight)
	}
	if m.DecayScore != 1.0 {
		t.Errorf("decay_score default: got %f, want 1.0", m.DecayScore)
	}
	if m.AccessCount != 0 {
		t.Errorf("access_count default: got %d, want 0", m.AccessCount)
	}
	if m.Tags != nil && len(m.Tags) != 0 {
		t.Errorf("tags default: got %v, want empty", m.Tags)
	}
}

// TestStore_GetNonExistent verifies that Get returns a wrapped sql.ErrNoRows
// for an unknown ID.
func TestStore_GetNonExistent(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	_, err := s.Get(ctx, "does-not-exist")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("error does not wrap sql.ErrNoRows: %v", err)
	}
}

// TestStore_UpdateContent verifies that Update correctly changes content.
func TestStore_UpdateContent(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	m, err := s.Create(ctx, CreateMemoryInput{Content: "original"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	updated, err := s.Update(ctx, m.ID, UpdateMemoryInput{Content: strPtr("updated content")})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated.Content != "updated content" {
		t.Errorf("content: got %q, want %q", updated.Content, "updated content")
	}
	// Importance should remain unchanged.
	if updated.Importance != "normal" {
		t.Errorf("importance unexpectedly changed to %q", updated.Importance)
	}
}

// TestStore_UpdateTags verifies that Update replaces tags atomically.
func TestStore_UpdateTags(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	m, err := s.Create(ctx, CreateMemoryInput{
		Content: "tagging test",
		Tags:    []string{"old-tag"},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	updated, err := s.Update(ctx, m.ID, UpdateMemoryInput{Tags: []string{"new-tag", "another"}})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}

	if len(updated.Tags) != 2 {
		t.Fatalf("expected 2 tags, got %d: %v", len(updated.Tags), updated.Tags)
	}
	tagSet := make(map[string]bool)
	for _, tg := range updated.Tags {
		tagSet[tg] = true
	}
	if tagSet["old-tag"] {
		t.Error("old-tag should have been removed")
	}
	if !tagSet["new-tag"] || !tagSet["another"] {
		t.Errorf("new tags not found: %v", updated.Tags)
	}
}

// TestStore_Delete verifies that Delete removes the memory row.
func TestStore_Delete(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	m, err := s.Create(ctx, CreateMemoryInput{Content: "to be deleted"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := s.Delete(ctx, m.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	_, err = s.Get(ctx, m.ID)
	if !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("expected ErrNoRows after delete, got: %v", err)
	}
}

// TestStore_DeleteCascadesTags verifies that deleting a memory also removes
// its tags (FK ON DELETE CASCADE).
func TestStore_DeleteCascadesTags(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	m, err := s.Create(ctx, CreateMemoryInput{
		Content: "cascade test",
		Tags:    []string{"cascade-tag"},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := s.Delete(ctx, m.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	var count int
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM memory_tags WHERE memory_id = ?`, m.ID,
	).Scan(&count); err != nil {
		t.Fatalf("counting tags: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 tags after cascade delete, got %d", count)
	}
}

// TestStore_ListAll verifies that List returns all memories when no tags
// filter is applied.
func TestStore_ListAll(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		if _, err := s.Create(ctx, CreateMemoryInput{
			Content: "memory",
		}); err != nil {
			t.Fatalf("Create %d: %v", i, err)
		}
	}

	memories, err := s.List(ctx, ListOptions{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(memories) != 3 {
		t.Errorf("expected 3 memories, got %d", len(memories))
	}
}

// TestStore_ListByTags verifies that List filters by tag correctly.
func TestStore_ListByTags(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	if _, err := s.Create(ctx, CreateMemoryInput{Content: "tagged", Tags: []string{"alpha"}}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := s.Create(ctx, CreateMemoryInput{Content: "untagged"}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := s.Create(ctx, CreateMemoryInput{Content: "beta-tagged", Tags: []string{"beta"}}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	memories, err := s.List(ctx, ListOptions{Tags: []string{"alpha"}})
	if err != nil {
		t.Fatalf("List by tags: %v", err)
	}
	if len(memories) != 1 {
		t.Errorf("expected 1 memory with tag alpha, got %d", len(memories))
	}
	if memories[0].Content != "tagged" {
		t.Errorf("unexpected content: %q", memories[0].Content)
	}
}

// TestStore_ListPagination verifies that Limit and Offset work correctly.
func TestStore_ListPagination(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		if _, err := s.Create(ctx, CreateMemoryInput{Content: "mem"}); err != nil {
			t.Fatalf("Create %d: %v", i, err)
		}
	}

	page1, err := s.List(ctx, ListOptions{Limit: 2, Offset: 0})
	if err != nil {
		t.Fatalf("List page 1: %v", err)
	}
	if len(page1) != 2 {
		t.Errorf("page 1: expected 2, got %d", len(page1))
	}

	page2, err := s.List(ctx, ListOptions{Limit: 2, Offset: 2})
	if err != nil {
		t.Fatalf("List page 2: %v", err)
	}
	if len(page2) != 2 {
		t.Errorf("page 2: expected 2, got %d", len(page2))
	}

	page3, err := s.List(ctx, ListOptions{Limit: 2, Offset: 4})
	if err != nil {
		t.Fatalf("List page 3: %v", err)
	}
	if len(page3) != 1 {
		t.Errorf("page 3: expected 1, got %d", len(page3))
	}
}

// TestStore_RecordAccessIncrementsCount verifies that RecordAccess increments
// access_count by 1 and resets decay_score to 1.0.
func TestStore_RecordAccessIncrementsCount(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	m, err := s.Create(ctx, CreateMemoryInput{Content: "access test"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if m.AccessCount != 0 {
		t.Fatalf("initial access_count should be 0, got %d", m.AccessCount)
	}

	// Manually set decay_score to something other than 1.0 to verify reset.
	if _, err := s.db.ExecContext(ctx,
		`UPDATE memories SET decay_score = 0.3 WHERE id = ?`, m.ID,
	); err != nil {
		t.Fatalf("setting decay_score: %v", err)
	}

	if err := s.RecordAccess(ctx, m.ID); err != nil {
		t.Fatalf("RecordAccess: %v", err)
	}

	got, err := s.Get(ctx, m.ID)
	if err != nil {
		t.Fatalf("Get after RecordAccess: %v", err)
	}
	if got.AccessCount != 1 {
		t.Errorf("access_count: got %d, want 1", got.AccessCount)
	}
	if got.DecayScore != 1.0 {
		t.Errorf("decay_score: got %f, want 1.0", got.DecayScore)
	}
}
