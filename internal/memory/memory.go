// Package memory implements persistent conversation memory with semantic search.
package memory

import "time"

// Memory represents a stored memory entry.
type Memory struct {
	ID             string
	Content        string
	Summary        string
	Importance     string
	ConversationID string
	Source         string

	ImportanceWeight float64
	DecayScore       float64

	Tags     []string
	Entities []Entity

	CreatedAt  time.Time
	UpdatedAt  time.Time
	AccessedAt time.Time

	AccessCount int
}

// Entity represents a named entity extracted from memory content.
type Entity struct {
	ID           string
	Name         string
	Type         string
	MentionCount int
}

// CreateMemoryInput holds the fields required to create a new memory.
type CreateMemoryInput struct {
	Content        string
	Context        string
	Importance     string
	ConversationID string
	Source         string
	Tags           []string
}

// UpdateMemoryInput holds optional fields for updating a memory.
// Only non-nil pointer fields are applied.
type UpdateMemoryInput struct {
	Content    *string
	Importance *string
	Tags       []string
}

// ListOptions controls pagination and filtering for List queries.
type ListOptions struct {
	Limit  int
	Offset int
	Tags   []string
}
