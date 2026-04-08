// Package memory implements persistent conversation memory with semantic search.
package memory

import (
	"database/sql"
	"fmt"
)

// migration represents a single versioned schema migration.
type migration struct {
	version int
	sql     string
}

// migrations is the ordered list of all schema migrations.
// Each migration is applied exactly once, in version order.
var migrations = []migration{
	{
		version: 1,
		sql: `
CREATE TABLE schema_version (
    version    INTEGER NOT NULL,
    applied_at TEXT    NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE memories (
    id                TEXT    PRIMARY KEY,
    content           TEXT    NOT NULL,
    summary           TEXT,
    importance        TEXT    NOT NULL DEFAULT 'normal'
                              CHECK(importance IN ('low','normal','high')),
    importance_weight REAL    NOT NULL DEFAULT 1.0,
    created_at        TEXT    NOT NULL DEFAULT (datetime('now')),
    updated_at        TEXT    NOT NULL DEFAULT (datetime('now')),
    accessed_at       TEXT    NOT NULL DEFAULT (datetime('now')),
    access_count      INTEGER NOT NULL DEFAULT 0,
    decay_score       REAL    NOT NULL DEFAULT 1.0,
    conversation_id   TEXT,
    source            TEXT
);

CREATE TABLE memory_tags (
    memory_id TEXT NOT NULL REFERENCES memories(id) ON DELETE CASCADE,
    tag       TEXT NOT NULL,
    PRIMARY KEY (memory_id, tag)
);

CREATE TABLE entities (
    id           TEXT    PRIMARY KEY,
    name         TEXT    NOT NULL,
    entity_type  TEXT    NOT NULL,
    first_seen   TEXT    NOT NULL DEFAULT (datetime('now')),
    mention_count INTEGER NOT NULL DEFAULT 1
);

CREATE TABLE memory_entities (
    memory_id TEXT NOT NULL REFERENCES memories(id) ON DELETE CASCADE,
    entity_id TEXT NOT NULL REFERENCES entities(id) ON DELETE CASCADE,
    PRIMARY KEY (memory_id, entity_id)
);

CREATE INDEX idx_memories_created_at       ON memories(created_at);
CREATE INDEX idx_memories_accessed_at      ON memories(accessed_at);
CREATE INDEX idx_memories_decay_score      ON memories(decay_score);
CREATE INDEX idx_memories_conversation_id  ON memories(conversation_id);
CREATE INDEX idx_memory_tags_tag           ON memory_tags(tag);
CREATE INDEX idx_entities_name             ON entities(name);
CREATE INDEX idx_entities_entity_type      ON entities(entity_type);
`,
	},
	{
		version: 2,
		sql: `
CREATE TABLE documents (
    id         TEXT PRIMARY KEY,
    path       TEXT NOT NULL UNIQUE,
    filename   TEXT NOT NULL,
    extension  TEXT NOT NULL,
    size_bytes INTEGER NOT NULL,
    file_hash  TEXT NOT NULL,
    status     TEXT NOT NULL DEFAULT 'pending' CHECK(status IN ('pending','indexing','indexed','error')),
    error_msg  TEXT,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now')),
    indexed_at TEXT
);

CREATE TABLE chunks (
    id          TEXT PRIMARY KEY,
    document_id TEXT NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
    chunk_index INTEGER NOT NULL,
    content     TEXT NOT NULL,
    start_line  INTEGER,
    end_line    INTEGER,
    metadata    TEXT,
    created_at  TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX idx_documents_path       ON documents(path);
CREATE INDEX idx_documents_status     ON documents(status);
CREATE INDEX idx_documents_extension  ON documents(extension);
CREATE INDEX idx_chunks_document_id   ON chunks(document_id);
CREATE UNIQUE INDEX idx_chunks_doc_index ON chunks(document_id, chunk_index);
`,
	},
	{
		version: 3,
		sql: `
CREATE TABLE conversations (
    id         TEXT PRIMARY KEY,
    title      TEXT,
    model      TEXT,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE messages (
    id              TEXT PRIMARY KEY,
    conversation_id TEXT NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
    role            TEXT NOT NULL CHECK(role IN ('system','user','assistant')),
    content         TEXT NOT NULL,
    tokens          INTEGER,
    created_at      TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX idx_conversations_updated ON conversations(updated_at);
CREATE INDEX idx_messages_conv_id ON messages(conversation_id);
CREATE INDEX idx_messages_created ON messages(created_at);
`,
	},
	{
		version: 4,
		sql: `
CREATE TABLE tool_audit_log (
    id          TEXT PRIMARY KEY,
    tool_name   TEXT NOT NULL,
    arguments   TEXT,
    result      TEXT,
    is_error    INTEGER NOT NULL DEFAULT 0,
    duration_ms INTEGER NOT NULL,
    caller      TEXT,
    created_at  TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX idx_audit_tool_name  ON tool_audit_log(tool_name);
CREATE INDEX idx_audit_created_at ON tool_audit_log(created_at);
`,
	},
	{
		version: 5,
		sql: `
CREATE TABLE messages_new (
    id              TEXT PRIMARY KEY,
    conversation_id TEXT NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
    role            TEXT NOT NULL CHECK(role IN ('system','user','assistant','tool')),
    content         TEXT NOT NULL,
    tokens          INTEGER,
    metadata        TEXT DEFAULT '',
    created_at      TEXT NOT NULL DEFAULT (datetime('now'))
);

INSERT INTO messages_new (id, conversation_id, role, content, tokens, created_at)
SELECT id, conversation_id, role, content, tokens, created_at FROM messages;

DROP TABLE messages;
ALTER TABLE messages_new RENAME TO messages;

CREATE INDEX idx_messages_conv_id ON messages(conversation_id);
CREATE INDEX idx_messages_created ON messages(created_at);
`,
	},
}

// Migrate applies all pending migrations to db in order.
// It is safe to call multiple times — already-applied migrations are skipped.
// The database is configured with WAL journal mode and foreign keys enabled.
func Migrate(db *sql.DB) error {
	var mode string
	if err := db.QueryRow(`PRAGMA journal_mode=WAL`).Scan(&mode); err != nil {
		return fmt.Errorf("enabling WAL mode: %w", err)
	}
	if mode != "wal" {
		return fmt.Errorf("expected WAL journal mode, got %q", mode)
	}
	if _, err := db.Exec(`PRAGMA foreign_keys=ON`); err != nil {
		return fmt.Errorf("enabling foreign keys: %w", err)
	}

	current, err := currentVersion(db)
	if err != nil {
		return fmt.Errorf("reading schema version: %w", err)
	}

	for _, m := range migrations {
		if m.version <= current {
			continue
		}

		tx, err := db.Begin()
		if err != nil {
			return fmt.Errorf("beginning migration %d transaction: %w", m.version, err)
		}

		if _, err := tx.Exec(m.sql); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("applying migration %d: %w", m.version, err)
		}

		if _, err := tx.Exec(`INSERT INTO schema_version (version) VALUES (?)`, m.version); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("recording migration %d version: %w", m.version, err)
		}

		if err := tx.Commit(); err != nil {
			return fmt.Errorf("committing migration %d: %w", m.version, err)
		}
	}

	return nil
}

// currentVersion returns the highest migration version recorded in
// schema_version, or 0 if the table does not yet exist.
func currentVersion(db *sql.DB) (int, error) {
	// Check whether schema_version table exists.
	var count int
	err := db.QueryRow(
		`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='schema_version'`,
	).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("checking schema_version existence: %w", err)
	}
	if count == 0 {
		return 0, nil
	}

	var version int
	err = db.QueryRow(`SELECT COALESCE(MAX(version), 0) FROM schema_version`).Scan(&version)
	if err != nil {
		return 0, fmt.Errorf("querying max schema version: %w", err)
	}
	return version, nil
}
