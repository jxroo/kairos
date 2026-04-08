package rag

import (
	"fmt"
	"os"
	"strings"
)

// TextParser handles plain text files.
type TextParser struct{}

func (p *TextParser) SupportedExtensions() []string {
	return []string{".txt"}
}

func (p *TextParser) Parse(path string) (ParseResult, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return ParseResult{}, fmt.Errorf("reading text file: %w", err)
	}

	text := string(data)
	lines := strings.Split(text, "\n")

	return ParseResult{
		Text:     text,
		Lines:    lines,
		Metadata: map[string]string{},
	}, nil
}
