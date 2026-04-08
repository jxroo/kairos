package rag

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// CodeParser handles source code files with language detection.
type CodeParser struct{}

var extToLanguage = map[string]string{
	".go":   "go",
	".py":   "python",
	".rs":   "rust",
	".js":   "javascript",
	".ts":   "typescript",
	".java": "java",
	".c":    "c",
	".cpp":  "cpp",
	".h":    "c",
	".rb":   "ruby",
	".sh":   "shell",
	".sql":  "sql",
	".yaml": "yaml",
	".yml":  "yaml",
	".json": "json",
	".toml": "toml",
}

func (p *CodeParser) SupportedExtensions() []string {
	exts := make([]string, 0, len(extToLanguage))
	for ext := range extToLanguage {
		exts = append(exts, ext)
	}
	return exts
}

func (p *CodeParser) Parse(path string) (ParseResult, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return ParseResult{}, fmt.Errorf("reading code file: %w", err)
	}

	text := string(data)
	lines := strings.Split(text, "\n")

	ext := strings.ToLower(filepath.Ext(path))
	lang, ok := extToLanguage[ext]
	if !ok {
		lang = "unknown"
	}

	return ParseResult{
		Text:  text,
		Lines: lines,
		Metadata: map[string]string{
			"language": lang,
		},
	}, nil
}
