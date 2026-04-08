package rag

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"go.uber.org/zap"

	"github.com/jxroo/kairos/internal/memory"
)

// RAGSearchService performs hybrid search (vector + keyword) over document chunks
// using Reciprocal Rank Fusion (RRF) to combine results.
type RAGSearchService struct {
	store    *Store
	embedder memory.Embedder
	vecIndex memory.VectorIndex
	bleve    *BleveWrapper
	logger   *zap.Logger
}

// RAGSearchQuery controls document search parameters.
type RAGSearchQuery struct {
	Query     string
	Limit     int
	FileTypes []string // filter by extension (e.g. ".md", ".go")
}

// NewRAGSearchService creates a RAGSearchService with the given dependencies.
func NewRAGSearchService(
	store *Store,
	embedder memory.Embedder,
	vecIndex memory.VectorIndex,
	bleve *BleveWrapper,
	logger *zap.Logger,
) *RAGSearchService {
	return &RAGSearchService{
		store:    store,
		embedder: embedder,
		vecIndex: vecIndex,
		bleve:    bleve,
		logger:   logger,
	}
}

// Search performs hybrid search using vector similarity and full-text keyword
// matching, combined via Reciprocal Rank Fusion.
func (s *RAGSearchService) Search(ctx context.Context, q RAGSearchQuery) ([]ChunkSearchResult, error) {
	if q.Limit <= 0 {
		q.Limit = 10
	}

	fetchK := q.Limit * 2

	// Build file type filter set.
	typeFilter := make(map[string]struct{}, len(q.FileTypes))
	for _, ft := range q.FileTypes {
		typeFilter[strings.ToLower(ft)] = struct{}{}
	}

	// 1. Vector search.
	vecRanking := make(map[string]int) // chunk ID → rank (1-based)
	vec, err := s.embedder.Embed(ctx, q.Query)
	if err != nil {
		return nil, fmt.Errorf("embedding search query: %w", err)
	}
	vecHits, err := s.vecIndex.Search(vec, fetchK)
	if err != nil {
		return nil, fmt.Errorf("vector search: %w", err)
	}
	for i, h := range vecHits {
		vecRanking[h.ID] = i + 1
	}

	// 2. Bleve (keyword) search.
	textRanking := make(map[string]int)
	if s.bleve != nil {
		bleveHits, err := s.bleve.Search(q.Query, fetchK)
		if err != nil {
			s.logger.Warn("bleve search failed, using vector-only", zap.Error(err))
		} else {
			for i, h := range bleveHits {
				textRanking[h.ChunkID] = i + 1
			}
		}
	}

	// 3. Merge candidates.
	candidates := make(map[string]struct{})
	for id := range vecRanking {
		candidates[id] = struct{}{}
	}
	for id := range textRanking {
		candidates[id] = struct{}{}
	}

	if len(candidates) == 0 {
		return nil, nil
	}

	// 4. RRF scoring.
	const k = 60 // RRF constant
	type scored struct {
		chunkID string
		score   float64
	}
	var scoredItems []scored

	for id := range candidates {
		var score float64
		if rank, ok := vecRanking[id]; ok {
			score += 0.6 / float64(k+rank)
		}
		if rank, ok := textRanking[id]; ok {
			score += 0.4 / float64(k+rank)
		}
		scoredItems = append(scoredItems, scored{chunkID: id, score: score})
	}

	sort.Slice(scoredItems, func(i, j int) bool {
		return scoredItems[i].score > scoredItems[j].score
	})

	// 5. Enrich with chunk + document data, apply file type filter.
	var results []ChunkSearchResult
	for _, item := range scoredItems {
		if len(results) >= q.Limit {
			break
		}

		chunk, err := s.store.GetChunk(ctx, item.chunkID)
		if err != nil {
			s.logger.Debug("skipping missing chunk", zap.String("id", item.chunkID))
			continue
		}

		doc, err := s.store.GetDocument(ctx, chunk.DocumentID)
		if err != nil {
			s.logger.Debug("skipping chunk with missing document", zap.String("id", item.chunkID))
			continue
		}

		// Apply file type filter.
		if len(typeFilter) > 0 {
			if _, ok := typeFilter[strings.ToLower(doc.Extension)]; !ok {
				continue
			}
		}

		results = append(results, ChunkSearchResult{
			Chunk:      *chunk,
			Document:   *doc,
			FinalScore: item.score,
		})
	}

	s.logger.Debug("RAG search complete",
		zap.String("query", q.Query),
		zap.Int("results", len(results)))

	return results, nil
}
