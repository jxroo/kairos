package memory

import "context"

// Embedder converts text into a dense float vector representation.
type Embedder interface {
	Embed(ctx context.Context, text string) ([]float32, error)
	Dimension() int
}

// SearchHit holds a vector search result with its similarity score.
type SearchHit struct {
	ID    string
	Score float32
}

// VectorIndex provides vector storage and approximate nearest-neighbor search.
type VectorIndex interface {
	Add(id string, vector []float32) error
	Search(query []float32, k int) ([]SearchHit, error)
	Delete(id string) error
	Close() error
}
