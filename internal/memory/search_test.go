package memory

import (
	"context"
	"testing"

	"go.uber.org/zap"
)

// TestKeywordMatchScore verifies the keyword scoring helper.
func TestKeywordMatchScore(t *testing.T) {
	tests := []struct {
		name    string
		query   string
		content string
		wantMin float64
		wantMax float64
	}{
		{
			name:    "all words match",
			query:   "dentist appointment",
			content: "Dentist appointment on Thursday at 2:30 PM",
			wantMin: 0.79, // 2/2 * 0.8 = 0.8
			wantMax: 0.81,
		},
		{
			name:    "partial match",
			query:   "dentist insurance cost",
			content: "Dentist appointment on Thursday",
			wantMin: 0.25, // 1/3 * 0.8 ≈ 0.267
			wantMax: 0.28,
		},
		{
			name:    "no match",
			query:   "python programming",
			content: "Dentist appointment on Thursday",
			wantMin: 0,
			wantMax: 0.01,
		},
		{
			name:    "short words ignored",
			query:   "is it on",
			content: "This is it on the table",
			wantMin: 0,
			wantMax: 0.01, // all words < 3 chars
		},
		{
			name:    "empty query",
			query:   "",
			content: "some content",
			wantMin: 0,
			wantMax: 0.01,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := keywordMatchScore(tc.query, tc.content)
			if got < tc.wantMin || got > tc.wantMax {
				t.Errorf("keywordMatchScore(%q, %q) = %f, want in [%f, %f]",
					tc.query, tc.content, got, tc.wantMin, tc.wantMax)
			}
		})
	}
}

// TestSearch_KeywordBoostSurfacesExactMatch verifies that a memory containing
// the exact query words is found even when the hash-based embedder produces
// low vector similarity (long memory content, short query).
func TestSearch_KeywordBoostSurfacesExactMatch(t *testing.T) {
	store, svc := newTestSearchService(t)
	ctx := context.Background()

	createAndIndex(t, store, svc, CreateMemoryInput{
		Content: "Dentist appointment on Thursday at 2:30 PM with Dr. Kowalski. Need to leave the office by 2:00 to make it on time. Remember to bring the referral letter from Dr. Nowak.",
	})
	createAndIndex(t, store, svc, CreateMemoryInput{
		Content: "banana apple mango tropical fruit salad recipe",
	})

	results, err := svc.Search(ctx, SearchQuery{
		Query:        "dentist appointment",
		Limit:        5,
		MinRelevance: 0.3,
	})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected dentist memory to be found via keyword boost")
	}
	if results[0].FinalScore < 0.3 {
		t.Errorf("top result FinalScore = %f, want >= 0.3", results[0].FinalScore)
	}
}

// newTestSearchService creates a Store + SearchService wired to FallbackEmbedder
// and FallbackIndex (no Rust dependency required).
func newTestSearchService(t *testing.T) (*Store, *SearchService) {
	t.Helper()
	store := newTestStore(t)
	embedder := NewFallbackEmbedder()
	index, err := NewFallbackIndex("")
	if err != nil {
		t.Fatalf("NewFallbackIndex: %v", err)
	}
	svc := NewSearchService(store, embedder, index, zap.NewNop())
	return store, svc
}

// createAndIndex is a convenience wrapper: Create a memory in the Store and
// immediately add it to the SearchService index.
func createAndIndex(t *testing.T, store *Store, svc *SearchService, in CreateMemoryInput) *Memory {
	t.Helper()
	ctx := context.Background()
	mem, err := store.Create(ctx, in)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := svc.IndexMemory(ctx, mem); err != nil {
		t.Fatalf("IndexMemory: %v", err)
	}
	return mem
}

// TestSearch_ReturnsRelevantMemories verifies the basic create→index→search
// flow returns the expected memory.
func TestSearch_ReturnsRelevantMemories(t *testing.T) {
	store, svc := newTestSearchService(t)
	ctx := context.Background()

	createAndIndex(t, store, svc, CreateMemoryInput{
		Content: "machine learning and neural networks",
	})
	createAndIndex(t, store, svc, CreateMemoryInput{
		Content: "banana apple orange fruit",
	})

	results, err := svc.Search(ctx, SearchQuery{
		Query: "deep learning neural network",
		Limit: 5,
	})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected at least one result")
	}
	// The ML memory should rank first.
	if results[0].Memory.Content != "machine learning and neural networks" {
		t.Errorf("top result content = %q, want machine learning and neural networks",
			results[0].Memory.Content)
	}
}

// TestSearch_ImportanceRanking verifies that a high-importance memory outranks
// a normal-importance memory with the same query.
func TestSearch_ImportanceRanking(t *testing.T) {
	store, svc := newTestSearchService(t)
	ctx := context.Background()

	// Both memories have very similar content so similarity will be close.
	// High importance should push its final score higher.
	normal := createAndIndex(t, store, svc, CreateMemoryInput{
		Content:    "go programming language concurrency",
		Importance: "normal",
	})
	high := createAndIndex(t, store, svc, CreateMemoryInput{
		Content:    "go programming language concurrency goroutines",
		Importance: "high",
	})

	results, err := svc.Search(ctx, SearchQuery{
		Query: "go programming concurrency",
		Limit: 5,
	})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) < 2 {
		t.Fatalf("expected at least 2 results, got %d", len(results))
	}

	// Find positions of the two memories.
	pos := map[string]int{}
	for i, r := range results {
		pos[r.Memory.ID] = i
	}

	normalPos, normalFound := pos[normal.ID]
	highPos, highFound := pos[high.ID]

	if !normalFound || !highFound {
		t.Fatalf("one of the memories not found in results: normal=%v high=%v", normalFound, highFound)
	}
	if highPos >= normalPos {
		t.Errorf("high-importance memory (pos %d) should outrank normal-importance (pos %d)",
			highPos, normalPos)
	}
}

// TestSearch_TagFilter verifies that only memories carrying all required tags
// are returned.
func TestSearch_TagFilter(t *testing.T) {
	store, svc := newTestSearchService(t)
	ctx := context.Background()

	createAndIndex(t, store, svc, CreateMemoryInput{
		Content: "golang concurrency patterns",
		Tags:    []string{"go", "concurrency"},
	})
	createAndIndex(t, store, svc, CreateMemoryInput{
		Content: "golang http server patterns",
		Tags:    []string{"go", "http"},
	})
	createAndIndex(t, store, svc, CreateMemoryInput{
		Content: "python concurrency asyncio",
		Tags:    []string{"python", "concurrency"},
	})

	results, err := svc.Search(ctx, SearchQuery{
		Query: "concurrency patterns",
		Limit: 10,
		Tags:  []string{"go"},
	})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}

	for _, r := range results {
		hasTag := false
		for _, tg := range r.Memory.Tags {
			if tg == "go" {
				hasTag = true
				break
			}
		}
		if !hasTag {
			t.Errorf("result %q lacks required tag 'go'", r.Memory.Content)
		}
	}

	// Exactly 2 memories carry the "go" tag.
	if len(results) != 2 {
		t.Errorf("expected 2 results with tag 'go', got %d", len(results))
	}
}

// TestSearch_MinRelevanceFilter verifies that results below MinRelevance are
// excluded.
func TestSearch_MinRelevanceFilter(t *testing.T) {
	store, svc := newTestSearchService(t)
	ctx := context.Background()

	createAndIndex(t, store, svc, CreateMemoryInput{
		Content: "machine learning deep learning neural networks",
	})
	createAndIndex(t, store, svc, CreateMemoryInput{
		Content: "banana apple mango tropical fruit salad",
	})

	// With a high min relevance only the closely related memory should survive.
	results, err := svc.Search(ctx, SearchQuery{
		Query:        "deep learning neural",
		Limit:        10,
		MinRelevance: 0.5,
	})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}

	for _, r := range results {
		if r.FinalScore < 0.5 {
			t.Errorf("result %q has FinalScore %f < MinRelevance 0.5",
				r.Memory.Content, r.FinalScore)
		}
	}
}

// TestSearch_EmptyIndex verifies that searching an empty index returns no
// results without error.
func TestSearch_EmptyIndex(t *testing.T) {
	_, svc := newTestSearchService(t)
	ctx := context.Background()

	results, err := svc.Search(ctx, SearchQuery{
		Query: "anything",
		Limit: 5,
	})
	if err != nil {
		t.Fatalf("Search on empty index returned error: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results on empty index, got %d", len(results))
	}
}

// TestSearch_RecordsAccess verifies that Search calls RecordAccess for each
// returned result (access_count should be incremented).
func TestSearch_RecordsAccess(t *testing.T) {
	store, svc := newTestSearchService(t)
	ctx := context.Background()

	mem := createAndIndex(t, store, svc, CreateMemoryInput{
		Content: "important information to remember",
	})

	// Baseline: access_count should be 0 after creation.
	before, err := store.Get(ctx, mem.ID)
	if err != nil {
		t.Fatalf("Get before search: %v", err)
	}
	if before.AccessCount != 0 {
		t.Fatalf("initial access_count should be 0, got %d", before.AccessCount)
	}

	_, err = svc.Search(ctx, SearchQuery{
		Query: "important information",
		Limit: 5,
	})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}

	after, err := store.Get(ctx, mem.ID)
	if err != nil {
		t.Fatalf("Get after search: %v", err)
	}
	if after.AccessCount != 1 {
		t.Errorf("access_count after search: got %d, want 1", after.AccessCount)
	}
}

// TestSearch_LimitRespected verifies that Search never returns more results
// than the requested limit.
func TestSearch_LimitRespected(t *testing.T) {
	store, svc := newTestSearchService(t)
	ctx := context.Background()

	for i := 0; i < 10; i++ {
		createAndIndex(t, store, svc, CreateMemoryInput{
			Content: "golang programming language systems",
		})
	}

	const limit = 3
	results, err := svc.Search(ctx, SearchQuery{
		Query: "golang programming",
		Limit: limit,
	})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) > limit {
		t.Errorf("got %d results, want at most %d", len(results), limit)
	}
}

// TestSearch_SortedByFinalScore verifies that results are returned in
// descending final_score order.
func TestSearch_SortedByFinalScore(t *testing.T) {
	store, svc := newTestSearchService(t)
	ctx := context.Background()

	// Create memories with differing importance to produce spread in scores.
	createAndIndex(t, store, svc, CreateMemoryInput{
		Content:    "rust programming language memory safety",
		Importance: "low",
	})
	createAndIndex(t, store, svc, CreateMemoryInput{
		Content:    "rust programming language memory safety systems",
		Importance: "high",
	})
	createAndIndex(t, store, svc, CreateMemoryInput{
		Content:    "rust programming systems language",
		Importance: "normal",
	})

	results, err := svc.Search(ctx, SearchQuery{
		Query: "rust programming memory",
		Limit: 10,
	})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) < 2 {
		t.Fatalf("expected at least 2 results, got %d", len(results))
	}

	for i := 1; i < len(results); i++ {
		if results[i].FinalScore > results[i-1].FinalScore {
			t.Errorf("results not sorted: index %d score %f > index %d score %f",
				i, results[i].FinalScore, i-1, results[i-1].FinalScore)
		}
	}
}
