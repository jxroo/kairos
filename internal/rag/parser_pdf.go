package rag

import (
	"bytes"
	"fmt"
	"io"
	"strings"

	"github.com/ledongthuc/pdf"
)

// PDFParser extracts plain text content from PDF files.
type PDFParser struct{}

func (p *PDFParser) SupportedExtensions() []string {
	return []string{".pdf"}
}

func (p *PDFParser) Parse(path string) (ParseResult, error) {
	f, reader, err := pdf.Open(path)
	if err != nil {
		return ParseResult{}, fmt.Errorf("opening pdf file: %w", err)
	}
	defer f.Close()

	textReader, err := reader.GetPlainText()
	if err != nil {
		return ParseResult{}, fmt.Errorf("extracting pdf text: %w", err)
	}

	var buf bytes.Buffer
	if _, err := io.Copy(&buf, textReader); err != nil {
		return ParseResult{}, fmt.Errorf("reading pdf text: %w", err)
	}

	text := strings.TrimSpace(buf.String())
	if text == "" {
		return ParseResult{
			Text:     "",
			Lines:    nil,
			Metadata: map[string]string{},
		}, nil
	}

	return ParseResult{
		Text:     text,
		Lines:    strings.Split(text, "\n"),
		Metadata: map[string]string{},
	}, nil
}
