package tools

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/jxroo/kairos/internal/config"
	"go.uber.org/zap"
)

func testLogger() *zap.Logger {
	return zap.NewNop()
}

func TestPermissionChecker(t *testing.T) {
	tests := []struct {
		name     string
		rules    []config.PermissionRule
		tool     string
		resource string
		path     string
		wantErr  bool
	}{
		{
			name: "explicit allow",
			rules: []config.PermissionRule{
				{Tool: "shell", Resource: "shell", Allow: true},
			},
			tool: "shell", resource: "shell",
			wantErr: false,
		},
		{
			name: "explicit deny",
			rules: []config.PermissionRule{
				{Tool: "shell", Resource: "shell", Allow: false},
			},
			tool: "shell", resource: "shell",
			wantErr: true,
		},
		{
			name: "wildcard matches",
			rules: []config.PermissionRule{
				{Tool: "*", Resource: "network", Allow: true},
			},
			tool: "web-fetch", resource: "network",
			wantErr: false,
		},
		{
			name: "no match defaults deny",
			rules: []config.PermissionRule{
				{Tool: "other", Resource: "shell", Allow: true},
			},
			tool: "shell", resource: "shell",
			wantErr: true,
		},
		{
			name: "path scoping allows",
			rules: []config.PermissionRule{
				{Tool: "file-ops", Resource: "filesystem", Allow: true, Paths: []string{"/tmp"}},
			},
			tool: "file-ops", resource: "filesystem", path: "/tmp/test.txt",
			wantErr: false,
		},
		{
			name: "path scoping denies out-of-scope",
			rules: []config.PermissionRule{
				{Tool: "file-ops", Resource: "filesystem", Allow: true, Paths: []string{"/tmp"}},
			},
			tool: "file-ops", resource: "filesystem", path: "/etc/passwd",
			wantErr: true,
		},
		{
			name: "first match wins - deny before allow",
			rules: []config.PermissionRule{
				{Tool: "shell", Resource: "shell", Allow: false},
				{Tool: "*", Resource: "shell", Allow: true},
			},
			tool: "shell", resource: "shell",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pc := NewPermissionChecker(tt.rules, testLogger())
			err := pc.Check(tt.tool, tt.resource, tt.path)
			if tt.wantErr && !errors.Is(err, ErrPermissionDenied) {
				t.Errorf("expected ErrPermissionDenied, got %v", err)
			}
			if !tt.wantErr && err != nil {
				t.Errorf("expected nil, got %v", err)
			}
		})
	}
}

func TestSymlinkTraversal(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks not reliably available on Windows")
	}

	// Create allowed directory with a file.
	allowedDir := t.TempDir()
	testFile := filepath.Join(allowedDir, "safe.txt")
	if err := os.WriteFile(testFile, []byte("safe"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create a symlink inside allowed dir that points outside.
	escapePath := filepath.Join(allowedDir, "escape")
	if err := os.Symlink("/etc", escapePath); err != nil {
		t.Skipf("cannot create symlink: %v", err)
	}

	// pathAllowed should deny the symlink target that escapes.
	if pathAllowed([]string{allowedDir}, filepath.Join(escapePath, "passwd")) {
		t.Error("expected symlink traversal to /etc/passwd to be denied")
	}

	// Regular file inside allowed dir should still work.
	if !pathAllowed([]string{allowedDir}, testFile) {
		t.Error("expected safe.txt inside allowed dir to be allowed")
	}
}
