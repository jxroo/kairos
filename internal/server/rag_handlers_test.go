package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/jxroo/kairos/internal/rag"
)

type fakeDocumentLister struct {
	docs []rag.Document
}

func (f fakeDocumentLister) ListDocuments(_ context.Context) ([]rag.Document, error) {
	return f.docs, nil
}

func TestHandleIndexStatusNilProgress(t *testing.T) {
	logger := zap.NewNop()
	srv := New(logger, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, RuntimeInfo{})

	req := httptest.NewRequest(http.MethodGet, "/index/status", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", w.Code)
	}
}

func TestHandleIndexStatusWithProgress(t *testing.T) {
	logger := zap.NewNop()
	progress := rag.NewProgress()
	progress.Begin("indexing", 10)
	progress.RecordIndexed()
	progress.RecordIndexed()

	srv := New(logger, nil, nil, nil, nil, progress, nil, nil, nil, nil, nil, nil, nil, RuntimeInfo{})

	req := httptest.NewRequest(http.MethodGet, "/index/status", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var status rag.IndexStatus
	if err := json.NewDecoder(w.Body).Decode(&status); err != nil {
		t.Fatalf("failed to decode: %v", err)
	}
	if status.TotalFiles != 10 {
		t.Errorf("expected 10 total, got %d", status.TotalFiles)
	}
	if status.IndexedFiles != 2 {
		t.Errorf("expected 2 indexed, got %d", status.IndexedFiles)
	}
	if status.Percent != 20 {
		t.Errorf("expected 20 percent, got %d", status.Percent)
	}
}

func TestHandleIndexRebuildNil(t *testing.T) {
	logger := zap.NewNop()
	srv := New(logger, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, RuntimeInfo{})

	req := httptest.NewRequest(http.MethodPost, "/index/rebuild", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", w.Code)
	}
}

func TestHandleIndexRebuild(t *testing.T) {
	logger := zap.NewNop()
	srv := New(logger, nil, nil, nil, nil, rag.NewProgress(), nil, nil, nil, nil, nil, nil, nil, RuntimeInfo{})
	var called atomic.Bool
	done := make(chan struct{})
	srv.SetRebuildFunc(func() error {
		called.Store(true)
		close(done)
		return nil
	})

	req := httptest.NewRequest(http.MethodPost, "/index/rebuild", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusAccepted {
		t.Errorf("expected 202, got %d", w.Code)
	}

	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("expected rebuild function to be called")
	}

	if !called.Load() {
		t.Fatal("expected rebuild function invocation to be observed")
	}
}

func TestHandleSearchDocumentsNil(t *testing.T) {
	logger := zap.NewNop()
	srv := New(logger, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, RuntimeInfo{})

	req := httptest.NewRequest(http.MethodGet, "/search/documents?query=test", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", w.Code)
	}
}

func TestHandleListDocuments(t *testing.T) {
	logger := zap.NewNop()
	srv := New(logger, nil, nil, nil, fakeDocumentLister{docs: []rag.Document{
		{ID: "1", Filename: "a.md", Path: "/tmp/a.md", Status: rag.StatusIndexed},
		{ID: "2", Filename: "b.md", Path: "/tmp/b.md", Status: rag.StatusError},
		{ID: "3", Filename: "c.md", Path: "/tmp/c.md", Status: rag.StatusIndexed},
	}}, nil, nil, nil, nil, nil, nil, nil, nil, RuntimeInfo{})

	req := httptest.NewRequest(http.MethodGet, "/documents?status=indexed&limit=1&offset=1", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var docs []rag.Document
	if err := json.NewDecoder(w.Body).Decode(&docs); err != nil {
		t.Fatalf("decoding response: %v", err)
	}
	if len(docs) != 1 || docs[0].ID != "3" {
		t.Fatalf("unexpected document slice: %+v", docs)
	}
}
