package rag

import (
	"fmt"
	"os"
	"strings"
)

// MarkdownParser handles .md files with heading-aware metadata extraction.
type MarkdownParser struct{}

func (p *MarkdownParser) SupportedExtensions() []string {
	return []string{".md"}
}

func (p *MarkdownParser) Parse(path string) (ParseResult, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return ParseResult{}, fmt.Errorf("reading markdown file: %w", err)
	}

	text := string(data)
	lines := strings.Split(text, "\n")

	metadata := map[string]string{}

	// Extract first heading as title.
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "# ") {
			metadata["title"] = strings.TrimPrefix(trimmed, "# ")
			break
		}
	}

	return ParseResult{
		Text:     text,
		Lines:    lines,
		Metadata: metadata,
	}, nil
}
