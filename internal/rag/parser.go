package rag

import "strings"

// ParseResult holds text extracted from a document along with metadata.
type ParseResult struct {
	Text     string
	Metadata map[string]string
	Lines    []string
}

// Parser extracts text content from a file.
type Parser interface {
	Parse(path string) (ParseResult, error)
	SupportedExtensions() []string
}

// ParserRegistry maps file extensions to their Parser implementations.
type ParserRegistry struct {
	parsers map[string]Parser
}

// NewParserRegistry creates an empty registry.
func NewParserRegistry() *ParserRegistry {
	return &ParserRegistry{parsers: make(map[string]Parser)}
}

// Register adds a parser for all of its supported extensions.
func (r *ParserRegistry) Register(p Parser) {
	for _, ext := range p.SupportedExtensions() {
		r.parsers[strings.ToLower(ext)] = p
	}
}

// Get returns the parser for the given extension, or nil if unsupported.
func (r *ParserRegistry) Get(ext string) Parser {
	return r.parsers[strings.ToLower(ext)]
}

// Supported returns true if the extension has a registered parser.
func (r *ParserRegistry) Supported(ext string) bool {
	_, ok := r.parsers[strings.ToLower(ext)]
	return ok
}

// DefaultRegistry returns a ParserRegistry pre-loaded with all built-in parsers.
func DefaultRegistry() *ParserRegistry {
	r := NewParserRegistry()
	r.Register(&TextParser{})
	r.Register(&MarkdownParser{})
	r.Register(&CodeParser{})
	r.Register(&PDFParser{})
	return r
}
