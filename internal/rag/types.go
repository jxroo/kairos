package rag

import "time"

// DocumentStatus represents the indexing state of a document.
type DocumentStatus string

const (
	StatusPending  DocumentStatus = "pending"
	StatusIndexing DocumentStatus = "indexing"
	StatusIndexed  DocumentStatus = "indexed"
	StatusError    DocumentStatus = "error"
)

// Document represents a tracked file in the RAG index.
type Document struct {
	ID        string
	Path      string
	Filename  string
	Extension string
	SizeBytes int64
	FileHash  string
	Status    DocumentStatus
	ErrorMsg  string
	CreatedAt time.Time
	UpdatedAt time.Time
	IndexedAt *time.Time
}

// Chunk represents a segment of a document stored for search.
type Chunk struct {
	ID          string
	DocumentID  string
	ChunkIndex  int
	Content     string
	StartLine   int
	EndLine     int
	Metadata    string
	CreatedAt   time.Time
}

// ChunkSearchResult pairs a Chunk with its parent Document and a score.
type ChunkSearchResult struct {
	Chunk      Chunk
	Document   Document
	FinalScore float64
}

// IndexStatus reports current indexing progress.
type IndexStatus struct {
	State        string `json:"state"`
	TotalFiles   int    `json:"total_files"`
	IndexedFiles int    `json:"indexed_files"`
	FailedFiles  int    `json:"failed_files"`
	Percent      int    `json:"percent"`
}
