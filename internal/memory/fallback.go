package memory

import (
	"context"
	"encoding/gob"
	"fmt"
	"hash/fnv"
	"math"
	"os"
	"sort"
	"strings"
	"sync"
)

// Compile-time interface satisfaction assertions.
var (
	_ Embedder    = (*FallbackEmbedder)(nil)
	_ VectorIndex = (*FallbackIndex)(nil)
)

const fallbackDim = 384

// FallbackEmbedder produces hash-based pseudo-embeddings without any model
// files. It is deterministic — identical text always produces the same vector.
// The embedding quality is poor compared to real models, but it is always
// available as a last-resort fallback.
type FallbackEmbedder struct{}

// NewFallbackEmbedder returns a ready FallbackEmbedder.
func NewFallbackEmbedder() *FallbackEmbedder {
	return &FallbackEmbedder{}
}

// Embed converts text into a 384-dimensional float32 vector using hash-based
// dimensionality reduction. Words are hashed to positions in the vector, then
// the result is L2-normalised to unit length.
func (e *FallbackEmbedder) Embed(_ context.Context, text string) ([]float32, error) {
	vec := make([]float32, fallbackDim)

	words := strings.Fields(strings.ToLower(text))
	if len(words) == 0 {
		// Return a zero vector for empty input (already zeroed by make).
		return vec, nil
	}

	h := fnv.New64a()
	for _, word := range words {
		h.Reset()
		_, _ = h.Write([]byte(word))
		sum := h.Sum64()

		// Primary position: low 32 bits mod dim.
		pos1 := int(sum & 0xFFFFFFFF % uint64(fallbackDim))
		// Secondary position: high 32 bits mod dim — adds more signal.
		pos2 := int((sum >> 32) % uint64(fallbackDim))

		vec[pos1] += 1.0
		vec[pos2] += 0.5
	}

	// L2-normalise using gonum.
	norm := l2NormFloat32(vec)
	if norm > 0 {
		for i := range vec {
			vec[i] /= norm
		}
	}

	return vec, nil
}

// Dimension returns the fixed embedding dimension (384).
func (e *FallbackEmbedder) Dimension() int {
	return fallbackDim
}

// l2NormFloat32 computes the L2 norm of a []float32 slice directly,
// without allocating a float64 intermediate slice.
func l2NormFloat32(v []float32) float32 {
	var sum float32
	for _, x := range v {
		sum += x * x
	}
	return float32(math.Sqrt(float64(sum)))
}

// cosineSimilarity returns the cosine similarity between two equal-length
// float32 vectors. Returns 0 if either vector has zero norm.
// Computed directly in float32 to avoid per-call float64 slice allocations.
func cosineSimilarity(a, b []float32) float32 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}
	var dot, normA, normB float32
	for i := range a {
		dot += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}
	if normA == 0 || normB == 0 {
		return 0
	}
	cos := float64(dot) / (math.Sqrt(float64(normA)) * math.Sqrt(float64(normB)))
	// Clamp to [-1, 1] to guard against floating-point drift.
	cos = math.Max(-1, math.Min(1, cos))
	return float32(cos)
}

// ---------------------------------------------------------------------------
// FallbackIndex
// ---------------------------------------------------------------------------

// gobPayload is the on-disk representation of FallbackIndex state.
type gobPayload struct {
	Vectors map[string][]float32
}

// FallbackIndex is an in-memory brute-force vector index that persists its
// contents to a GOB file on Close.
type FallbackIndex struct {
	mu       sync.RWMutex
	vectors  map[string][]float32
	filePath string
}

// NewFallbackIndex creates (or loads) a FallbackIndex backed by filePath.
// If filePath already exists it is loaded; otherwise an empty index is
// returned. Pass an empty string to get a purely in-memory index that never
// persists.
func NewFallbackIndex(filePath string) (*FallbackIndex, error) {
	idx := &FallbackIndex{
		vectors:  make(map[string][]float32),
		filePath: filePath,
	}

	if filePath == "" {
		return idx, nil
	}

	f, err := os.Open(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return idx, nil
		}
		return nil, fmt.Errorf("opening index file %q: %w", filePath, err)
	}
	defer f.Close()

	var payload gobPayload
	if err := gob.NewDecoder(f).Decode(&payload); err != nil {
		return nil, fmt.Errorf("decoding index file %q: %w", filePath, err)
	}
	idx.vectors = payload.Vectors
	return idx, nil
}

// Add inserts or replaces the vector for the given id.
func (idx *FallbackIndex) Add(id string, vector []float32) error {
	if len(vector) == 0 {
		return fmt.Errorf("adding vector: empty vector for id %q", id)
	}
	idx.mu.Lock()
	defer idx.mu.Unlock()
	// Store a copy to prevent external mutation.
	cp := make([]float32, len(vector))
	copy(cp, vector)
	idx.vectors[id] = cp
	return nil
}

// Search returns up to k vectors most similar to query, ordered by cosine
// similarity descending.
func (idx *FallbackIndex) Search(query []float32, k int) ([]SearchHit, error) {
	if len(query) == 0 {
		return nil, fmt.Errorf("searching index: empty query vector")
	}
	if k <= 0 {
		return nil, nil
	}

	idx.mu.RLock()
	hits := make([]SearchHit, 0, len(idx.vectors))
	for id, vec := range idx.vectors {
		score := cosineSimilarity(query, vec)
		hits = append(hits, SearchHit{ID: id, Score: score})
	}
	idx.mu.RUnlock()

	sort.Slice(hits, func(i, j int) bool {
		return hits[i].Score > hits[j].Score
	})

	if k < len(hits) {
		hits = hits[:k]
	}
	return hits, nil
}

// Delete removes the vector with the given id. It is not an error if the id
// does not exist.
func (idx *FallbackIndex) Delete(id string) error {
	idx.mu.Lock()
	defer idx.mu.Unlock()
	delete(idx.vectors, id)
	return nil
}

// Close persists the index to filePath (if non-empty) and releases resources.
// The RLock is held for the entire encode operation to prevent data races.
func (idx *FallbackIndex) Close() error {
	if idx.filePath == "" {
		return nil
	}

	idx.mu.RLock()
	defer idx.mu.RUnlock()

	f, err := os.Create(idx.filePath)
	if err != nil {
		return fmt.Errorf("creating index file %q: %w", idx.filePath, err)
	}
	defer f.Close()

	payload := gobPayload{Vectors: idx.vectors}
	if err := gob.NewEncoder(f).Encode(payload); err != nil {
		return fmt.Errorf("encoding index to %q: %w", idx.filePath, err)
	}
	return nil
}
