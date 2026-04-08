package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"go.uber.org/zap"

	"github.com/jxroo/kairos/internal/memory"
)

func setupTestServer(t *testing.T) (*Server, *memory.Store) {
	t.Helper()
	logger := zap.NewNop()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := memory.NewStore(dbPath, logger)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	t.Cleanup(func() { store.Close() })

	embedder := memory.NewFallbackEmbedder()
	index, err := memory.NewFallbackIndex("")
	if err != nil {
		t.Fatalf("failed to create index: %v", err)
	}
	searchSvc := memory.NewSearchService(store, embedder, index, logger)

	srv := New(logger, store, searchSvc, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, RuntimeInfo{})
	return srv, store
}

func TestHandleCreateMemory(t *testing.T) {
	srv, _ := setupTestServer(t)

	in := memory.CreateMemoryInput{
		Content:    "test memory content",
		Importance: "normal",
	}
	body, _ := json.Marshal(in)

	req := httptest.NewRequest(http.MethodPost, "/memories", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("expected status 201, got %d: %s", w.Code, w.Body.String())
	}

	var resp memory.Memory
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.ID == "" {
		t.Error("expected non-empty ID in response")
	}
	if resp.Content != in.Content {
		t.Errorf("expected content %q, got %q", in.Content, resp.Content)
	}
}

func TestHandleCreateMemoryMissingContent(t *testing.T) {
	srv, _ := setupTestServer(t)

	in := memory.CreateMemoryInput{Content: ""}
	body, _ := json.Marshal(in)

	req := httptest.NewRequest(http.MethodPost, "/memories", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}
}

func TestHandleSearchMemories(t *testing.T) {
	srv, store := setupTestServer(t)

	// Pre-populate a memory so search has something to find.
	_, err := store.Create(t.Context(), memory.CreateMemoryInput{
		Content: "Go programming language created by Google",
	})
	if err != nil {
		t.Fatalf("failed to seed memory: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/memories/search?query=Go+programming&limit=5", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	var results []memory.SearchResult
	if err := json.NewDecoder(w.Body).Decode(&results); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
}

func TestHandleGetMemory(t *testing.T) {
	srv, store := setupTestServer(t)

	mem, err := store.Create(t.Context(), memory.CreateMemoryInput{
		Content: "a specific memory",
	})
	if err != nil {
		t.Fatalf("failed to seed memory: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/memories/"+mem.ID, nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp memory.Memory
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.ID != mem.ID {
		t.Errorf("expected ID %q, got %q", mem.ID, resp.ID)
	}
}

func TestHandleGetMemoryNotFound(t *testing.T) {
	srv, _ := setupTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/memories/nonexistent-id", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected status 404, got %d", w.Code)
	}
}

func TestHandleUpdateMemory(t *testing.T) {
	srv, store := setupTestServer(t)

	mem, err := store.Create(t.Context(), memory.CreateMemoryInput{
		Content: "original content",
	})
	if err != nil {
		t.Fatalf("failed to seed memory: %v", err)
	}

	newContent := "updated content"
	in := memory.UpdateMemoryInput{Content: &newContent}
	body, _ := json.Marshal(in)

	req := httptest.NewRequest(http.MethodPut, "/memories/"+mem.ID, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp memory.Memory
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.Content != newContent {
		t.Errorf("expected content %q, got %q", newContent, resp.Content)
	}
}

func TestHandleDeleteMemory(t *testing.T) {
	srv, store := setupTestServer(t)

	mem, err := store.Create(t.Context(), memory.CreateMemoryInput{
		Content: "memory to delete",
	})
	if err != nil {
		t.Fatalf("failed to seed memory: %v", err)
	}

	req := httptest.NewRequest(http.MethodDelete, "/memories/"+mem.ID, nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("expected status 204, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleSearchNilSearchSvc(t *testing.T) {
	logger := zap.NewNop()
	// Build server with no searchSvc.
	srv := New(logger, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, RuntimeInfo{})

	req := httptest.NewRequest(http.MethodGet, "/memories/search?query=test", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected status 503, got %d", w.Code)
	}
}
