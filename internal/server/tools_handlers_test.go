package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.uber.org/zap"
)

func TestHandleExecuteToolErrorIsValidJSON(t *testing.T) {
	logger := zap.NewNop()
	// Server with no executor — returns service unavailable, but let's test
	// the JSON injection fix by using a server with nil executor.
	srv := New(logger, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, RuntimeInfo{})

	body := `{"name":"nonexistent","arguments":{}}`
	req := httptest.NewRequest(http.MethodPost, "/tools/execute", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	// Without an executor, it returns 503.
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", w.Code)
	}

	// Verify the response is valid JSON.
	var resp map[string]string
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Errorf("response is not valid JSON: %v, body: %s", err, w.Body.String())
	}
}

func TestBodySizeLimitRejectsLargePayload(t *testing.T) {
	logger := zap.NewNop()
	srv := New(logger, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, RuntimeInfo{})

	// Create a body larger than 1MB.
	largeBody := bytes.Repeat([]byte("x"), 2<<20) // 2MB
	req := httptest.NewRequest(http.MethodPost, "/memories", bytes.NewReader(largeBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	// The server should reject this — either 413 or 400 depending on how
	// MaxBytesReader triggers. The key point is it's not 200/201.
	if w.Code == http.StatusOK || w.Code == http.StatusCreated {
		t.Errorf("expected rejection of >1MB body, got status %d", w.Code)
	}
}
