package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"go.uber.org/zap"
)

func TestHandleGetConfigMissingReturnsStarter(t *testing.T) {
	logger := zap.NewNop()
	cfgPath := filepath.Join(t.TempDir(), "config.toml")
	srv := New(logger, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, RuntimeInfo{
		ConfigPath:    cfgPath,
		StarterConfig: "hello = \"world\"\n",
	})

	req := httptest.NewRequest(http.MethodGet, "/config", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp configResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decoding response: %v", err)
	}
	if resp.Content != "hello = \"world\"\n" {
		t.Fatalf("expected starter config, got %q", resp.Content)
	}
}

func TestHandleUpdateConfigValidatesAndWrites(t *testing.T) {
	logger := zap.NewNop()
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.toml")
	srv := New(logger, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, RuntimeInfo{
		ConfigPath: cfgPath,
	})

	body := []byte(`{"content":"[server]\nport = 8888\n"}`)
	req := httptest.NewRequest(http.MethodPut, "/config", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	content, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("reading config file: %v", err)
	}
	if string(content) != "[server]\nport = 8888\n" {
		t.Fatalf("unexpected persisted content: %q", content)
	}
}

func TestHandleUpdateConfigRejectsInvalidTOML(t *testing.T) {
	logger := zap.NewNop()
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.toml")
	if err := os.WriteFile(cfgPath, []byte("[server]\nport = 7777\n"), 0644); err != nil {
		t.Fatalf("seeding config: %v", err)
	}

	srv := New(logger, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, RuntimeInfo{
		ConfigPath: cfgPath,
	})

	body := []byte(`{"content":"[server]\nport = \"oops\""}`)
	req := httptest.NewRequest(http.MethodPut, "/config", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}

	content, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("reading config file: %v", err)
	}
	if string(content) != "[server]\nport = 7777\n" {
		t.Fatalf("config should remain unchanged, got %q", content)
	}
}
