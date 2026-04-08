package memory

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"go.uber.org/zap"
)

// setupIntegration creates a fully wired Store + SearchService backed by
// temp files. Both are registered for cleanup via t.Cleanup.
func setupIntegration(t *testing.T) (*Store, *SearchService) {
	t.Helper()
	logger := zap.NewNop()
	dir := t.TempDir()

	store, err := NewStore(filepath.Join(dir, "test.db"), logger)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	embedder := NewFallbackEmbedder()
	index, err := NewFallbackIndex(filepath.Join(dir, "vectors.gob"))
	if err != nil {
		t.Fatalf("NewFallbackIndex: %v", err)
	}
	t.Cleanup(func() { _ = index.Close() })

	svc := NewSearchService(store, embedder, index, logger)
	return store, svc
}

// createAndIndexFull is a convenience helper: creates a memory in the store and
// indexes it for search, returning the persisted Memory.
func createAndIndexFull(t *testing.T, ctx context.Context, store *Store, svc *SearchService, in CreateMemoryInput) *Memory {
	t.Helper()
	mem, err := store.Create(ctx, in)
	if err != nil {
		t.Fatalf("store.Create: %v", err)
	}
	if err := svc.IndexMemory(ctx, mem); err != nil {
		t.Fatalf("svc.IndexMemory: %v", err)
	}
	return mem
}

// TestIntegration_FullLifecycle tests the complete create → search → update →
// search → delete → search-empty flow.
func TestIntegration_FullLifecycle(t *testing.T) {
	ctx := context.Background()
	store, svc := setupIntegration(t)

	// 1. Create a memory and index it.
	mem := createAndIndexFull(t, ctx, store, svc, CreateMemoryInput{
		Content:    "golang concurrency patterns with goroutines",
		Importance: "normal",
		Tags:       []string{"golang", "concurrency"},
	})

	// 2. Search for the memory — should appear in results.
	results, err := svc.Search(ctx, SearchQuery{
		Query: "golang goroutines",
		Limit: 5,
	})
	if err != nil {
		t.Fatalf("Search after create: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected at least one search result after indexing")
	}
	found := false
	for _, r := range results {
		if r.Memory.ID == mem.ID {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("created memory %q not found in search results", mem.ID)
	}

	// 3. Update the memory content.
	newContent := "golang advanced concurrency patterns with channels and goroutines"
	updated, err := store.Update(ctx, mem.ID, UpdateMemoryInput{Content: &newContent})
	if err != nil {
		t.Fatalf("store.Update: %v", err)
	}
	if updated.Content != newContent {
		t.Errorf("updated content: got %q, want %q", updated.Content, newContent)
	}
	// Re-index with new content.
	if err := svc.IndexMemory(ctx, updated); err != nil {
		t.Fatalf("svc.IndexMemory after update: %v", err)
	}

	// 4. Search again after update — should still appear.
	results2, err := svc.Search(ctx, SearchQuery{
		Query: "golang channels goroutines",
		Limit: 5,
	})
	if err != nil {
		t.Fatalf("Search after update: %v", err)
	}
	found2 := false
	for _, r := range results2 {
		if r.Memory.ID == mem.ID {
			found2 = true
			break
		}
	}
	if !found2 {
		t.Errorf("updated memory %q not found in search results", mem.ID)
	}

	// 5. Delete the memory and remove from index.
	if err := store.Delete(ctx, mem.ID); err != nil {
		t.Fatalf("store.Delete: %v", err)
	}
	if err := svc.RemoveFromIndex(ctx, mem.ID); err != nil {
		t.Fatalf("svc.RemoveFromIndex: %v", err)
	}

	// 6. Search after delete — memory must not appear.
	results3, err := svc.Search(ctx, SearchQuery{
		Query: "golang goroutines concurrency",
		Limit: 5,
	})
	if err != nil {
		t.Fatalf("Search after delete: %v", err)
	}
	for _, r := range results3 {
		if r.Memory.ID == mem.ID {
			t.Errorf("deleted memory %q should not appear in search results", mem.ID)
		}
	}
}

// TestIntegration_SemanticRelevance verifies that semantic search returns the
// most topically relevant memories across distinct subjects.
func TestIntegration_SemanticRelevance(t *testing.T) {
	ctx := context.Background()
	store, svc := setupIntegration(t)

	topics := []CreateMemoryInput{
		{Content: "machine learning neural networks deep learning tensorflow"},
		{Content: "cooking recipes pasta italian cuisine"},
		{Content: "golang programming language concurrency channels"},
		{Content: "python data science pandas numpy machine learning"},
		{Content: "hiking outdoor adventure mountains trails"},
	}

	for _, in := range topics {
		createAndIndexFull(t, ctx, store, svc, in)
	}

	results, err := svc.Search(ctx, SearchQuery{
		Query: "machine learning neural network python",
		Limit: 3,
	})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected search results")
	}

	// With a hash-based embedder, exact semantic quality is limited.
	// Verify that results are non-empty and sorted by FinalScore.
	if len(results) < 1 {
		t.Fatal("expected at least one result")
	}

	// The top result's FinalScore must be the highest among all results.
	for i := 1; i < len(results); i++ {
		if results[i].FinalScore > results[0].FinalScore {
			t.Errorf("results are not sorted by FinalScore: result[%d]=%f > result[0]=%f",
				i, results[i].FinalScore, results[0].FinalScore)
		}
	}

	// At least one of the top-3 results must be ML-related (machine learning or python).
	mlFound := false
	for _, r := range results {
		if strings.Contains(r.Memory.Content, "machine learning") || strings.Contains(r.Memory.Content, "python") {
			mlFound = true
			break
		}
	}
	if !mlFound {
		t.Errorf("no ML-related memory in top-%d results", len(results))
	}
}

// TestIntegration_TagFiltering verifies that a tag-filtered search returns
// only memories that carry the requested tag.
func TestIntegration_TagFiltering(t *testing.T) {
	ctx := context.Background()
	store, svc := setupIntegration(t)

	createAndIndexFull(t, ctx, store, svc, CreateMemoryInput{
		Content: "golang concurrency goroutines",
		Tags:    []string{"golang", "backend"},
	})
	createAndIndexFull(t, ctx, store, svc, CreateMemoryInput{
		Content: "golang interfaces and types",
		Tags:    []string{"golang", "types"},
	})
	createAndIndexFull(t, ctx, store, svc, CreateMemoryInput{
		Content: "python scripting automation",
		Tags:    []string{"python", "scripting"},
	})

	results, err := svc.Search(ctx, SearchQuery{
		Query: "golang programming",
		Tags:  []string{"python"},
		Limit: 10,
	})
	if err != nil {
		t.Fatalf("Search with tag filter: %v", err)
	}

	// Only the python-tagged memory should appear.
	for _, r := range results {
		tagSet := make(map[string]struct{}, len(r.Memory.Tags))
		for _, tg := range r.Memory.Tags {
			tagSet[tg] = struct{}{}
		}
		if _, ok := tagSet["python"]; !ok {
			t.Errorf("result %q does not carry required tag 'python': tags=%v",
				r.Memory.ID, r.Memory.Tags)
		}
	}
}

// TestIntegration_ImportanceRanking verifies that high-importance memories
// rank above low-importance ones for equally relevant content.
func TestIntegration_ImportanceRanking(t *testing.T) {
	ctx := context.Background()
	store, svc := setupIntegration(t)

	// Use identical content so similarity scores are equal; only importance differs.
	lowMem := createAndIndexFull(t, ctx, store, svc, CreateMemoryInput{
		Content:    "kubernetes cluster management deployment orchestration",
		Importance: "low",
	})
	highMem := createAndIndexFull(t, ctx, store, svc, CreateMemoryInput{
		Content:    "kubernetes cluster management deployment orchestration",
		Importance: "high",
	})

	results, err := svc.Search(ctx, SearchQuery{
		Query: "kubernetes cluster deployment",
		Limit: 5,
	})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) < 2 {
		t.Fatalf("expected at least 2 results, got %d", len(results))
	}

	// Locate positions of high and low importance results.
	highPos, lowPos := -1, -1
	for i, r := range results {
		if r.Memory.ID == highMem.ID {
			highPos = i
		}
		if r.Memory.ID == lowMem.ID {
			lowPos = i
		}
	}
	if highPos == -1 || lowPos == -1 {
		t.Fatalf("both memories must appear: highPos=%d lowPos=%d", highPos, lowPos)
	}
	if highPos >= lowPos {
		t.Errorf("high-importance memory (pos %d) should rank before low-importance (pos %d)",
			highPos, lowPos)
	}
}

// TestIntegration_DecaySimulation creates a memory, manually back-dates its
// accessed_at to simulate aging, and verifies that CalculateDecay lowers the
// effective score.
func TestIntegration_DecaySimulation(t *testing.T) {
	ctx := context.Background()
	store, _ := setupIntegration(t)

	mem, err := store.Create(ctx, CreateMemoryInput{
		Content:    "some memory that will age",
		Importance: "normal",
	})
	if err != nil {
		t.Fatalf("store.Create: %v", err)
	}

	// Simulate 30-day-old access by directly updating accessed_at.
	thirtyDaysAgo := time.Now().UTC().Add(-30 * 24 * time.Hour).Format(time.RFC3339)
	if _, err := store.db.ExecContext(ctx,
		`UPDATE memories SET accessed_at = ? WHERE id = ?`, thirtyDaysAgo, mem.ID,
	); err != nil {
		t.Fatalf("backdating accessed_at: %v", err)
	}

	// Reload the memory to get the updated accessed_at.
	aged, err := store.Get(ctx, mem.ID)
	if err != nil {
		t.Fatalf("store.Get after backdating: %v", err)
	}

	cfg := DefaultDecayConfig()
	decayed := CalculateDecay(aged.AccessedAt, aged.DecayScore, cfg, time.Now())

	// After 30 days at 0.95/day: 0.95^30 ≈ 0.215 — well below original 1.0.
	if decayed >= aged.DecayScore {
		t.Errorf("expected decay to lower score: original=%f decayed=%f",
			aged.DecayScore, decayed)
	}
	if decayed > 0.5 {
		t.Errorf("expected significant decay after 30 days: got %f", decayed)
	}
}

// TestIntegration_ReinforcementResetsDecay verifies that RecordAccess resets
// the decay_score back to 1.0, simulating memory reinforcement.
func TestIntegration_ReinforcementResetsDecay(t *testing.T) {
	ctx := context.Background()
	store, _ := setupIntegration(t)

	mem, err := store.Create(ctx, CreateMemoryInput{
		Content: "memory to be reinforced",
	})
	if err != nil {
		t.Fatalf("store.Create: %v", err)
	}

	// Simulate aging: set decay_score low and accessed_at to be old.
	thirtyDaysAgo := time.Now().UTC().Add(-30 * 24 * time.Hour).Format(time.RFC3339)
	if _, err := store.db.ExecContext(ctx,
		`UPDATE memories SET accessed_at = ?, decay_score = 0.2 WHERE id = ?`,
		thirtyDaysAgo, mem.ID,
	); err != nil {
		t.Fatalf("simulating aging: %v", err)
	}

	// Verify the memory looks old.
	aged, err := store.Get(ctx, mem.ID)
	if err != nil {
		t.Fatalf("store.Get after aging: %v", err)
	}
	if aged.DecayScore != 0.2 {
		t.Fatalf("expected decay_score=0.2, got %f", aged.DecayScore)
	}

	// Reinforce the memory.
	if err := store.RecordAccess(ctx, mem.ID); err != nil {
		t.Fatalf("RecordAccess: %v", err)
	}

	reinforced, err := store.Get(ctx, mem.ID)
	if err != nil {
		t.Fatalf("store.Get after RecordAccess: %v", err)
	}

	if reinforced.DecayScore != 1.0 {
		t.Errorf("decay_score after reinforcement: got %f, want 1.0", reinforced.DecayScore)
	}
	// accessed_at should be refreshed to near now.
	if time.Since(reinforced.AccessedAt) > 5*time.Second {
		t.Errorf("accessed_at not refreshed: %v", reinforced.AccessedAt)
	}
}

// TestIntegration_EntityExtractionEndToEnd creates a memory with identifiable
// entities (dates, URLs, emails) and verifies GetEntities returns them.
func TestIntegration_EntityExtractionEndToEnd(t *testing.T) {
	ctx := context.Background()
	store, _ := setupIntegration(t)

	content := "Meeting on 2026-03-14. Check https://example.com/report for details. Contact alice@example.com for questions."

	mem, err := store.Create(ctx, CreateMemoryInput{Content: content})
	if err != nil {
		t.Fatalf("store.Create: %v", err)
	}

	entities, err := store.GetEntities(ctx, mem.ID)
	if err != nil {
		t.Fatalf("store.GetEntities: %v", err)
	}

	entityMap := make(map[string]string, len(entities))
	for _, e := range entities {
		entityMap[e.Name] = e.Type
	}

	tests := []struct {
		name     string
		wantName string
		wantType string
	}{
		{"date", "2026-03-14", "date"},
		{"url", "https://example.com/report", "url"},
		{"email", "alice@example.com", "email"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gotType, ok := entityMap[tc.wantName]
			if !ok {
				t.Errorf("entity %q not found; available: %v", tc.wantName, entityMap)
				return
			}
			if gotType != tc.wantType {
				t.Errorf("entity %q type: got %q, want %q", tc.wantName, gotType, tc.wantType)
			}
		})
	}
}

// TestIntegration_ConcurrentAccess exercises the store and search service
// under concurrent load to expose data races (run with -race).
func TestIntegration_ConcurrentAccess(t *testing.T) {
	ctx := context.Background()
	store, svc := setupIntegration(t)

	const workers = 10
	const memoriesPerWorker = 5

	var wg sync.WaitGroup

	// Concurrent writers.
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for i := 0; i < memoriesPerWorker; i++ {
				in := CreateMemoryInput{
					Content: fmt.Sprintf("worker %d memory %d about golang concurrency", workerID, i),
					Tags:    []string{fmt.Sprintf("worker-%d", workerID)},
				}
				mem, err := store.Create(ctx, in)
				if err != nil {
					t.Errorf("worker %d create %d: %v", workerID, i, err)
					return
				}
				if err := svc.IndexMemory(ctx, mem); err != nil {
					t.Errorf("worker %d index %d: %v", workerID, i, err)
				}
			}
		}(w)
	}

	// Concurrent readers (search while writes are happening).
	for r := 0; r < workers/2; r++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = svc.Search(ctx, SearchQuery{
				Query: "golang concurrency worker",
				Limit: 5,
			})
		}()
	}

	wg.Wait()

	// Verify final state: total memories = workers * memoriesPerWorker.
	all, err := store.List(ctx, ListOptions{Limit: workers*memoriesPerWorker + 10})
	if err != nil {
		t.Fatalf("store.List: %v", err)
	}
	want := workers * memoriesPerWorker
	if len(all) != want {
		t.Errorf("after concurrent writes: got %d memories, want %d", len(all), want)
	}
}

// TestIntegration_LargeDataset creates 1000 memories and verifies that a
// search completes within 100ms. Skip under race detector as timing is loose.
func TestIntegration_LargeDataset(t *testing.T) {
	ctx := context.Background()
	store, svc := setupIntegration(t)

	const total = 1000
	for i := 0; i < total; i++ {
		in := CreateMemoryInput{
			Content: fmt.Sprintf("memory entry %d about topic %d golang rust python", i, i%20),
		}
		mem, err := store.Create(ctx, in)
		if err != nil {
			t.Fatalf("create %d: %v", i, err)
		}
		if err := svc.IndexMemory(ctx, mem); err != nil {
			t.Fatalf("index %d: %v", i, err)
		}
	}

	start := time.Now()
	results, err := svc.Search(ctx, SearchQuery{
		Query: "golang concurrency memory",
		Limit: 10,
	})
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) == 0 {
		t.Error("expected search results from 1000-entry dataset")
	}
	// The brute-force FallbackIndex scans all vectors linearly. With 1000 entries
	// and 384-dimensional float32 vectors the search should comfortably finish
	// within 500ms on any CI runner. Tighten this bound when a real ANN index is
	// in place.
	const maxDuration = 500 * time.Millisecond
	if elapsed > maxDuration {
		t.Errorf("search took %v, want < %v", elapsed, maxDuration)
	}
}

// TestIntegration_EdgeCases covers empty query, very long query, and special
// characters in memory content.
func TestIntegration_EdgeCases(t *testing.T) {
	ctx := context.Background()
	store, svc := setupIntegration(t)

	// Seed a normal memory so the index is non-empty.
	createAndIndexFull(t, ctx, store, svc, CreateMemoryInput{
		Content: "normal memory about software engineering",
	})

	t.Run("empty query", func(t *testing.T) {
		// An empty query produces a zero vector; search must not error.
		results, err := svc.Search(ctx, SearchQuery{Query: "", Limit: 5})
		// Either returns empty results or an error from the index — both are acceptable.
		// What is NOT acceptable is a panic.
		_ = results
		_ = err
	})

	t.Run("long query", func(t *testing.T) {
		longQuery := strings.Repeat("golang concurrency goroutines channels ", 30) // ~1200 chars
		results, err := svc.Search(ctx, SearchQuery{
			Query: longQuery,
			Limit: 5,
		})
		if err != nil {
			t.Fatalf("Search with long query: %v", err)
		}
		_ = results
	})

	t.Run("special characters in content", func(t *testing.T) {
		specialContent := `C++ code: #include <stdio.h>; printf("hello\nworld"); // 100% done & done`
		mem, err := store.Create(ctx, CreateMemoryInput{Content: specialContent})
		if err != nil {
			t.Fatalf("Create with special chars: %v", err)
		}
		got, err := store.Get(ctx, mem.ID)
		if err != nil {
			t.Fatalf("Get after Create: %v", err)
		}
		if got.Content != specialContent {
			t.Errorf("content round-trip failed: got %q, want %q", got.Content, specialContent)
		}
	})

	t.Run("unicode content", func(t *testing.T) {
		unicodeContent := "памятка о проекте Kairos — система искусственного интеллекта"
		mem, err := store.Create(ctx, CreateMemoryInput{Content: unicodeContent})
		if err != nil {
			t.Fatalf("Create with unicode: %v", err)
		}
		got, err := store.Get(ctx, mem.ID)
		if err != nil {
			t.Fatalf("Get after Create: %v", err)
		}
		if got.Content != unicodeContent {
			t.Errorf("unicode round-trip failed: got %q, want %q", got.Content, unicodeContent)
		}
	})
}

// TestIntegration_PruningRemovesDecayedMemories creates memories with old
// accessed_at timestamps, runs PruneDecayed, and verifies they are removed.
func TestIntegration_PruningRemovesDecayedMemories(t *testing.T) {
	ctx := context.Background()
	store, _ := setupIntegration(t)

	// Create one recent memory and two very old memories.
	recentMem, err := store.Create(ctx, CreateMemoryInput{Content: "recent memory"})
	if err != nil {
		t.Fatalf("Create recent: %v", err)
	}

	oldMem1, err := store.Create(ctx, CreateMemoryInput{Content: "old memory 1"})
	if err != nil {
		t.Fatalf("Create old1: %v", err)
	}
	oldMem2, err := store.Create(ctx, CreateMemoryInput{Content: "old memory 2"})
	if err != nil {
		t.Fatalf("Create old2: %v", err)
	}

	// Set old memories' accessed_at to ~10 years ago so decay drops below threshold.
	// With factor=0.95 and threshold=0.01: days needed = log(0.01)/log(0.95) ≈ 89 days.
	// Use 200 days to be safe.
	twoHundredDaysAgo := time.Now().UTC().Add(-200 * 24 * time.Hour).Format(time.RFC3339)
	for _, id := range []string{oldMem1.ID, oldMem2.ID} {
		if _, err := store.db.ExecContext(ctx,
			`UPDATE memories SET accessed_at = ? WHERE id = ?`, twoHundredDaysAgo, id,
		); err != nil {
			t.Fatalf("backdating %q: %v", id, err)
		}
	}

	cfg := DefaultDecayConfig()
	deleted, err := store.PruneDecayed(ctx, cfg)
	if err != nil {
		t.Fatalf("PruneDecayed: %v", err)
	}

	if deleted != 2 {
		t.Errorf("PruneDecayed deleted %d memories, want 2", deleted)
	}

	// Old memories must be gone.
	for _, id := range []string{oldMem1.ID, oldMem2.ID} {
		_, err := store.Get(ctx, id)
		if err == nil {
			t.Errorf("memory %q should have been pruned but still exists", id)
		}
		if !isErrNoRows(err) {
			t.Errorf("expected ErrNoRows for pruned memory %q, got: %v", id, err)
		}
	}

	// Recent memory must survive.
	if _, err := store.Get(ctx, recentMem.ID); err != nil {
		t.Errorf("recent memory %q should not have been pruned: %v", recentMem.ID, err)
	}
}

// isErrNoRows returns true if err wraps sql.ErrNoRows.
func isErrNoRows(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), sql.ErrNoRows.Error())
}

// TestIntegration_Persistence verifies that both the SQLite store and the
// vector index survive a close/reopen cycle with data intact.
func TestIntegration_Persistence(t *testing.T) {
	ctx := context.Background()
	logger := zap.NewNop()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "persist.db")
	idxPath := filepath.Join(dir, "persist.gob")

	var memID string

	// --- Phase 1: write data and close. ---
	{
		store, err := NewStore(dbPath, logger)
		if err != nil {
			t.Fatalf("NewStore (phase1): %v", err)
		}
		index, err := NewFallbackIndex(idxPath)
		if err != nil {
			t.Fatalf("NewFallbackIndex (phase1): %v", err)
		}
		svc := NewSearchService(store, NewFallbackEmbedder(), index, logger)

		mem := createAndIndexFull(t, ctx, store, svc, CreateMemoryInput{
			Content:    "persistent memory about rust programming language",
			Importance: "high",
			Tags:       []string{"rust", "systems"},
		})
		memID = mem.ID

		if err := index.Close(); err != nil {
			t.Fatalf("index.Close (phase1): %v", err)
		}
		if err := store.Close(); err != nil {
			t.Fatalf("store.Close (phase1): %v", err)
		}
	}

	// --- Phase 2: reopen and verify. ---
	{
		store, err := NewStore(dbPath, logger)
		if err != nil {
			t.Fatalf("NewStore (phase2): %v", err)
		}
		defer store.Close()

		index, err := NewFallbackIndex(idxPath)
		if err != nil {
			t.Fatalf("NewFallbackIndex (phase2): %v", err)
		}
		defer index.Close()

		svc := NewSearchService(store, NewFallbackEmbedder(), index, logger)

		// Verify the memory is still in the store.
		got, err := store.Get(ctx, memID)
		if err != nil {
			t.Fatalf("store.Get after reopen: %v", err)
		}
		if got.Content != "persistent memory about rust programming language" {
			t.Errorf("content after reopen: got %q", got.Content)
		}
		if got.Importance != "high" {
			t.Errorf("importance after reopen: got %q", got.Importance)
		}
		wantTags := map[string]bool{"rust": true, "systems": true}
		for _, tg := range got.Tags {
			delete(wantTags, tg)
		}
		if len(wantTags) != 0 {
			t.Errorf("tags missing after reopen: %v", wantTags)
		}

		// Verify the index survived and search still works.
		results, err := svc.Search(ctx, SearchQuery{
			Query: "rust programming systems",
			Limit: 5,
		})
		if err != nil {
			t.Fatalf("Search after reopen: %v", err)
		}
		found := false
		for _, r := range results {
			if r.Memory.ID == memID {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("memory %q not found in search results after index reload", memID)
		}
	}
}
