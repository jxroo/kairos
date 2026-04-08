//go:build cgo

// NOTE: all tests in this package share the global Rust engine singleton
// (OnceLock). Only the first Init() call sets the data directory; subsequent
// calls are no-ops. Tests must not rely on index isolation between cases.

package vecbridge

import (
	"context"
	"os"
	"sort"
	"testing"
)

func requireOnlineTests(t *testing.T) {
	t.Helper()
	if os.Getenv("KAIROS_ONLINE_TESTS") != "1" {
		t.Skip("set KAIROS_ONLINE_TESTS=1 to run real vecbridge smoke tests")
	}
}

// TestInitSucceeds verifies that Init does not return an error for a valid
// temporary directory.
func TestInitSucceeds(t *testing.T) {
	requireOnlineTests(t)
	dir := t.TempDir()
	if err := Init(dir); err != nil {
		t.Fatalf("Init(%q) returned unexpected error: %v", dir, err)
	}
	// Idempotency: calling Init again with the same dir must also succeed.
	if err := Init(dir); err != nil {
		t.Fatalf("Init(%q) second call returned unexpected error: %v", dir, err)
	}
}

// TestEmbedReturns384Dim verifies that Embed produces a 384-dimensional vector.
func TestEmbedReturns384Dim(t *testing.T) {
	requireOnlineTests(t)
	dir := t.TempDir()
	e, err := NewRustEmbedder(dir)
	if err != nil {
		t.Fatalf("NewRustEmbedder: %v", err)
	}

	vec, err := e.Embed(context.Background(), "hello world")
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}
	if len(vec) != 384 {
		t.Fatalf("expected 384-dimensional vector, got %d", len(vec))
	}

	// Dimension() must agree.
	if d := e.Dimension(); d != 384 {
		t.Fatalf("Dimension() = %d, want 384", d)
	}
}

// TestAddSearchRoundTrip verifies that a vector added to RustIndex can be
// retrieved via Search.
func TestAddSearchRoundTrip(t *testing.T) {
	requireOnlineTests(t)
	dir := t.TempDir()

	e, err := NewRustEmbedder(dir)
	if err != nil {
		t.Fatalf("NewRustEmbedder: %v", err)
	}
	idx, err := NewRustIndex(dir)
	if err != nil {
		t.Fatalf("NewRustIndex: %v", err)
	}

	ctx := context.Background()

	query := "the quick brown fox"
	qvec, err := e.Embed(ctx, query)
	if err != nil {
		t.Fatalf("Embed query: %v", err)
	}

	// Add one document.
	const docID = "doc-fox"
	docVec, err := e.Embed(ctx, "the quick brown fox jumps over the lazy dog")
	if err != nil {
		t.Fatalf("Embed doc: %v", err)
	}
	if err := idx.Add(docID, docVec); err != nil {
		t.Fatalf("Add: %v", err)
	}

	hits, err := idx.Search(qvec, 1)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(hits) == 0 {
		t.Fatal("Search returned no results")
	}
	if hits[0].ID != docID {
		t.Errorf("expected hit id %q, got %q", docID, hits[0].ID)
	}
}

// TestDeleteRemovesVector verifies that a deleted vector no longer appears in
// search results.
func TestDeleteRemovesVector(t *testing.T) {
	requireOnlineTests(t)
	dir := t.TempDir()

	e, err := NewRustEmbedder(dir)
	if err != nil {
		t.Fatalf("NewRustEmbedder: %v", err)
	}
	idx, err := NewRustIndex(dir)
	if err != nil {
		t.Fatalf("NewRustIndex: %v", err)
	}

	ctx := context.Background()

	vec, err := e.Embed(ctx, "unique phrase to delete")
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}

	const id = "to-delete"
	if err := idx.Add(id, vec); err != nil {
		t.Fatalf("Add: %v", err)
	}

	if err := idx.Delete(id); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	hits, err := idx.Search(vec, 10)
	if err != nil {
		t.Fatalf("Search after delete: %v", err)
	}
	for _, h := range hits {
		if h.ID == id {
			t.Errorf("deleted id %q still appears in search results", id)
		}
	}
}

// TestResultsSortedByScore verifies that Search returns hits ordered by score
// in descending order.
func TestResultsSortedByScore(t *testing.T) {
	requireOnlineTests(t)
	dir := t.TempDir()

	e, err := NewRustEmbedder(dir)
	if err != nil {
		t.Fatalf("NewRustEmbedder: %v", err)
	}
	idx, err := NewRustIndex(dir)
	if err != nil {
		t.Fatalf("NewRustIndex: %v", err)
	}

	ctx := context.Background()

	docs := []struct {
		id   string
		text string
	}{
		{"doc-a", "cats and dogs"},
		{"doc-b", "machine learning neural networks"},
		{"doc-c", "cooking recipes pasta"},
	}

	for _, d := range docs {
		v, err := e.Embed(ctx, d.text)
		if err != nil {
			t.Fatalf("Embed %q: %v", d.id, err)
		}
		if err := idx.Add(d.id, v); err != nil {
			t.Fatalf("Add %q: %v", d.id, err)
		}
	}

	query, err := e.Embed(ctx, "deep learning")
	if err != nil {
		t.Fatalf("Embed query: %v", err)
	}

	hits, err := idx.Search(query, len(docs))
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(hits) == 0 {
		t.Fatal("Search returned no results")
	}

	// Verify scores are monotonically non-increasing (descending).
	if !sort.SliceIsSorted(hits, func(i, j int) bool {
		return hits[i].Score >= hits[j].Score
	}) {
		t.Errorf("search results not sorted by score descending: %v", hits)
	}
}
