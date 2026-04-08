package memory

import (
	"context"
	"testing"

	"go.uber.org/zap"
)

// newTestExtractor returns an EntityExtractor using a no-op logger.
func newTestExtractor(t *testing.T) *EntityExtractor {
	t.Helper()
	return NewEntityExtractor(zap.NewNop())
}

// entityMap converts a slice of ExtractedEntity to a map[name]type for
// easy membership testing.
func entityMap(entities []ExtractedEntity) map[string]string {
	m := make(map[string]string, len(entities))
	for _, e := range entities {
		m[e.Name] = e.Type
	}
	return m
}

// --- Extraction tests -------------------------------------------------------

// TestExtract_Dates verifies that ISO, long-form, and slash-form dates are
// all detected with type "date".
func TestExtract_Dates(t *testing.T) {
	ex := newTestExtractor(t)

	tests := []struct {
		name     string
		text     string
		wantName string
	}{
		{"ISO date", "The release was on 2026-03-12 afternoon.", "2026-03-12"},
		{"long form", "We met on March 12, 2026 to discuss it.", "March 12, 2026"},
		{"slash form", "Date: 12/03/2026 confirmed.", "12/03/2026"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := entityMap(ex.Extract(tc.text))
			typ, ok := got[tc.wantName]
			if !ok {
				t.Errorf("expected entity %q not found in %v", tc.wantName, got)
				return
			}
			if typ != "date" {
				t.Errorf("entity %q: got type %q, want %q", tc.wantName, typ, "date")
			}
		})
	}
}

// TestExtract_URLs verifies that http and https URLs are detected.
func TestExtract_URLs(t *testing.T) {
	ex := newTestExtractor(t)

	tests := []struct {
		name     string
		text     string
		wantName string
	}{
		{"https URL", "See https://example.com/path for details.", "https://example.com/path"},
		{"http URL", "Visit http://old-site.org/page today.", "http://old-site.org/page"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := entityMap(ex.Extract(tc.text))
			typ, ok := got[tc.wantName]
			if !ok {
				t.Errorf("expected URL %q not found in %v", tc.wantName, got)
				return
			}
			if typ != "url" {
				t.Errorf("entity %q: got type %q, want %q", tc.wantName, typ, "url")
			}
		})
	}
}

// TestExtract_Emails verifies that email addresses are detected.
func TestExtract_Emails(t *testing.T) {
	ex := newTestExtractor(t)

	tests := []struct {
		name     string
		text     string
		wantName string
	}{
		{"simple email", "Contact us at hello@example.com for info.", "hello@example.com"},
		{"plus address", "Send to user+tag@sub.domain.org now.", "user+tag@sub.domain.org"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := entityMap(ex.Extract(tc.text))
			typ, ok := got[tc.wantName]
			if !ok {
				t.Errorf("expected email %q not found in %v", tc.wantName, got)
				return
			}
			if typ != "email" {
				t.Errorf("entity %q: got type %q, want %q", tc.wantName, typ, "email")
			}
		})
	}
}

// TestExtract_Persons verifies that capitalized 2-word sequences not at
// sentence start are extracted as "person".
func TestExtract_Persons(t *testing.T) {
	ex := newTestExtractor(t)

	tests := []struct {
		name     string
		text     string
		wantName string
	}{
		{
			"two-word name mid-sentence",
			"Yesterday I met John Smith at the conference.",
			"John Smith",
		},
		{
			"three-word name mid-sentence",
			"The talk was given by Mary Jane Watson.",
			"Mary Jane Watson",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := entityMap(ex.Extract(tc.text))
			typ, ok := got[tc.wantName]
			if !ok {
				t.Errorf("expected person %q not found in %v", tc.wantName, got)
				return
			}
			if typ != "person" {
				t.Errorf("entity %q: got type %q, want %q", tc.wantName, typ, "person")
			}
		})
	}
}

// TestExtract_NothingFromLowercase verifies that fully-lowercase text with no
// special patterns yields no entities.
func TestExtract_NothingFromLowercase(t *testing.T) {
	ex := newTestExtractor(t)
	text := "this is just some ordinary lowercase text with no special things."
	got := ex.Extract(text)
	if len(got) != 0 {
		t.Errorf("expected no entities from lowercase text, got %v", got)
	}
}

// --- Store integration tests ------------------------------------------------

// TestStore_EntityLinkingOnCreate creates a memory whose content contains
// a date and an email, then verifies that GetEntities returns both.
func TestStore_EntityLinkingOnCreate(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	m, err := s.Create(ctx, CreateMemoryInput{
		Content: "Meeting scheduled for 2026-03-14. Contact alice@example.com for details.",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	entities, err := s.GetEntities(ctx, m.ID)
	if err != nil {
		t.Fatalf("GetEntities: %v", err)
	}

	em := make(map[string]string)
	for _, e := range entities {
		em[e.Name] = e.Type
	}

	if em["2026-03-14"] != "date" {
		t.Errorf("expected date entity '2026-03-14', got entities: %v", em)
	}
	if em["alice@example.com"] != "email" {
		t.Errorf("expected email entity 'alice@example.com', got entities: %v", em)
	}
}

// TestStore_SearchByEntity verifies that SearchByEntity finds memories that
// contain a linked entity matching the search term.
func TestStore_SearchByEntity(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	_, err := s.Create(ctx, CreateMemoryInput{
		Content: "Discussed with Bob Johnson about the Q1 roadmap.",
	})
	if err != nil {
		t.Fatalf("Create memory 1: %v", err)
	}

	_, err = s.Create(ctx, CreateMemoryInput{
		Content: "No relevant names here, just ordinary text.",
	})
	if err != nil {
		t.Fatalf("Create memory 2: %v", err)
	}

	results, err := s.SearchByEntity(ctx, "Bob Johnson")
	if err != nil {
		t.Fatalf("SearchByEntity: %v", err)
	}

	if len(results) == 0 {
		t.Fatal("expected at least one memory matching 'Bob Johnson', got none")
	}
	found := false
	for _, m := range results {
		if m.Content == "Discussed with Bob Johnson about the Q1 roadmap." {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected memory with Bob Johnson not in results: %v", results)
	}
}

// TestStore_DuplicateEntitiesMerge verifies that when the same entity appears
// in two different memories, the mention_count in the entities table reaches 2.
func TestStore_DuplicateEntitiesMerge(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	_, err := s.Create(ctx, CreateMemoryInput{
		Content: "Sent email to carol@example.com today.",
	})
	if err != nil {
		t.Fatalf("Create memory 1: %v", err)
	}

	_, err = s.Create(ctx, CreateMemoryInput{
		Content: "Received reply from carol@example.com as expected.",
	})
	if err != nil {
		t.Fatalf("Create memory 2: %v", err)
	}

	// Query the mention_count directly.
	var count int
	err = s.db.QueryRowContext(ctx,
		`SELECT mention_count FROM entities WHERE name = ? AND entity_type = ?`,
		"carol@example.com", "email",
	).Scan(&count)
	if err != nil {
		t.Fatalf("querying mention_count: %v", err)
	}
	if count != 2 {
		t.Errorf("mention_count: got %d, want 2", count)
	}
}

// TestStore_CascadeDeleteMemoryEntities verifies that deleting a memory also
// removes its rows from memory_entities (FK ON DELETE CASCADE).
func TestStore_CascadeDeleteMemoryEntities(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	m, err := s.Create(ctx, CreateMemoryInput{
		Content: "Meeting on 2026-03-14 with Dave Evans.",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Confirm at least one entity was linked.
	entities, err := s.GetEntities(ctx, m.ID)
	if err != nil {
		t.Fatalf("GetEntities before delete: %v", err)
	}
	if len(entities) == 0 {
		t.Fatal("expected entities linked before delete, got none")
	}

	if err := s.Delete(ctx, m.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	var linkCount int
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM memory_entities WHERE memory_id = ?`, m.ID,
	).Scan(&linkCount); err != nil {
		t.Fatalf("counting memory_entities after delete: %v", err)
	}
	if linkCount != 0 {
		t.Errorf("expected 0 memory_entities after cascade delete, got %d", linkCount)
	}
}
