package rag

import (
	"fmt"

	"github.com/blevesearch/bleve/v2"
)

// bleveDoc is the structure indexed into Bleve.
type bleveDoc struct {
	Content    string `json:"content"`
	DocumentID string `json:"document_id"`
	Path       string `json:"path"`
	Extension  string `json:"extension"`
}

// BleveHit represents a single full-text search result.
type BleveHit struct {
	ChunkID string
	Score   float64
}

// BleveWrapper wraps a bleve.Index for full-text search of document chunks.
type BleveWrapper struct {
	index bleve.Index
}

// OpenOrCreateBleve opens an existing Bleve index or creates a new one at path.
func OpenOrCreateBleve(path string) (*BleveWrapper, error) {
	idx, err := bleve.Open(path)
	if err == bleve.ErrorIndexPathDoesNotExist {
		mapping := bleve.NewIndexMapping()
		idx, err = bleve.New(path, mapping)
		if err != nil {
			return nil, fmt.Errorf("creating bleve index: %w", err)
		}
		return &BleveWrapper{index: idx}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("opening bleve index: %w", err)
	}
	return &BleveWrapper{index: idx}, nil
}

// IndexChunk indexes a chunk's content with document metadata.
func (b *BleveWrapper) IndexChunk(chunk Chunk, docPath string) error {
	doc := bleveDoc{
		Content:    chunk.Content,
		DocumentID: chunk.DocumentID,
		Path:       docPath,
		Extension:  "",
	}
	if err := b.index.Index(chunk.ID, doc); err != nil {
		return fmt.Errorf("indexing chunk in bleve: %w", err)
	}
	return nil
}

// RemoveChunk removes a chunk from the Bleve index by ID.
func (b *BleveWrapper) RemoveChunk(chunkID string) error {
	if err := b.index.Delete(chunkID); err != nil {
		return fmt.Errorf("removing chunk from bleve: %w", err)
	}
	return nil
}

// RemoveByDocumentID is a convenience method; however since Bleve doesn't support
// field-based deletion natively, individual chunk removal should be used instead.
// This is kept for the interface but callers should use RemoveChunk per chunk ID.
func (b *BleveWrapper) RemoveByDocumentID(docID string) error {
	// Bleve doesn't support batch-delete by field value efficiently.
	// Callers should iterate chunk IDs and call RemoveChunk.
	return nil
}

// Search performs a full-text search query and returns up to limit results.
func (b *BleveWrapper) Search(query string, limit int) ([]BleveHit, error) {
	if limit <= 0 {
		limit = 10
	}

	q := bleve.NewMatchQuery(query)
	req := bleve.NewSearchRequestOptions(q, limit, 0, false)
	result, err := b.index.Search(req)
	if err != nil {
		return nil, fmt.Errorf("bleve search: %w", err)
	}

	hits := make([]BleveHit, 0, len(result.Hits))
	for _, h := range result.Hits {
		hits = append(hits, BleveHit{
			ChunkID: h.ID,
			Score:   h.Score,
		})
	}
	return hits, nil
}

// Close closes the underlying Bleve index.
func (b *BleveWrapper) Close() error {
	return b.index.Close()
}
