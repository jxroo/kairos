package memory

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"go.uber.org/zap"
)

// SearchService performs semantic search over stored memories by combining
// vector similarity with decay scoring and importance weighting.
type SearchService struct {
	store    *Store
	embedder Embedder
	index    VectorIndex
	decayCfg DecayConfig
	logger   *zap.Logger
}

// SearchQuery controls what the SearchService looks for.
type SearchQuery struct {
	Query        string
	Limit        int
	Tags         []string
	MinRelevance float64
}

// SearchResult pairs a Memory with the scores that determined its ranking.
type SearchResult struct {
	Memory          Memory
	SimilarityScore float64
	FinalScore      float64
}

// NewSearchService constructs a SearchService with the given dependencies.
// All parameters are required; passing nil will cause panics at call sites.
// An optional DecayConfig can be provided; if none is given, DefaultDecayConfig
// is used.
func NewSearchService(store *Store, embedder Embedder, index VectorIndex, logger *zap.Logger, opts ...DecayConfig) *SearchService {
	cfg := DefaultDecayConfig()
	if len(opts) > 0 {
		cfg = opts[0]
	}
	return &SearchService{
		store:    store,
		embedder: embedder,
		index:    index,
		decayCfg: cfg,
		logger:   logger,
	}
}

// Search embeds the query text, finds the nearest neighbors in the vector
// index, enriches each candidate with metadata from the Store, applies tag
// filtering and decay scoring, and returns results ranked by final_score.
//
// final_score = similarity × decay_score × importance_weight
func (s *SearchService) Search(ctx context.Context, query SearchQuery) ([]SearchResult, error) {
	if query.Limit <= 0 {
		query.Limit = 10
	}

	// 1. Embed the query text.
	vec, err := s.embedder.Embed(ctx, query.Query)
	if err != nil {
		return nil, fmt.Errorf("embedding search query: %w", err)
	}

	// 2. Over-fetch candidates to leave room for tag filtering.
	k := query.Limit * 3
	hits, err := s.index.Search(vec, k)
	if err != nil {
		return nil, fmt.Errorf("searching vector index: %w", err)
	}
	if len(hits) == 0 {
		return nil, nil
	}

	// 3. Build a set of required tags for fast membership testing.
	tagFilter := make(map[string]struct{}, len(query.Tags))
	for _, t := range query.Tags {
		tagFilter[t] = struct{}{}
	}

	now := time.Now()

	// 4. Fetch metadata, filter, score.
	results := make([]SearchResult, 0, len(hits))
	for _, hit := range hits {
		mem, err := s.store.Get(ctx, hit.ID)
		if err != nil {
			// Memory may have been deleted since it was indexed; skip it.
			s.logger.Debug("skipping missing memory during search",
				zap.String("id", hit.ID), zap.Error(err))
			continue
		}

		// 4a. Tag filter — all required tags must be present.
		if len(tagFilter) > 0 {
			tagSet := make(map[string]struct{}, len(mem.Tags))
			for _, t := range mem.Tags {
				tagSet[t] = struct{}{}
			}
			match := true
			for required := range tagFilter {
				if _, ok := tagSet[required]; !ok {
					match = false
					break
				}
			}
			if !match {
				continue
			}
		}

		// 4b. Apply decay.
		decay := CalculateDecay(mem.AccessedAt, mem.DecayScore, s.decayCfg, now)

		// 4c. Compute final score: use the higher of vector similarity
		// and keyword match to compensate for hash-based embedder weakness
		// when queries are much shorter than memory content.
		similarity := float64(hit.Score)
		kwScore := keywordMatchScore(query.Query, mem.Content)
		effectiveSimilarity := similarity
		if kwScore > effectiveSimilarity {
			effectiveSimilarity = kwScore
		}
		finalScore := effectiveSimilarity * decay * mem.ImportanceWeight

		results = append(results, SearchResult{
			Memory:          *mem,
			SimilarityScore: similarity,
			FinalScore:      finalScore,
		})
	}

	// 5. Sort descending by final score.
	sort.Slice(results, func(i, j int) bool {
		return results[i].FinalScore > results[j].FinalScore
	})

	// 6. Filter by minimum relevance.
	if query.MinRelevance > 0 {
		cutoff := 0
		for cutoff < len(results) && results[cutoff].FinalScore >= query.MinRelevance {
			cutoff++
		}
		results = results[:cutoff]
	}

	// 7. Truncate to limit.
	if len(results) > query.Limit {
		results = results[:query.Limit]
	}

	// 8. Record access for every result returned to the caller.
	for i := range results {
		if err := s.store.RecordAccess(ctx, results[i].Memory.ID); err != nil {
			s.logger.Warn("failed to record memory access",
				zap.String("id", results[i].Memory.ID), zap.Error(err))
		}
	}

	s.logger.Debug("search complete",
		zap.String("query", query.Query),
		zap.Int("results", len(results)))

	return results, nil
}

// IndexMemory embeds the memory's content and adds the resulting vector to the
// VectorIndex under the memory's ID. Calling this again for the same ID
// replaces the existing entry.
func (s *SearchService) IndexMemory(ctx context.Context, mem *Memory) error {
	vec, err := s.embedder.Embed(ctx, mem.Content)
	if err != nil {
		return fmt.Errorf("embedding memory %q: %w", mem.ID, err)
	}
	if err := s.index.Add(mem.ID, vec); err != nil {
		return fmt.Errorf("adding memory %q to index: %w", mem.ID, err)
	}
	s.logger.Debug("memory indexed", zap.String("id", mem.ID))
	return nil
}

// RemoveFromIndex deletes the vector entry for the given memory ID from the
// VectorIndex. It is not an error if the ID does not exist in the index.
func (s *SearchService) RemoveFromIndex(ctx context.Context, id string) error {
	if err := s.index.Delete(id); err != nil {
		return fmt.Errorf("removing memory %q from index: %w", id, err)
	}
	s.logger.Debug("memory removed from index", zap.String("id", id))
	return nil
}

// keywordMatchScore returns the fraction of meaningful query words (3+ chars)
// that appear in content. The result is scaled by keywordBoostCeiling so that
// a perfect keyword match is worth 0.8 vector similarity — strong enough to
// surface the result but not overpowering true semantic matches.
const keywordBoostCeiling = 0.8

func keywordMatchScore(query, content string) float64 {
	queryWords := strings.Fields(strings.ToLower(query))
	contentLower := strings.ToLower(content)

	var meaningful, matches int
	for _, w := range queryWords {
		if len(w) < 3 {
			continue // skip stopwords/short words
		}
		meaningful++
		if strings.Contains(contentLower, w) {
			matches++
		}
	}
	if meaningful == 0 {
		return 0
	}
	return float64(matches) / float64(meaningful) * keywordBoostCeiling
}
