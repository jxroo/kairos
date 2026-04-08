package memory

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// Conversation represents a stored conversation session.
type Conversation struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Model     string    `json:"model"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ConversationMessage represents a single message within a conversation.
type ConversationMessage struct {
	ID             string    `json:"id"`
	ConversationID string    `json:"conversation_id"`
	Role           string    `json:"role"`
	Content        string    `json:"content"`
	Tokens         int       `json:"tokens,omitempty"`
	Metadata       string    `json:"metadata,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
}

// CreateConversation inserts a new Conversation row and returns the full record.
func (s *Store) CreateConversation(ctx context.Context, title, model string) (*Conversation, error) {
	id := uuid.New().String()
	now := time.Now().UTC().Format("2006-01-02 15:04:05")

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO conversations (id, title, model, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)`,
		id, nullableString(title), nullableString(model), now, now,
	)
	if err != nil {
		return nil, fmt.Errorf("inserting conversation: %w", err)
	}

	s.logger.Debug("conversation created", zap.String("id", id))
	return s.GetConversation(ctx, id)
}

// GetConversation retrieves a Conversation by ID.
// Returns an error wrapping sql.ErrNoRows if the conversation does not exist.
func (s *Store) GetConversation(ctx context.Context, id string) (*Conversation, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, COALESCE(title,''), COALESCE(model,''), created_at, updated_at
		FROM conversations WHERE id = ?`, id)

	c, err := scanConversation(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("conversation %q not found: %w", id, err)
		}
		return nil, fmt.Errorf("scanning conversation: %w", err)
	}
	return c, nil
}

// ListConversations returns conversations ordered by updated_at descending,
// with pagination support.
func (s *Store) ListConversations(ctx context.Context, limit, offset int) ([]Conversation, error) {
	if limit <= 0 {
		limit = 50
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT id, COALESCE(title,''), COALESCE(model,''), created_at, updated_at
		FROM conversations
		ORDER BY updated_at DESC
		LIMIT ? OFFSET ?`, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("listing conversations: %w", err)
	}
	defer rows.Close()

	var convs []Conversation
	for rows.Next() {
		c, err := scanConversation(rows)
		if err != nil {
			return nil, fmt.Errorf("scanning conversation row: %w", err)
		}
		convs = append(convs, *c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating conversation rows: %w", err)
	}
	return convs, nil
}

// SearchConversations returns conversations whose title or message content
// matches the supplied query.
func (s *Store) SearchConversations(ctx context.Context, query string, limit, offset int) ([]Conversation, error) {
	if limit <= 0 {
		limit = 50
	}

	like := "%" + query + "%"
	rows, err := s.db.QueryContext(ctx, `
		SELECT DISTINCT c.id, COALESCE(c.title,''), COALESCE(c.model,''), c.created_at, c.updated_at
		FROM conversations c
		LEFT JOIN messages m ON m.conversation_id = c.id
		WHERE c.title LIKE ? COLLATE NOCASE OR m.content LIKE ? COLLATE NOCASE
		ORDER BY c.updated_at DESC
		LIMIT ? OFFSET ?`, like, like, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("searching conversations: %w", err)
	}
	defer rows.Close()

	var convs []Conversation
	for rows.Next() {
		c, err := scanConversation(rows)
		if err != nil {
			return nil, fmt.Errorf("scanning conversation row: %w", err)
		}
		convs = append(convs, *c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating conversation rows: %w", err)
	}
	return convs, nil
}

// DeleteConversation removes the conversation and, via FK cascade, all its messages.
func (s *Store) DeleteConversation(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM conversations WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("deleting conversation: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("checking rows affected: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("conversation %q not found: %w", id, sql.ErrNoRows)
	}
	s.logger.Debug("conversation deleted", zap.String("id", id))
	return nil
}

// AddMessage appends a ConversationMessage to the given conversation.
// The message ID is generated if empty. updated_at on the parent conversation
// is bumped to reflect recent activity.
func (s *Store) AddMessage(ctx context.Context, convID string, msg ConversationMessage) error {
	if msg.ID == "" {
		msg.ID = uuid.New().String()
	}
	now := time.Now().UTC().Format("2006-01-02 15:04:05")
	createdAt := now
	if !msg.CreatedAt.IsZero() {
		createdAt = msg.CreatedAt.UTC().Format("2006-01-02 15:04:05")
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("beginning add message transaction: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	var tokens interface{}
	if msg.Tokens != 0 {
		tokens = msg.Tokens
	}

	_, err = tx.ExecContext(ctx, `
		INSERT INTO messages (id, conversation_id, role, content, tokens, metadata, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		msg.ID, convID, msg.Role, msg.Content, tokens, msg.Metadata, createdAt,
	)
	if err != nil {
		return fmt.Errorf("inserting message: %w", err)
	}

	_, err = tx.ExecContext(ctx, `
		UPDATE conversations SET updated_at = ? WHERE id = ?`, now, convID)
	if err != nil {
		return fmt.Errorf("updating conversation updated_at: %w", err)
	}

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("committing add message transaction: %w", err)
	}

	s.logger.Debug("message added",
		zap.String("conversation_id", convID),
		zap.String("message_id", msg.ID),
	)
	return nil
}

// GetMessages returns all messages for the given conversation, ordered by
// created_at ascending.
func (s *Store) GetMessages(ctx context.Context, convID string) ([]ConversationMessage, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, conversation_id, role, content, COALESCE(tokens, 0), COALESCE(metadata, ''), created_at
		FROM messages
		WHERE conversation_id = ?
		ORDER BY created_at ASC`, convID)
	if err != nil {
		return nil, fmt.Errorf("querying messages: %w", err)
	}
	defer rows.Close()

	var msgs []ConversationMessage
	for rows.Next() {
		var (
			m          ConversationMessage
			createdStr string
		)
		if err := rows.Scan(
			&m.ID, &m.ConversationID, &m.Role, &m.Content, &m.Tokens, &m.Metadata, &createdStr,
		); err != nil {
			return nil, fmt.Errorf("scanning message row: %w", err)
		}
		m.CreatedAt = parseTime(createdStr)
		msgs = append(msgs, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating message rows: %w", err)
	}
	return msgs, nil
}

// ---- helpers ----------------------------------------------------------------

func scanConversation(s scanner) (*Conversation, error) {
	var (
		c          Conversation
		createdStr string
		updatedStr string
	)
	err := s.Scan(&c.ID, &c.Title, &c.Model, &createdStr, &updatedStr)
	if err != nil {
		return nil, err
	}
	c.CreatedAt = parseTime(createdStr)
	c.UpdatedAt = parseTime(updatedStr)
	return &c, nil
}
