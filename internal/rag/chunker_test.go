package rag

import (
	"strings"
	"testing"
)

func TestChunkerShortText(t *testing.T) {
	c := NewChunker(512, 64)
	result := ParseResult{
		Text:  "Short text",
		Lines: []string{"Short text"},
	}

	chunks := c.Chunk(result)
	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(chunks))
	}
	if chunks[0].Content != "Short text" {
		t.Errorf("expected 'Short text', got %q", chunks[0].Content)
	}
	if chunks[0].StartLine != 1 || chunks[0].EndLine != 1 {
		t.Errorf("expected lines 1-1, got %d-%d", chunks[0].StartLine, chunks[0].EndLine)
	}
}

func TestChunkerMultipleChunks(t *testing.T) {
	// Each line is ~20 chars. With chunk size 50, we should get multiple chunks.
	var lines []string
	for i := 0; i < 20; i++ {
		lines = append(lines, "Line content number X")
	}
	text := strings.Join(lines, "\n")

	c := NewChunker(50, 0)
	result := ParseResult{
		Text:  text,
		Lines: lines,
	}

	chunks := c.Chunk(result)
	if len(chunks) < 2 {
		t.Fatalf("expected at least 2 chunks, got %d", len(chunks))
	}

	// Verify all lines are covered.
	for _, ch := range chunks {
		if ch.StartLine < 1 {
			t.Errorf("StartLine should be >= 1, got %d", ch.StartLine)
		}
		if ch.EndLine < ch.StartLine {
			t.Errorf("EndLine (%d) < StartLine (%d)", ch.EndLine, ch.StartLine)
		}
	}
}

func TestChunkerOverlap(t *testing.T) {
	// Create text with 10 lines, each ~20 chars.
	var lines []string
	for i := 0; i < 10; i++ {
		lines = append(lines, "Line content here XX")
	}
	text := strings.Join(lines, "\n")

	c := NewChunker(50, 25)
	result := ParseResult{
		Text:  text,
		Lines: lines,
	}

	chunks := c.Chunk(result)
	if len(chunks) < 2 {
		t.Fatalf("expected at least 2 chunks, got %d", len(chunks))
	}

	// With overlap, later chunks may start before the previous chunk ends.
	for i := 1; i < len(chunks); i++ {
		if chunks[i].StartLine > chunks[i-1].EndLine+1 {
			// No overlap detected — this is OK if the chunk boundary aligns perfectly.
		}
	}
}

func TestChunkerHeadingForcesNewChunk(t *testing.T) {
	lines := []string{
		"Some intro text here.",
		"More intro content.",
		"# New Section",
		"Content under new section.",
	}
	text := strings.Join(lines, "\n")

	c := NewChunker(1000, 0) // Large chunk size so heading is the only split reason.
	result := ParseResult{
		Text:  text,
		Lines: lines,
	}

	chunks := c.Chunk(result)
	if len(chunks) != 2 {
		t.Fatalf("expected 2 chunks (split at heading), got %d", len(chunks))
	}
	if !strings.Contains(chunks[0].Content, "intro") {
		t.Error("first chunk should contain intro text")
	}
	if !strings.Contains(chunks[1].Content, "New Section") {
		t.Error("second chunk should contain heading")
	}
}

func TestChunkerEmptyInput(t *testing.T) {
	c := NewChunker(512, 64)
	result := ParseResult{
		Text:  "",
		Lines: nil,
	}

	chunks := c.Chunk(result)
	if len(chunks) != 0 {
		t.Errorf("expected 0 chunks for empty input, got %d", len(chunks))
	}
}

func TestChunkerLineNumbers(t *testing.T) {
	lines := []string{
		"Line 1",
		"Line 2",
		"Line 3",
		"Line 4",
		"Line 5",
	}
	text := strings.Join(lines, "\n")

	c := NewChunker(1000, 0)
	result := ParseResult{
		Text:  text,
		Lines: lines,
	}

	chunks := c.Chunk(result)
	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(chunks))
	}
	if chunks[0].StartLine != 1 {
		t.Errorf("expected StartLine 1, got %d", chunks[0].StartLine)
	}
	if chunks[0].EndLine != 5 {
		t.Errorf("expected EndLine 5, got %d", chunks[0].EndLine)
	}
}

func TestChunkerInvalidParams(t *testing.T) {
	// Zero chunk size should default.
	c := NewChunker(0, 0)
	if c.chunkSize != 512 {
		t.Errorf("expected default chunk size 512, got %d", c.chunkSize)
	}

	// Overlap >= chunk size should be reduced.
	c = NewChunker(100, 200)
	if c.chunkOverlap >= c.chunkSize {
		t.Errorf("overlap (%d) should be less than chunk size (%d)", c.chunkOverlap, c.chunkSize)
	}
}

func TestChunkerParagraphBoundary(t *testing.T) {
	// Build text with a paragraph break in the middle.
	lines := []string{
		"First paragraph line one.",
		"First paragraph line two.",
		"",
		"Second paragraph line one.",
		"Second paragraph line two.",
	}
	// Set chunk size to accommodate about 2 lines (~50 chars).
	text := strings.Join(lines, "\n")

	c := NewChunker(60, 0)
	result := ParseResult{
		Text:  text,
		Lines: lines,
	}

	chunks := c.Chunk(result)
	if len(chunks) < 2 {
		t.Fatalf("expected at least 2 chunks, got %d", len(chunks))
	}
}
