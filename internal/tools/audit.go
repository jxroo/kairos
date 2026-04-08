package tools

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

const maxResultLen = 10 * 1024 // 10 KB

// AuditEntry records the execution of a tool.
type AuditEntry struct {
	ID         string    `json:"id"`
	ToolName   string    `json:"tool_name"`
	Arguments  string    `json:"arguments"`
	Result     string    `json:"result"`
	IsError    bool      `json:"is_error"`
	DurationMs int64     `json:"duration_ms"`
	Caller     string    `json:"caller"`
	CreatedAt  time.Time `json:"created_at"`
}

// AuditLogger persists tool execution audit entries to SQLite.
type AuditLogger struct {
	db     *sql.DB
	logger *zap.Logger
}

// NewAuditLogger creates an AuditLogger backed by the given database.
func NewAuditLogger(db *sql.DB, logger *zap.Logger) *AuditLogger {
	return &AuditLogger{db: db, logger: logger}
}

// Log inserts an audit entry. Large results are truncated to 10 KB.
func (a *AuditLogger) Log(ctx context.Context, entry AuditEntry) error {
	if entry.ID == "" {
		entry.ID = uuid.New().String()
	}
	result := entry.Result
	if len(result) > maxResultLen {
		result = result[:maxResultLen] + "...[truncated]"
	}

	_, err := a.db.ExecContext(ctx, `
		INSERT INTO tool_audit_log (id, tool_name, arguments, result, is_error, duration_ms, caller, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		entry.ID, entry.ToolName, entry.Arguments, result,
		boolToInt(entry.IsError), entry.DurationMs,
		entry.Caller, time.Now().UTC().Format(time.RFC3339),
	)
	if err != nil {
		return fmt.Errorf("inserting audit entry: %w", err)
	}
	return nil
}

// List returns the most recent audit entries, ordered by created_at descending.
func (a *AuditLogger) List(ctx context.Context, limit int) ([]AuditEntry, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := a.db.QueryContext(ctx, `
		SELECT id, tool_name, COALESCE(arguments,''), COALESCE(result,''),
		       is_error, duration_ms, COALESCE(caller,''), created_at
		FROM tool_audit_log
		ORDER BY created_at DESC
		LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("querying audit log: %w", err)
	}
	defer rows.Close()

	var entries []AuditEntry
	for rows.Next() {
		var (
			e          AuditEntry
			isErr      int
			createdStr string
		)
		if err := rows.Scan(&e.ID, &e.ToolName, &e.Arguments, &e.Result,
			&isErr, &e.DurationMs, &e.Caller, &createdStr); err != nil {
			return nil, fmt.Errorf("scanning audit entry: %w", err)
		}
		e.IsError = isErr != 0
		e.CreatedAt = parseTimeStr(createdStr)
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func parseTimeStr(s string) time.Time {
	for _, layout := range []string{time.RFC3339, "2006-01-02T15:04:05Z", "2006-01-02 15:04:05"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC()
		}
	}
	return time.Time{}
}
