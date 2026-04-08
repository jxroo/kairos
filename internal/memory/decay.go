package memory

import (
	"context"
	"fmt"
	"math"
	"time"
)

// DecayConfig controls how memory decay is calculated and when memories are pruned.
type DecayConfig struct {
	Factor    float64 // daily multiplier, e.g. 0.95 means 5% decay per day
	Threshold float64 // memories below this score are pruned
}

// DefaultDecayConfig returns the canonical decay configuration.
func DefaultDecayConfig() DecayConfig {
	return DecayConfig{
		Factor:    0.95,
		Threshold: 0.01,
	}
}

// CalculateDecay computes the new decay score given how much time has elapsed
// since the memory was last accessed.
//
// Formula: new_score = current_score * (factor ^ days_since_access)
//
// If now is before accessedAt (negative elapsed time), the score is returned
// unchanged.
func CalculateDecay(accessedAt time.Time, currentScore float64, cfg DecayConfig, now time.Time) float64 {
	days := now.Sub(accessedAt).Hours() / 24
	if days <= 0 {
		return currentScore
	}
	return currentScore * math.Pow(cfg.Factor, days)
}

// ApplyDecay recalculates the decay score for each memory and returns only
// those whose new score is at or above cfg.Threshold. The returned slice
// contains updated DecayScore values. The input slice is modified in-place
// (DecayScore fields are updated) and a sub-slice is returned; callers should
// not rely on the original slice's contents after calling this function.
func ApplyDecay(memories []Memory, cfg DecayConfig, now time.Time) []Memory {
	writeIdx := 0
	for i := range memories {
		memories[i].DecayScore = CalculateDecay(memories[i].AccessedAt, memories[i].DecayScore, cfg, now)
		if memories[i].DecayScore >= cfg.Threshold {
			memories[writeIdx] = memories[i]
			writeIdx++
		}
	}
	return memories[:writeIdx]
}

// PruneDecayed updates the decay_score for every memory in the database, then
// deletes all rows whose score has fallen below cfg.Threshold. It returns the
// number of rows deleted.
func (s *Store) PruneDecayed(ctx context.Context, cfg DecayConfig) (int, error) {
	now := time.Now().UTC()

	// Load all memories so we can compute their current decay scores.
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, accessed_at, decay_score FROM memories`)
	if err != nil {
		return 0, fmt.Errorf("querying memories for decay: %w", err)
	}

	type row struct {
		id         string
		accessedAt string
		score      float64
	}
	var records []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.id, &r.accessedAt, &r.score); err != nil {
			rows.Close()
			return 0, fmt.Errorf("scanning memory row: %w", err)
		}
		records = append(records, r)
	}
	if err := rows.Close(); err != nil {
		return 0, fmt.Errorf("closing rows: %w", err)
	}
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("iterating memory rows: %w", err)
	}

	// Update decay scores and collect IDs that are below threshold.
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("beginning prune transaction: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	var belowThreshold []string
	for _, r := range records {
		accessedAt := parseTime(r.accessedAt)
		newScore := CalculateDecay(accessedAt, r.score, cfg, now)

		if _, err = tx.ExecContext(ctx,
			`UPDATE memories SET decay_score = ? WHERE id = ?`, newScore, r.id,
		); err != nil {
			return 0, fmt.Errorf("updating decay_score for %q: %w", r.id, err)
		}

		if newScore < cfg.Threshold {
			belowThreshold = append(belowThreshold, r.id)
		}
	}

	// Delete memories that fell below the threshold.
	deleted := 0
	for _, id := range belowThreshold {
		res, execErr := tx.ExecContext(ctx, `DELETE FROM memories WHERE id = ?`, id)
		if execErr != nil {
			err = fmt.Errorf("deleting decayed memory %q: %w", id, execErr)
			return 0, err
		}
		n, execErr := res.RowsAffected()
		if execErr != nil {
			err = fmt.Errorf("checking rows affected for %q: %w", id, execErr)
			return 0, err
		}
		deleted += int(n)
	}

	if err = tx.Commit(); err != nil {
		return 0, fmt.Errorf("committing prune transaction: %w", err)
	}

	s.logger.Sugar().Infof("pruned %d decayed memories", deleted)
	return deleted, nil
}
