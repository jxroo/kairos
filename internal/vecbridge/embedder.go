//go:build cgo

package vecbridge

import (
	"context"
	"fmt"

	"github.com/jxroo/kairos/internal/memory"
)

// Compile-time assertions that RustEmbedder and RustIndex satisfy the
// interfaces defined in internal/memory.
var (
	_ memory.Embedder    = (*RustEmbedder)(nil)
	_ memory.VectorIndex = (*RustIndex)(nil)
)

// ---------------------------------------------------------------------------
// RustEmbedder
// ---------------------------------------------------------------------------

// RustEmbedder implements memory.Embedder by delegating to the Rust engine
// via CGO. The engine is initialized once during construction.
type RustEmbedder struct{}

// NewRustEmbedder initializes the Rust engine with dataDir and returns a
// ready RustEmbedder. The dataDir is used to load/persist model state.
func NewRustEmbedder(dataDir string) (*RustEmbedder, error) {
	if err := Init(dataDir); err != nil {
		return nil, fmt.Errorf("creating RustEmbedder: %w", err)
	}
	return &RustEmbedder{}, nil
}

// Embed converts text into a 384-dimensional float32 vector.
// The ctx parameter is accepted for interface compatibility; cancellation
// is not propagated into the synchronous Rust call.
func (e *RustEmbedder) Embed(_ context.Context, text string) ([]float32, error) {
	vec, err := Embed(text)
	if err != nil {
		return nil, fmt.Errorf("RustEmbedder.Embed: %w", err)
	}
	return vec, nil
}

// Dimension returns the fixed embedding dimension (384).
func (e *RustEmbedder) Dimension() int {
	return Dimension()
}

// ---------------------------------------------------------------------------
// RustIndex
// ---------------------------------------------------------------------------

// RustIndex implements memory.VectorIndex by delegating to the Rust engine
// via CGO. The engine is initialized once during construction.
type RustIndex struct{}

// NewRustIndex initializes the Rust engine with dataDir and returns a ready
// RustIndex.
func NewRustIndex(dataDir string) (*RustIndex, error) {
	if err := Init(dataDir); err != nil {
		return nil, fmt.Errorf("creating RustIndex: %w", err)
	}
	return &RustIndex{}, nil
}

// Add inserts (or replaces) the vector for id in the index.
func (idx *RustIndex) Add(id string, vector []float32) error {
	if err := Add(id, vector); err != nil {
		return fmt.Errorf("RustIndex.Add: %w", err)
	}
	return nil
}

// Search returns up to k nearest neighbors of query, ordered by score
// descending.
func (idx *RustIndex) Search(query []float32, k int) ([]memory.SearchHit, error) {
	hits, err := Search(query, k)
	if err != nil {
		return nil, fmt.Errorf("RustIndex.Search: %w", err)
	}
	return hits, nil
}

// Delete removes the vector with the given id from the index. Returns an
// error if the id does not exist in the index.
func (idx *RustIndex) Delete(id string) error {
	if err := DeleteVector(id); err != nil {
		return fmt.Errorf("RustIndex.Delete: %w", err)
	}
	return nil
}

// Close persists the index to disk by calling Save.
func (idx *RustIndex) Close() error {
	if err := Save(); err != nil {
		return fmt.Errorf("RustIndex.Close: %w", err)
	}
	return nil
}
