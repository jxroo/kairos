package memory

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	_ "github.com/mattn/go-sqlite3"
	"go.uber.org/zap"
)

// Store provides CRUD operations for Memory records backed by SQLite.
type Store struct {
	db        *sql.DB
	logger    *zap.Logger
	extractor *EntityExtractor
	decayCfg  DecayConfig
}

// NewStore opens (or creates) the SQLite database at dbPath, runs all pending
// schema migrations, and returns a ready Store.
//
// The DSN enables WAL journal mode and foreign-key enforcement on every
// connection in the pool.
func NewStore(dbPath string, logger *zap.Logger) (*Store, error) {
	dsn := dbPath + "?_journal_mode=WAL&_foreign_keys=on"
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, fmt.Errorf("opening sqlite db: %w", err)
	}

	if err := Migrate(db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("running migrations: %w", err)
	}

	return &Store{db: db, logger: logger, extractor: NewEntityExtractor(logger), decayCfg: DefaultDecayConfig()}, nil
}

// DB returns the underlying *sql.DB for use by other packages that share this
// database (e.g. tool audit logging).
func (s *Store) DB() *sql.DB {
	return s.db
}

// SetDecayConfig overrides the default decay configuration used by List to
// filter fully-decayed memories.
func (s *Store) SetDecayConfig(cfg DecayConfig) {
	s.decayCfg = cfg
}

// Close releases the underlying database connection.
func (s *Store) Close() error {
	if err := s.db.Close(); err != nil {
		return fmt.Errorf("closing db: %w", err)
	}
	return nil
}

// importanceWeight returns the canonical weight for the given importance level.
func importanceWeight(importance string) float64 {
	switch importance {
	case "low":
		return 0.5
	case "high":
		return 2.0
	default:
		return 1.0
	}
}

// Create inserts a new Memory row (plus its tags) and returns the full record.
func (s *Store) Create(ctx context.Context, in CreateMemoryInput) (*Memory, error) {
	importance := in.Importance
	if importance == "" {
		importance = "normal"
	}

	weight := importanceWeight(importance)
	id := uuid.New().String()
	now := time.Now().UTC()

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("beginning create transaction: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	summary := nullableString(in.Context)

	_, err = tx.ExecContext(ctx, `
		INSERT INTO memories
			(id, content, summary, importance, importance_weight, conversation_id, source,
			 created_at, updated_at, accessed_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, in.Content, summary, importance, weight,
		nullableString(in.ConversationID), nullableString(in.Source),
		now.Format(time.RFC3339), now.Format(time.RFC3339), now.Format(time.RFC3339),
	)
	if err != nil {
		return nil, fmt.Errorf("inserting memory: %w", err)
	}

	if err = insertTags(ctx, tx, id, in.Tags); err != nil {
		return nil, err
	}

	entities := s.extractor.Extract(in.Content)
	if err = s.linkEntities(ctx, tx, id, entities); err != nil {
		return nil, err
	}

	if err = tx.Commit(); err != nil {
		return nil, fmt.Errorf("committing create transaction: %w", err)
	}

	s.logger.Debug("memory created", zap.String("id", id))
	return s.Get(ctx, id)
}

// Get retrieves a Memory by ID, loading its tags. Returns an error wrapping
// sql.ErrNoRows if the memory does not exist.
func (s *Store) Get(ctx context.Context, id string) (*Memory, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, content, COALESCE(summary,''), importance, importance_weight,
		       decay_score, COALESCE(conversation_id,''), COALESCE(source,''),
		       created_at, updated_at, accessed_at, access_count
		FROM memories WHERE id = ?`, id)

	m, err := scanMemory(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("memory %q not found: %w", id, err)
		}
		return nil, fmt.Errorf("scanning memory: %w", err)
	}

	tags, err := s.loadTags(ctx, id)
	if err != nil {
		return nil, err
	}
	m.Tags = tags

	return m, nil
}

// Update applies non-nil fields from in to the memory identified by id.
// Tags, if provided (even empty), replace all existing tags atomically.
func (s *Store) Update(ctx context.Context, id string, in UpdateMemoryInput) (*Memory, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("beginning update transaction: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	// Build a dynamic SET clause for the columns being updated.
	setClauses := []string{"updated_at = ?"}
	args := []interface{}{time.Now().UTC().Format(time.RFC3339)}

	if in.Content != nil {
		setClauses = append(setClauses, "content = ?")
		args = append(args, *in.Content)
	}
	if in.Importance != nil {
		setClauses = append(setClauses, "importance = ?", "importance_weight = ?")
		args = append(args, *in.Importance, importanceWeight(*in.Importance))
	}

	args = append(args, id)
	query := "UPDATE memories SET " + strings.Join(setClauses, ", ") + " WHERE id = ?"

	res, err := tx.ExecContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("updating memory: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return nil, fmt.Errorf("checking rows affected: %w", err)
	}
	if n == 0 {
		return nil, fmt.Errorf("memory %q not found: %w", id, sql.ErrNoRows)
	}

	// Replace tags only when the caller provided them.
	if in.Tags != nil {
		if _, err = tx.ExecContext(ctx, `DELETE FROM memory_tags WHERE memory_id = ?`, id); err != nil {
			return nil, fmt.Errorf("deleting old tags: %w", err)
		}
		if err = insertTags(ctx, tx, id, in.Tags); err != nil {
			return nil, err
		}
	}

	if err = tx.Commit(); err != nil {
		return nil, fmt.Errorf("committing update transaction: %w", err)
	}

	s.logger.Debug("memory updated", zap.String("id", id))
	return s.Get(ctx, id)
}

// Delete removes the memory and, via FK cascade, its tags and entity links.
func (s *Store) Delete(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM memories WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("deleting memory: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("checking rows affected: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("memory %q not found: %w", id, sql.ErrNoRows)
	}
	s.logger.Debug("memory deleted", zap.String("id", id))
	return nil
}

// List returns memories in descending creation order, with optional tag
// filtering and pagination.
func (s *Store) List(ctx context.Context, opts ListOptions) ([]Memory, error) {
	limit := opts.Limit
	if limit <= 0 {
		limit = 50
	}

	var (
		query string
		args  []interface{}
	)

	if len(opts.Tags) > 0 {
		placeholders := make([]string, len(opts.Tags))
		for i, tag := range opts.Tags {
			placeholders[i] = "?"
			args = append(args, tag)
		}
		query = fmt.Sprintf(`
			SELECT DISTINCT m.id, m.content, COALESCE(m.summary,''), m.importance,
			       m.importance_weight, m.decay_score,
			       COALESCE(m.conversation_id,''), COALESCE(m.source,''),
			       m.created_at, m.updated_at, m.accessed_at, m.access_count
			FROM memories m
			JOIN memory_tags mt ON mt.memory_id = m.id
			WHERE mt.tag IN (%s)
			ORDER BY m.created_at DESC
			LIMIT ? OFFSET ?`, strings.Join(placeholders, ","))
	} else {
		query = `
			SELECT id, content, COALESCE(summary,''), importance,
			       importance_weight, decay_score,
			       COALESCE(conversation_id,''), COALESCE(source,''),
			       created_at, updated_at, accessed_at, access_count
			FROM memories
			ORDER BY created_at DESC
			LIMIT ? OFFSET ?`
	}

	args = append(args, limit, opts.Offset)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("listing memories: %w", err)
	}
	defer rows.Close()

	now := time.Now()
	var memories []Memory
	for rows.Next() {
		m, err := scanMemory(rows)
		if err != nil {
			return nil, fmt.Errorf("scanning memory row: %w", err)
		}

		// Filter out fully-decayed memories.
		effectiveDecay := CalculateDecay(m.AccessedAt, m.DecayScore, s.decayCfg, now)
		if effectiveDecay < s.decayCfg.Threshold {
			continue
		}

		tags, err := s.loadTags(ctx, m.ID)
		if err != nil {
			return nil, err
		}
		m.Tags = tags
		memories = append(memories, *m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating memory rows: %w", err)
	}

	return memories, nil
}

// RecordAccess increments access_count, refreshes accessed_at, and resets
// decay_score to 1.0 for the given memory.
func (s *Store) RecordAccess(ctx context.Context, id string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := s.db.ExecContext(ctx, `
		UPDATE memories
		SET access_count = access_count + 1,
		    accessed_at  = ?,
		    decay_score  = 1.0
		WHERE id = ?`, now, id)
	if err != nil {
		return fmt.Errorf("recording access: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("checking rows affected: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("memory %q not found: %w", id, sql.ErrNoRows)
	}
	return nil
}

// ---- helpers ----------------------------------------------------------------

// scanner is the common interface for *sql.Row and *sql.Rows.
type scanner interface {
	Scan(dest ...interface{}) error
}

func scanMemory(s scanner) (*Memory, error) {
	var (
		m          Memory
		createdStr string
		updatedStr string
		accessedStr string
	)
	err := s.Scan(
		&m.ID, &m.Content, &m.Summary, &m.Importance, &m.ImportanceWeight,
		&m.DecayScore, &m.ConversationID, &m.Source,
		&createdStr, &updatedStr, &accessedStr, &m.AccessCount,
	)
	if err != nil {
		return nil, err
	}
	m.CreatedAt = parseTime(createdStr)
	m.UpdatedAt = parseTime(updatedStr)
	m.AccessedAt = parseTime(accessedStr)
	return &m, nil
}

func parseTime(s string) time.Time {
	for _, layout := range []string{time.RFC3339, "2006-01-02T15:04:05Z", "2006-01-02 15:04:05"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC()
		}
	}
	return time.Time{}
}

// insertTags inserts each tag for memoryID within the given transaction.
func insertTags(ctx context.Context, tx *sql.Tx, memoryID string, tags []string) error {
	for _, tag := range tags {
		if tag == "" {
			continue
		}
		_, err := tx.ExecContext(ctx,
			`INSERT OR IGNORE INTO memory_tags (memory_id, tag) VALUES (?, ?)`,
			memoryID, tag,
		)
		if err != nil {
			return fmt.Errorf("inserting tag %q: %w", tag, err)
		}
	}
	return nil
}

// loadTags returns all tags associated with memoryID.
func (s *Store) loadTags(ctx context.Context, memoryID string) ([]string, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT tag FROM memory_tags WHERE memory_id = ? ORDER BY tag`, memoryID)
	if err != nil {
		return nil, fmt.Errorf("loading tags for %q: %w", memoryID, err)
	}
	defer rows.Close()

	var tags []string
	for rows.Next() {
		var tag string
		if err := rows.Scan(&tag); err != nil {
			return nil, fmt.Errorf("scanning tag: %w", err)
		}
		tags = append(tags, tag)
	}
	return tags, rows.Err()
}

// nullableString converts an empty string to nil for nullable SQL columns.
func nullableString(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}
