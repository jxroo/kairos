package memory

import (
	"context"
	"fmt"
	"math"
	"path/filepath"
	"testing"
)

// ── FallbackEmbedder tests ───────────────────────────────────────────────────

func TestFallbackEmbedder_Dimension(t *testing.T) {
	e := NewFallbackEmbedder()
	if got := e.Dimension(); got != 384 {
		t.Fatalf("Dimension() = %d, want 384", got)
	}
}

func TestFallbackEmbedder_OutputLength(t *testing.T) {
	e := NewFallbackEmbedder()
	vec, err := e.Embed(context.Background(), "hello world")
	if err != nil {
		t.Fatalf("Embed returned error: %v", err)
	}
	if len(vec) != 384 {
		t.Fatalf("len(vec) = %d, want 384", len(vec))
	}
}

func TestFallbackEmbedder_Deterministic(t *testing.T) {
	e := NewFallbackEmbedder()
	ctx := context.Background()
	text := "the quick brown fox jumps over the lazy dog"

	v1, err := e.Embed(ctx, text)
	if err != nil {
		t.Fatalf("first Embed: %v", err)
	}
	v2, err := e.Embed(ctx, text)
	if err != nil {
		t.Fatalf("second Embed: %v", err)
	}
	for i := range v1 {
		if v1[i] != v2[i] {
			t.Fatalf("non-deterministic at index %d: %v vs %v", i, v1[i], v2[i])
		}
	}
}

func TestFallbackEmbedder_SimilarMoreThanDissimilar(t *testing.T) {
	e := NewFallbackEmbedder()
	ctx := context.Background()

	vecA, _ := e.Embed(ctx, "machine learning neural network")
	vecB, _ := e.Embed(ctx, "deep learning neural network model")
	vecC, _ := e.Embed(ctx, "banana apple orange fruit salad")

	simAB := cosineSimilarity(vecA, vecB)
	simAC := cosineSimilarity(vecA, vecC)

	if simAB <= simAC {
		t.Fatalf("expected sim(A,B)=%f > sim(A,C)=%f (related > unrelated)", simAB, simAC)
	}
}

func TestFallbackEmbedder_EmptyText(t *testing.T) {
	e := NewFallbackEmbedder()
	vec, err := e.Embed(context.Background(), "")
	if err != nil {
		t.Fatalf("Embed empty text returned error: %v", err)
	}
	if len(vec) != 384 {
		t.Fatalf("len(vec) = %d, want 384", len(vec))
	}
}

// ── cosineSimilarity tests ───────────────────────────────────────────────────

func TestCosineSimilarity_Identity(t *testing.T) {
	v := make([]float32, 384)
	for i := range v {
		v[i] = float32(i + 1)
	}
	got := cosineSimilarity(v, v)
	if math.Abs(float64(got)-1.0) > 1e-5 {
		t.Fatalf("cosine(v,v) = %f, want ~1.0", got)
	}
}

func TestCosineSimilarity_Orthogonal(t *testing.T) {
	a := make([]float32, 4)
	b := make([]float32, 4)
	a[0] = 1.0 // [1,0,0,0]
	b[1] = 1.0 // [0,1,0,0]
	got := cosineSimilarity(a, b)
	if math.Abs(float64(got)) > 1e-6 {
		t.Fatalf("cosine(orthogonal) = %f, want ~0.0", got)
	}
}

// ── FallbackIndex tests ──────────────────────────────────────────────────────

func TestFallbackIndex_AddSearchRoundTrip(t *testing.T) {
	idx, err := NewFallbackIndex("")
	if err != nil {
		t.Fatalf("NewFallbackIndex: %v", err)
	}

	e := NewFallbackEmbedder()
	ctx := context.Background()

	vec, _ := e.Embed(ctx, "persistent memory storage")
	if err := idx.Add("mem-1", vec); err != nil {
		t.Fatalf("Add: %v", err)
	}

	hits, err := idx.Search(vec, 1)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(hits) != 1 {
		t.Fatalf("expected 1 hit, got %d", len(hits))
	}
	if hits[0].ID != "mem-1" {
		t.Fatalf("expected ID mem-1, got %q", hits[0].ID)
	}
	if math.Abs(float64(hits[0].Score)-1.0) > 1e-5 {
		t.Fatalf("score = %f, want ~1.0 for identical vector", hits[0].Score)
	}
}

func TestFallbackIndex_Delete(t *testing.T) {
	idx, err := NewFallbackIndex("")
	if err != nil {
		t.Fatalf("NewFallbackIndex: %v", err)
	}

	vec := []float32{1, 0, 0, 0}
	_ = idx.Add("to-delete", vec)

	if err := idx.Delete("to-delete"); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	hits, err := idx.Search(vec, 5)
	if err != nil {
		t.Fatalf("Search after delete: %v", err)
	}
	for _, h := range hits {
		if h.ID == "to-delete" {
			t.Fatalf("deleted entry still returned in Search")
		}
	}
}

func TestFallbackIndex_Persistence(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "index.gob")

	e := NewFallbackEmbedder()
	ctx := context.Background()

	vec, _ := e.Embed(ctx, "persistence test vector")

	// Write index to disk.
	idx1, err := NewFallbackIndex(path)
	if err != nil {
		t.Fatalf("NewFallbackIndex (create): %v", err)
	}
	if err := idx1.Add("persist-1", vec); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if err := idx1.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// Reload from disk.
	idx2, err := NewFallbackIndex(path)
	if err != nil {
		t.Fatalf("NewFallbackIndex (reload): %v", err)
	}
	hits, err := idx2.Search(vec, 1)
	if err != nil {
		t.Fatalf("Search after reload: %v", err)
	}
	if len(hits) == 0 || hits[0].ID != "persist-1" {
		t.Fatalf("expected persist-1 in reloaded index, got %v", hits)
	}
}

// ── Benchmarks ───────────────────────────────────────────────────────────────

func BenchmarkCosineSimilarity384(b *testing.B) {
	a := make([]float32, 384)
	v := make([]float32, 384)
	for i := range a {
		a[i] = float32(i) * 0.001
		v[i] = float32(384-i) * 0.001
	}
	b.ResetTimer()
	for range b.N {
		cosineSimilarity(a, v)
	}
}

func BenchmarkFallbackIndexSearch10K(b *testing.B) {
	idx, err := NewFallbackIndex("")
	if err != nil {
		b.Fatalf("NewFallbackIndex: %v", err)
	}

	e := NewFallbackEmbedder()
	ctx := context.Background()

	// Populate with 10 000 vectors.
	for i := range 10_000 {
		vec, _ := e.Embed(ctx, fmt.Sprintf("document number %d about some topic", i))
		if addErr := idx.Add(fmt.Sprintf("id-%d", i), vec); addErr != nil {
			b.Fatalf("Add: %v", addErr)
		}
	}

	query, _ := e.Embed(ctx, "document about topic")

	b.ResetTimer()
	for range b.N {
		if _, searchErr := idx.Search(query, 10); searchErr != nil {
			b.Fatalf("Search: %v", searchErr)
		}
	}
}
