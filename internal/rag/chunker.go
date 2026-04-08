package rag

import "strings"

// Chunker splits parsed text into overlapping chunks with line tracking.
type Chunker struct {
	chunkSize    int
	chunkOverlap int
}

// ChunkOutput represents a single chunk with positional info.
type ChunkOutput struct {
	Content   string
	StartLine int
	EndLine   int
	Metadata  map[string]string
}

// NewChunker creates a Chunker with the given size and overlap in characters.
func NewChunker(chunkSize, chunkOverlap int) *Chunker {
	if chunkSize <= 0 {
		chunkSize = 512
	}
	if chunkOverlap < 0 {
		chunkOverlap = 0
	}
	if chunkOverlap >= chunkSize {
		chunkOverlap = chunkSize / 4
	}
	return &Chunker{chunkSize: chunkSize, chunkOverlap: chunkOverlap}
}

// Chunk splits a ParseResult into chunks respecting semantic boundaries.
func (c *Chunker) Chunk(result ParseResult) []ChunkOutput {
	lines := result.Lines
	if len(lines) == 0 {
		return nil
	}

	// Remove trailing empty line from Split artifact.
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	if len(lines) == 0 {
		return nil
	}

	var chunks []ChunkOutput
	var buf strings.Builder
	startLine := 1
	currentLine := 1

	for i, line := range lines {
		lineNum := i + 1

		// If adding this line would exceed chunkSize and we have content,
		// find a good split point.
		lineWithNewline := line + "\n"
		if buf.Len()+len(lineWithNewline) > c.chunkSize && buf.Len() > 0 {
			content := strings.TrimRight(buf.String(), "\n")
			if content != "" {
				chunks = append(chunks, ChunkOutput{
					Content:   content,
					StartLine: startLine,
					EndLine:   currentLine - 1,
					Metadata:  copyMetadata(result.Metadata),
				})
			}

			// Compute overlap: walk backward from the end of the buffer content.
			overlapContent, overlapLines := c.computeOverlap(content, currentLine-1)
			buf.Reset()
			if overlapContent != "" {
				buf.WriteString(overlapContent)
				buf.WriteString("\n")
				startLine = currentLine - overlapLines
			} else {
				startLine = lineNum
			}
		}

		if buf.Len() == 0 {
			startLine = lineNum
		}

		// Heading forces a new chunk (if we have accumulated content).
		if isHeading(line) && buf.Len() > 0 {
			content := strings.TrimRight(buf.String(), "\n")
			if content != "" {
				chunks = append(chunks, ChunkOutput{
					Content:   content,
					StartLine: startLine,
					EndLine:   lineNum - 1,
					Metadata:  copyMetadata(result.Metadata),
				})
			}
			buf.Reset()
			startLine = lineNum
		}

		buf.WriteString(lineWithNewline)
		currentLine = lineNum + 1
	}

	// Flush remaining buffer.
	content := strings.TrimRight(buf.String(), "\n")
	if content != "" {
		chunks = append(chunks, ChunkOutput{
			Content:   content,
			StartLine: startLine,
			EndLine:   len(lines),
			Metadata:  copyMetadata(result.Metadata),
		})
	}

	return chunks
}

// computeOverlap returns the last chunkOverlap characters of content,
// aligned to line boundaries, along with how many lines are included.
func (c *Chunker) computeOverlap(content string, endLine int) (string, int) {
	if c.chunkOverlap <= 0 {
		return "", 0
	}

	lines := strings.Split(content, "\n")
	var overlap strings.Builder
	lineCount := 0

	for i := len(lines) - 1; i >= 0; i-- {
		candidate := lines[i]
		if overlap.Len()+len(candidate)+1 > c.chunkOverlap {
			break
		}
		if lineCount > 0 {
			overlap.Reset()
			// Rebuild from the discovered start line.
			for j := i; j < len(lines); j++ {
				if j > i {
					overlap.WriteString("\n")
				}
				overlap.WriteString(lines[j])
			}
			lineCount = len(lines) - i
		} else {
			overlap.WriteString(candidate)
			lineCount = 1
		}
	}

	return overlap.String(), lineCount
}

func isHeading(line string) bool {
	trimmed := strings.TrimSpace(line)
	return strings.HasPrefix(trimmed, "# ") ||
		strings.HasPrefix(trimmed, "## ") ||
		strings.HasPrefix(trimmed, "### ")
}

func copyMetadata(m map[string]string) map[string]string {
	if m == nil {
		return nil
	}
	cp := make(map[string]string, len(m))
	for k, v := range m {
		cp[k] = v
	}
	return cp
}
