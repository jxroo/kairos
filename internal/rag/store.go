package rag

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	_ "github.com/mattn/go-sqlite3"
	"go.uber.org/zap"

	"github.com/jxroo/kairos/internal/memory"
)

// Store provides CRUD operations for Document and Chunk records backed by SQLite.
type Store struct {
	db     *sql.DB
	logger *zap.Logger
}

// NewStore opens the SQLite database at dbPath, ensures schema is current via
// memory.Migrate, and returns a ready Store.
func NewStore(dbPath string, logger *zap.Logger) (*Store, error) {
	dsn := dbPath + "?_journal_mode=WAL&_foreign_keys=on"
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, fmt.Errorf("opening sqlite db: %w", err)
	}

	if err := memory.Migrate(db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("running migrations: %w", err)
	}

	return &Store{db: db, logger: logger}, nil
}

// Close releases the underlying database connection.
func (s *Store) Close() error {
	return s.db.Close()
}

// CreateDocument inserts a new document record.
func (s *Store) CreateDocument(ctx context.Context, doc *Document) error {
	if doc.ID == "" {
		doc.ID = uuid.New().String()
	}
	now := time.Now().UTC().Format(time.RFC3339)

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO documents (id, path, filename, extension, size_bytes, file_hash, status, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		doc.ID, doc.Path, doc.Filename, doc.Extension, doc.SizeBytes, doc.FileHash,
		string(doc.Status), now, now,
	)
	if err != nil {
		return fmt.Errorf("inserting document: %w", err)
	}
	doc.CreatedAt = parseTime(now)
	doc.UpdatedAt = parseTime(now)
	return nil
}

// GetDocumentByPath retrieves a document by its file path.
func (s *Store) GetDocumentByPath(ctx context.Context, path string) (*Document, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, path, filename, extension, size_bytes, file_hash, status,
		       COALESCE(error_msg,''), created_at, updated_at, indexed_at
		FROM documents WHERE path = ?`, path)
	return scanDocument(row)
}

// GetDocument retrieves a document by ID.
func (s *Store) GetDocument(ctx context.Context, id string) (*Document, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, path, filename, extension, size_bytes, file_hash, status,
		       COALESCE(error_msg,''), created_at, updated_at, indexed_at
		FROM documents WHERE id = ?`, id)
	return scanDocument(row)
}

// UpdateDocumentStatus updates a document's status and optional error message.
func (s *Store) UpdateDocumentStatus(ctx context.Context, id string, status DocumentStatus, errMsg string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	var indexedAt interface{}
	if status == StatusIndexed {
		indexedAt = now
	}

	_, err := s.db.ExecContext(ctx, `
		UPDATE documents SET status = ?, error_msg = ?, updated_at = ?, indexed_at = COALESCE(?, indexed_at)
		WHERE id = ?`,
		string(status), nullableString(errMsg), now, indexedAt, id,
	)
	if err != nil {
		return fmt.Errorf("updating document status: %w", err)
	}
	return nil
}

// DeleteDocument removes a document and its chunks (via FK cascade).
func (s *Store) DeleteDocument(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM documents WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("deleting document: %w", err)
	}
	return nil
}

// DeleteDocumentByPath removes a document by path and returns the deleted doc's ID.
// Returns empty string if no document existed at that path.
func (s *Store) DeleteDocumentByPath(ctx context.Context, path string) (string, error) {
	doc, err := s.GetDocumentByPath(ctx, path)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", nil
		}
		return "", err
	}
	if err := s.DeleteDocument(ctx, doc.ID); err != nil {
		return "", err
	}
	return doc.ID, nil
}

// CreateChunks batch-inserts chunks for a document.
func (s *Store) CreateChunks(ctx context.Context, chunks []Chunk) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("beginning chunk transaction: %w", err)
	}

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO chunks (id, document_id, chunk_index, content, start_line, end_line, metadata)
		VALUES (?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("preparing chunk insert: %w", err)
	}
	defer stmt.Close()

	for i := range chunks {
		c := &chunks[i]
		if c.ID == "" {
			c.ID = uuid.New().String()
		}
		_, err := stmt.ExecContext(ctx, c.ID, c.DocumentID, c.ChunkIndex, c.Content,
			c.StartLine, c.EndLine, nullableString(c.Metadata))
		if err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("inserting chunk %d: %w", i, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing chunk transaction: %w", err)
	}
	return nil
}

// GetChunksByDocumentID returns all chunks for a document, ordered by chunk_index.
func (s *Store) GetChunksByDocumentID(ctx context.Context, docID string) ([]Chunk, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, document_id, chunk_index, content, start_line, end_line,
		       COALESCE(metadata,''), created_at
		FROM chunks WHERE document_id = ? ORDER BY chunk_index`, docID)
	if err != nil {
		return nil, fmt.Errorf("querying chunks: %w", err)
	}
	defer rows.Close()

	var chunks []Chunk
	for rows.Next() {
		c, err := scanChunk(rows)
		if err != nil {
			return nil, err
		}
		chunks = append(chunks, *c)
	}
	return chunks, rows.Err()
}

// GetChunk retrieves a single chunk by ID.
func (s *Store) GetChunk(ctx context.Context, id string) (*Chunk, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, document_id, chunk_index, content, start_line, end_line,
		       COALESCE(metadata,''), created_at
		FROM chunks WHERE id = ?`, id)
	return scanChunk(row)
}

// DeleteChunksByDocumentID removes all chunks for a document.
func (s *Store) DeleteChunksByDocumentID(ctx context.Context, docID string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM chunks WHERE document_id = ?`, docID)
	if err != nil {
		return fmt.Errorf("deleting chunks: %w", err)
	}
	return nil
}

// GetDocumentStats returns aggregate counts of documents by status.
func (s *Store) GetDocumentStats(ctx context.Context) (map[DocumentStatus]int, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT status, COUNT(*) FROM documents GROUP BY status`)
	if err != nil {
		return nil, fmt.Errorf("querying document stats: %w", err)
	}
	defer rows.Close()

	stats := make(map[DocumentStatus]int)
	for rows.Next() {
		var status string
		var count int
		if err := rows.Scan(&status, &count); err != nil {
			return nil, fmt.Errorf("scanning stats: %w", err)
		}
		stats[DocumentStatus(status)] = count
	}
	return stats, rows.Err()
}

// ListDocuments returns all documents ordered by path.
func (s *Store) ListDocuments(ctx context.Context) ([]Document, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, path, filename, extension, size_bytes, file_hash, status,
		       COALESCE(error_msg,''), created_at, updated_at, indexed_at
		FROM documents ORDER BY path`)
	if err != nil {
		return nil, fmt.Errorf("listing documents: %w", err)
	}
	defer rows.Close()

	var docs []Document
	for rows.Next() {
		d, err := scanDocument(rows)
		if err != nil {
			return nil, err
		}
		docs = append(docs, *d)
	}
	return docs, rows.Err()
}

// ChunkIDsByDocumentID returns all chunk IDs for a given document.
func (s *Store) ChunkIDsByDocumentID(ctx context.Context, docID string) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id FROM chunks WHERE document_id = ? ORDER BY chunk_index`, docID)
	if err != nil {
		return nil, fmt.Errorf("querying chunk IDs: %w", err)
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scanning chunk ID: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// ---- helpers ----------------------------------------------------------------

type scanner interface {
	Scan(dest ...interface{}) error
}

func scanDocument(s scanner) (*Document, error) {
	var (
		d          Document
		status     string
		createdStr string
		updatedStr string
		indexedStr sql.NullString
	)
	err := s.Scan(&d.ID, &d.Path, &d.Filename, &d.Extension, &d.SizeBytes,
		&d.FileHash, &status, &d.ErrorMsg, &createdStr, &updatedStr, &indexedStr)
	if err != nil {
		return nil, fmt.Errorf("scanning document: %w", err)
	}
	d.Status = DocumentStatus(status)
	d.CreatedAt = parseTime(createdStr)
	d.UpdatedAt = parseTime(updatedStr)
	if indexedStr.Valid {
		t := parseTime(indexedStr.String)
		d.IndexedAt = &t
	}
	return &d, nil
}

func scanChunk(s scanner) (*Chunk, error) {
	var (
		c          Chunk
		createdStr string
	)
	err := s.Scan(&c.ID, &c.DocumentID, &c.ChunkIndex, &c.Content,
		&c.StartLine, &c.EndLine, &c.Metadata, &createdStr)
	if err != nil {
		return nil, fmt.Errorf("scanning chunk: %w", err)
	}
	c.CreatedAt = parseTime(createdStr)
	return &c, nil
}

func parseTime(s string) time.Time {
	for _, layout := range []string{time.RFC3339, "2006-01-02T15:04:05Z", "2006-01-02 15:04:05"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC()
		}
	}
	return time.Time{}
}

func nullableString(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

// metadataJSON marshals metadata map to JSON string, or empty string if nil.
func metadataJSON(m map[string]string) string {
	if len(m) == 0 {
		return ""
	}
	b, _ := json.Marshal(m)
	return string(b)
}
