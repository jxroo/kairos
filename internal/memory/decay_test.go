package memory

import (
	"context"
	"math"
	"testing"
	"time"
)

// TestCalculateDecay_ZeroDays verifies that a score of 1.0 accessed today
// (0 days elapsed) is returned unchanged.
func TestCalculateDecay_ZeroDays(t *testing.T) {
	cfg := DefaultDecayConfig()
	now := time.Now().UTC()
	got := CalculateDecay(now, 1.0, cfg, now)
	if got != 1.0 {
		t.Errorf("expected 1.0 with 0 days elapsed, got %f", got)
	}
}

// TestCalculateDecay_OneDay verifies that the score after 1 day is ~0.95.
func TestCalculateDecay_OneDay(t *testing.T) {
	cfg := DefaultDecayConfig()
	now := time.Now().UTC()
	accessedAt := now.Add(-24 * time.Hour)
	got := CalculateDecay(accessedAt, 1.0, cfg, now)
	want := 0.95
	if math.Abs(got-want) > 1e-9 {
		t.Errorf("expected ~%f after 1 day, got %f", want, got)
	}
}

// TestCalculateDecay_SevenDays verifies that 0.95^7 ≈ 0.698.
func TestCalculateDecay_SevenDays(t *testing.T) {
	cfg := DefaultDecayConfig()
	now := time.Now().UTC()
	accessedAt := now.Add(-7 * 24 * time.Hour)
	got := CalculateDecay(accessedAt, 1.0, cfg, now)
	want := math.Pow(0.95, 7) // ≈ 0.6983
	if math.Abs(got-want) > 1e-9 {
		t.Errorf("expected ~%f after 7 days, got %f", want, got)
	}
}

// TestCalculateDecay_ThirtyDays verifies that 0.95^30 ≈ 0.215.
func TestCalculateDecay_ThirtyDays(t *testing.T) {
	cfg := DefaultDecayConfig()
	now := time.Now().UTC()
	accessedAt := now.Add(-30 * 24 * time.Hour)
	got := CalculateDecay(accessedAt, 1.0, cfg, now)
	want := math.Pow(0.95, 30) // ≈ 0.2146
	if math.Abs(got-want) > 1e-9 {
		t.Errorf("expected ~%f after 30 days, got %f", want, got)
	}
}

// TestCalculateDecay_FutureTime verifies that a future accessedAt (negative
// days elapsed) leaves the score unchanged.
func TestCalculateDecay_FutureTime(t *testing.T) {
	cfg := DefaultDecayConfig()
	now := time.Now().UTC()
	future := now.Add(48 * time.Hour)
	got := CalculateDecay(future, 0.8, cfg, now)
	if got != 0.8 {
		t.Errorf("expected score unchanged for future accessedAt, got %f", got)
	}
}

// TestApplyDecay_PreservesRecentFiltersOld verifies that ApplyDecay keeps
// recent memories and removes those below threshold.
func TestApplyDecay_PreservesRecentFiltersOld(t *testing.T) {
	cfg := DefaultDecayConfig()
	now := time.Now().UTC()

	recent := Memory{
		ID:         "recent",
		Content:    "still fresh",
		AccessedAt: now.Add(-1 * 24 * time.Hour), // 1 day ago → score ~0.95
		DecayScore: 1.0,
	}
	old := Memory{
		ID:         "old",
		Content:    "very old",
		AccessedAt: now.Add(-90 * 24 * time.Hour), // 90 days ago → far below threshold
		DecayScore: 1.0,
	}

	result := ApplyDecay([]Memory{recent, old}, cfg, now)

	if len(result) != 1 {
		t.Fatalf("expected 1 memory after decay filter, got %d", len(result))
	}
	if result[0].ID != "recent" {
		t.Errorf("expected recent memory to survive, got ID=%q", result[0].ID)
	}
	if math.Abs(result[0].DecayScore-math.Pow(0.95, 1)) > 1e-9 {
		t.Errorf("decay score not updated: got %f", result[0].DecayScore)
	}
}

// TestApplyDecay_BelowThreshold verifies that a memory at ~90 days is excluded.
func TestApplyDecay_BelowThreshold(t *testing.T) {
	cfg := DefaultDecayConfig()
	now := time.Now().UTC()
	accessedAt := now.Add(-90 * 24 * time.Hour)

	score := CalculateDecay(accessedAt, 1.0, cfg, now)
	if score >= cfg.Threshold {
		t.Fatalf("expected score below threshold after 90 days, got %f (threshold %f)", score, cfg.Threshold)
	}

	mem := Memory{
		ID:         "ancient",
		Content:    "forgotten",
		AccessedAt: accessedAt,
		DecayScore: 1.0,
	}
	result := ApplyDecay([]Memory{mem}, cfg, now)
	if len(result) != 0 {
		t.Errorf("expected 0 memories after 90-day decay filter, got %d", len(result))
	}
}

// TestRecordAccess_ResetsDecayScore verifies that RecordAccess resets
// decay_score to 1.0 (reinforcement).
func TestRecordAccess_ResetsDecayScore(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	m, err := s.Create(ctx, CreateMemoryInput{Content: "reinforcement test"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Manually set a decayed score.
	if _, err := s.db.ExecContext(ctx,
		`UPDATE memories SET decay_score = 0.2 WHERE id = ?`, m.ID,
	); err != nil {
		t.Fatalf("setting decay_score: %v", err)
	}

	if err := s.RecordAccess(ctx, m.ID); err != nil {
		t.Fatalf("RecordAccess: %v", err)
	}

	got, err := s.Get(ctx, m.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.DecayScore != 1.0 {
		t.Errorf("expected decay_score=1.0 after RecordAccess, got %f", got.DecayScore)
	}
}

// TestPruneDecayed_RemovesOldMemories verifies that PruneDecayed deletes
// memories whose decayed score falls below the threshold.
func TestPruneDecayed_RemovesOldMemories(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	// Create a recent memory that should survive.
	recent, err := s.Create(ctx, CreateMemoryInput{Content: "recent"})
	if err != nil {
		t.Fatalf("Create recent: %v", err)
	}

	// Create and back-date an old memory that should be pruned.
	old, err := s.Create(ctx, CreateMemoryInput{Content: "old"})
	if err != nil {
		t.Fatalf("Create old: %v", err)
	}

	pastTime := time.Now().UTC().Add(-90 * 24 * time.Hour).Format(time.RFC3339)
	if _, err := s.db.ExecContext(ctx,
		`UPDATE memories SET accessed_at = ? WHERE id = ?`, pastTime, old.ID,
	); err != nil {
		t.Fatalf("back-dating accessed_at: %v", err)
	}

	cfg := DefaultDecayConfig()
	deleted, err := s.PruneDecayed(ctx, cfg)
	if err != nil {
		t.Fatalf("PruneDecayed: %v", err)
	}
	if deleted != 1 {
		t.Errorf("expected 1 deleted memory, got %d", deleted)
	}

	// Recent memory should still exist.
	if _, err := s.Get(ctx, recent.ID); err != nil {
		t.Errorf("recent memory should still exist: %v", err)
	}

	// Old memory should be gone.
	memories, err := s.List(ctx, ListOptions{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	for _, m := range memories {
		if m.ID == old.ID {
			t.Errorf("old memory %q should have been pruned", old.ID)
		}
	}
}

// BenchmarkApplyDecay verifies that ApplyDecay handles 10,000 memories in
// under 1ms.
func BenchmarkApplyDecay(b *testing.B) {
	cfg := DefaultDecayConfig()
	now := time.Now().UTC()

	memories := make([]Memory, 10_000)
	for i := range memories {
		memories[i] = Memory{
			ID:         "bench-id",
			Content:    "benchmark content",
			AccessedAt: now.Add(-time.Duration(i%30) * 24 * time.Hour),
			DecayScore: 1.0,
		}
	}

	b.ResetTimer()
	for range b.N {
		_ = ApplyDecay(memories, cfg, now)
	}
}
