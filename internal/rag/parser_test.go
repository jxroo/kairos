package rag

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTextParser(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	content := "Hello world\nSecond line\nThird line"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	p := &TextParser{}
	result, err := p.Parse(path)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	if result.Text != content {
		t.Errorf("expected text %q, got %q", content, result.Text)
	}
	if len(result.Lines) != 3 {
		t.Errorf("expected 3 lines, got %d", len(result.Lines))
	}
}

func TestTextParserExtensions(t *testing.T) {
	p := &TextParser{}
	exts := p.SupportedExtensions()
	if len(exts) != 1 || exts[0] != ".txt" {
		t.Errorf("expected [.txt], got %v", exts)
	}
}

func TestMarkdownParser(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.md")
	content := "# My Title\n\nSome paragraph.\n\n## Section\n\nMore text."
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	p := &MarkdownParser{}
	result, err := p.Parse(path)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	if result.Metadata["title"] != "My Title" {
		t.Errorf("expected title 'My Title', got %q", result.Metadata["title"])
	}
	if result.Text != content {
		t.Errorf("expected text to match input")
	}
}

func TestMarkdownParserNoTitle(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "notitle.md")
	content := "Just some text without headings."
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	p := &MarkdownParser{}
	result, err := p.Parse(path)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	if _, ok := result.Metadata["title"]; ok {
		t.Error("expected no title metadata")
	}
}

func TestCodeParser(t *testing.T) {
	tests := []struct {
		name     string
		filename string
		lang     string
	}{
		{"Go", "main.go", "go"},
		{"Python", "script.py", "python"},
		{"Rust", "lib.rs", "rust"},
		{"JavaScript", "app.js", "javascript"},
		{"TypeScript", "app.ts", "typescript"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, tt.filename)
			content := "// some code\nfunc main() {}\n"
			if err := os.WriteFile(path, []byte(content), 0644); err != nil {
				t.Fatal(err)
			}

			p := &CodeParser{}
			result, err := p.Parse(path)
			if err != nil {
				t.Fatalf("Parse() error: %v", err)
			}
			if result.Metadata["language"] != tt.lang {
				t.Errorf("expected language %q, got %q", tt.lang, result.Metadata["language"])
			}
			if len(result.Lines) == 0 {
				t.Error("expected non-empty lines")
			}
		})
	}
}

func TestPDFParser(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.pdf")
	if err := writeSimplePDF(path, "Hello PDF world"); err != nil {
		t.Fatalf("writeSimplePDF() error: %v", err)
	}

	p := &PDFParser{}
	result, err := p.Parse(path)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	if !strings.Contains(result.Text, "Hello PDF world") {
		t.Fatalf("expected extracted PDF text, got %q", result.Text)
	}
	if len(result.Lines) == 0 {
		t.Fatal("expected non-empty lines from PDF parser")
	}
}

func TestParserRegistry(t *testing.T) {
	r := DefaultRegistry()

	tests := []struct {
		ext       string
		supported bool
	}{
		{".md", true},
		{".txt", true},
		{".go", true},
		{".py", true},
		{".rs", true},
		{".js", true},
		{".ts", true},
		{".pdf", true},
		{".docx", false},
	}

	for _, tt := range tests {
		t.Run(tt.ext, func(t *testing.T) {
			if got := r.Supported(tt.ext); got != tt.supported {
				t.Errorf("Supported(%q) = %v, want %v", tt.ext, got, tt.supported)
			}
		})
	}
}

func TestParserRegistryCaseInsensitive(t *testing.T) {
	r := DefaultRegistry()
	if !r.Supported(".MD") {
		t.Error("expected .MD to be supported (case insensitive)")
	}
	if !r.Supported(".Go") {
		t.Error("expected .Go to be supported (case insensitive)")
	}
}

func TestParserFileNotFound(t *testing.T) {
	p := &TextParser{}
	_, err := p.Parse("/nonexistent/file.txt")
	if err == nil {
		t.Error("expected error for non-existent file")
	}
}

func writeSimplePDF(path string, text string) error {
	escaped := escapePDFText(text)
	stream := fmt.Sprintf("BT\n/F1 12 Tf\n72 72 Td\n(%s) Tj\nET\n", escaped)
	objects := []string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 300 144] /Contents 4 0 R /Resources << /Font << /F1 5 0 R >> >> >>",
		fmt.Sprintf("<< /Length %d >>\nstream\n%sendstream", len(stream), stream),
		"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
	}

	var b strings.Builder
	b.WriteString("%PDF-1.4\n")
	offsets := make([]int, len(objects)+1)
	for i, obj := range objects {
		offsets[i+1] = b.Len()
		fmt.Fprintf(&b, "%d 0 obj\n%s\nendobj\n", i+1, obj)
	}

	xrefOffset := b.Len()
	fmt.Fprintf(&b, "xref\n0 %d\n", len(objects)+1)
	b.WriteString("0000000000 65535 f \n")
	for i := 1; i <= len(objects); i++ {
		fmt.Fprintf(&b, "%010d 00000 n \n", offsets[i])
	}
	fmt.Fprintf(&b, "trailer\n<< /Root 1 0 R /Size %d >>\nstartxref\n%d\n%%%%EOF\n", len(objects)+1, xrefOffset)

	return os.WriteFile(path, []byte(b.String()), 0644)
}

func escapePDFText(s string) string {
	replacer := strings.NewReplacer(`\`, `\\`, "(", `\(`, ")", `\)`)
	return replacer.Replace(s)
}
