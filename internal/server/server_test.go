package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.uber.org/zap"
)

func TestHealthEndpoint(t *testing.T) {
	logger := zap.NewNop()
	srv := New(logger, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, RuntimeInfo{Version: "1.2.3"})

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var resp map[string]string
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp["status"] != "ok" {
		t.Errorf("expected status ok, got %s", resp["status"])
	}
	if resp["version"] != "1.2.3" {
		t.Errorf("expected version 1.2.3, got %s", resp["version"])
	}
	if resp["uptime"] == "" {
		t.Error("expected non-empty uptime")
	}
}

func TestNotFound(t *testing.T) {
	logger := zap.NewNop()
	srv := New(logger, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, RuntimeInfo{})

	req := httptest.NewRequest(http.MethodGet, "/nonexistent", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected status 404, got %d", w.Code)
	}
}

func TestMCPRouteMounted(t *testing.T) {
	logger := zap.NewNop()
	mcpHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/mcp/sse" {
			t.Fatalf("unexpected MCP path %q", r.URL.Path)
		}
		w.WriteHeader(http.StatusNoContent)
	})
	srv := New(logger, nil, nil, nil, nil, nil, nil, nil, nil, nil, mcpHandler, nil, nil, RuntimeInfo{})

	req := httptest.NewRequest(http.MethodGet, "/mcp/sse", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("expected status 204, got %d", w.Code)
	}
}
