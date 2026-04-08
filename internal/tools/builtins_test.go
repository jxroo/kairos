package tools

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jxroo/kairos/internal/config"
	"go.uber.org/zap"
)

func shellPerms() *PermissionChecker {
	return NewPermissionChecker([]config.PermissionRule{
		{Tool: "*", Resource: "shell", Allow: true},
	}, zap.NewNop())
}

func networkPerms() *PermissionChecker {
	return NewPermissionChecker([]config.PermissionRule{
		{Tool: "*", Resource: "network", Allow: true},
	}, zap.NewNop())
}

func fsPerms(paths []string) *PermissionChecker {
	return NewPermissionChecker([]config.PermissionRule{
		{Tool: "*", Resource: "filesystem", Allow: true, Paths: paths},
	}, zap.NewNop())
}

// --- calc ---

func TestCalc(t *testing.T) {
	tests := []struct {
		expr    string
		want    string
		isError bool
	}{
		{"2 + 3", "5", false},
		{"10 - 4 * 2", "2", false},
		{"(1 + 2) * 3", "9", false},
		{"2 ^ 10", "1024", false},
		{"-5 + 3", "-2", false},
		{"3.14 * 2", "6.28", false},
		{"10 / 0", "", true},
		{"", "", true},
		{"abc", "", true},
	}

	handler := calcHandler()
	for _, tt := range tests {
		t.Run(tt.expr, func(t *testing.T) {
			result, err := handler(context.Background(), map[string]any{"expression": tt.expr})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.isError {
				if !result.IsError {
					t.Errorf("expected IsError for %q, got content=%q", tt.expr, result.Content)
				}
				return
			}
			if result.IsError {
				t.Errorf("unexpected IsError for %q: %s", tt.expr, result.Content)
				return
			}
			if result.Content != tt.want {
				t.Errorf("calc(%q) = %q, want %q", tt.expr, result.Content, tt.want)
			}
		})
	}
}

// --- shell ---

func TestShell_Echo(t *testing.T) {
	handler := shellHandler(shellPerms(), zap.NewNop())
	result, err := handler(context.Background(), map[string]any{
		"command": "echo",
		"args":    []any{"hello world"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result.Content, "hello world") {
		t.Errorf("got %q, want 'hello world'", result.Content)
	}
}

func TestShell_PermissionDenied(t *testing.T) {
	handler := shellHandler(permDenyAll(), zap.NewNop())
	result, _ := handler(context.Background(), map[string]any{
		"command": "echo",
		"args":    []any{"test"},
	})
	if !result.IsError || !strings.Contains(result.Content, "permission denied") {
		t.Errorf("expected permission denied, got: %+v", result)
	}
}

// --- git ---

func TestGit_Status(t *testing.T) {
	// Create a temp git repo so the test works on any machine.
	dir := t.TempDir()
	exec.Command("git", "init", dir).Run()
	exec.Command("git", "-C", dir, "commit", "--allow-empty", "-m", "init").Run()

	handler := gitHandler(shellPerms(), zap.NewNop())
	result, err := handler(context.Background(), map[string]any{
		"repo_path": dir,
		"command":   "status",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Errorf("unexpected error: %s", result.Content)
	}
}

func TestGit_DisallowedCommand(t *testing.T) {
	handler := gitHandler(shellPerms(), zap.NewNop())
	result, _ := handler(context.Background(), map[string]any{
		"repo_path": "/tmp",
		"command":   "push",
	})
	if !result.IsError || !strings.Contains(result.Content, "not allowed") {
		t.Errorf("expected disallowed, got: %+v", result)
	}
}

// --- web-fetch ---

func TestWebFetch_Success(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "test response")
	}))
	defer ts.Close()

	handler := webFetchHandler(networkPerms(), zap.NewNop())
	result, err := handler(context.Background(), map[string]any{"url": ts.URL})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result.Content, "test response") {
		t.Errorf("got %q, want 'test response'", result.Content)
	}
	if !strings.Contains(result.Content, "HTTP 200") {
		t.Errorf("missing HTTP status in %q", result.Content)
	}
}

func TestWebFetch_PermissionDenied(t *testing.T) {
	handler := webFetchHandler(permDenyAll(), zap.NewNop())
	result, _ := handler(context.Background(), map[string]any{"url": "http://example.com"})
	if !result.IsError || !strings.Contains(result.Content, "permission denied") {
		t.Errorf("expected permission denied, got: %+v", result)
	}
}

// --- file-ops ---

func TestFileOps_Read(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	os.WriteFile(testFile, []byte("hello file"), 0644)

	handler := fileOpsHandler(fsPerms([]string{tmpDir}), zap.NewNop())
	result, err := handler(context.Background(), map[string]any{
		"operation": "read",
		"path":      testFile,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Content != "hello file" {
		t.Errorf("got %q, want %q", result.Content, "hello file")
	}
}

func TestFileOps_List(t *testing.T) {
	tmpDir := t.TempDir()
	os.WriteFile(filepath.Join(tmpDir, "a.txt"), []byte("a"), 0644)
	os.Mkdir(filepath.Join(tmpDir, "subdir"), 0755)

	handler := fileOpsHandler(fsPerms([]string{tmpDir}), zap.NewNop())
	result, _ := handler(context.Background(), map[string]any{
		"operation": "list",
		"path":      tmpDir,
	})
	if !strings.Contains(result.Content, "a.txt") || !strings.Contains(result.Content, "subdir/") {
		t.Errorf("unexpected list: %s", result.Content)
	}
}

func TestFileOps_Stat(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "stat.txt")
	os.WriteFile(testFile, []byte("data"), 0644)

	handler := fileOpsHandler(fsPerms([]string{tmpDir}), zap.NewNop())
	result, _ := handler(context.Background(), map[string]any{
		"operation": "stat",
		"path":      testFile,
	})
	if !strings.Contains(result.Content, `"name":"stat.txt"`) {
		t.Errorf("unexpected stat: %s", result.Content)
	}
}

func TestFileOps_PathRejection(t *testing.T) {
	handler := fileOpsHandler(fsPerms([]string{"/tmp/safe"}), zap.NewNop())
	result, _ := handler(context.Background(), map[string]any{
		"operation": "read",
		"path":      "/etc/passwd",
	})
	if !result.IsError || !strings.Contains(result.Content, "permission denied") {
		t.Errorf("expected path rejection, got: %+v", result)
	}
}
