package tools

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jxroo/kairos/internal/config"
	"go.uber.org/zap"
)

func TestLoadToolFile_Valid(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "word-count.toml")
	os.WriteFile(path, []byte(`
name = "word-count"
description = "Count words in text"

[input_schema.text]
type = "string"
required = true

[script]
inline = "function run(args) { return String(args.text.split(/\\s+/).filter(Boolean).length); }"
`), 0644)

	def, script, err := LoadToolFile(path)
	if err != nil {
		t.Fatalf("LoadToolFile: %v", err)
	}
	if def.Name != "word-count" {
		t.Errorf("name = %q, want %q", def.Name, "word-count")
	}
	if script == "" {
		t.Error("script is empty")
	}
	if _, ok := def.InputSchema["text"]; !ok {
		t.Error("missing 'text' param in input_schema")
	}
}

func TestLoadToolFile_ExternalScript(t *testing.T) {
	dir := t.TempDir()

	os.WriteFile(filepath.Join(dir, "count.js"), []byte(`function run(args) { return "42"; }`), 0644)
	os.WriteFile(filepath.Join(dir, "count.toml"), []byte(`
name = "count"
description = "A counter"

[script]
file = "count.js"
`), 0644)

	def, script, err := LoadToolFile(filepath.Join(dir, "count.toml"))
	if err != nil {
		t.Fatalf("LoadToolFile: %v", err)
	}
	if def.Name != "count" {
		t.Errorf("name = %q", def.Name)
	}
	if script != `function run(args) { return "42"; }` {
		t.Errorf("script = %q", script)
	}
}

func TestLoadToolFile_InvalidTOML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.toml")
	os.WriteFile(path, []byte(`this is not valid toml {{{`), 0644)

	_, _, err := LoadToolFile(path)
	if err == nil {
		t.Error("expected error for invalid TOML")
	}
}

func TestLoadToolFile_MissingName(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "noname.toml")
	os.WriteFile(path, []byte(`description = "no name"`), 0644)

	_, _, err := LoadToolFile(path)
	if err == nil {
		t.Error("expected error for missing name")
	}
}

func TestLoadToolsFromDir(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "greet.toml"), []byte(`
name = "greet"
description = "A greeter"

[input_schema.name]
type = "string"
required = true

[script]
inline = "function run(args) { return 'Hello ' + args.name; }"
`), 0644)

	// Non-toml files should be skipped.
	os.WriteFile(filepath.Join(dir, "readme.md"), []byte("# Not a tool"), 0644)

	registry := NewRegistry()
	perms := NewPermissionChecker(nil, zap.NewNop())
	sandbox := NewSandboxRunner(perms, 5*time.Second, zap.NewNop())

	err := LoadToolsFromDir(dir, registry, sandbox, zap.NewNop())
	if err != nil {
		t.Fatalf("LoadToolsFromDir: %v", err)
	}

	defs := registry.List()
	if len(defs) != 1 {
		t.Fatalf("got %d tools, want 1", len(defs))
	}
	if defs[0].Name != "greet" {
		t.Errorf("tool name = %q, want %q", defs[0].Name, "greet")
	}

	// Verify the tool actually runs.
	_, handler, _ := registry.Get("greet")
	result, _ := handler(context.Background(), map[string]any{"name": "Alice"})
	if result.Content != "Hello Alice" {
		t.Errorf("greet(Alice) = %q, want %q", result.Content, "Hello Alice")
	}
}

func TestLoadToolsFromDir_NonexistentDir(t *testing.T) {
	registry := NewRegistry()
	perms := NewPermissionChecker(nil, zap.NewNop())
	sandbox := NewSandboxRunner(perms, 5*time.Second, zap.NewNop())

	err := LoadToolsFromDir("/nonexistent/path", registry, sandbox, zap.NewNop())
	if err != nil {
		t.Fatalf("expected nil for nonexistent dir, got %v", err)
	}
}

func TestLoadToolsFromDir_SkipsSubdirs(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "subdir"), 0755)

	registry := NewRegistry()
	perms := NewPermissionChecker([]config.PermissionRule{}, zap.NewNop())
	sandbox := NewSandboxRunner(perms, 5*time.Second, zap.NewNop())

	err := LoadToolsFromDir(dir, registry, sandbox, zap.NewNop())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(registry.List()) != 0 {
		t.Error("expected no tools loaded")
	}
}
