package inference

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"
)

// newTestLogger returns a no-op zap logger suitable for tests.
func newTestLogger() *zap.Logger {
	return zap.NewNop()
}

// --- helpers ----------------------------------------------------------------

func ollamaTagsHandler(models []ollamaModelEntry) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(ollamaTagsResponse{Models: models})
	}
}

// --- TestOllamaChat --------------------------------------------------------

func TestOllamaChat(t *testing.T) {
	tests := []struct {
		name        string
		serverResp  ollamaChatResponse
		serverCode  int
		wantContent string
		wantErr     bool
	}{
		{
			name: "valid response",
			serverResp: ollamaChatResponse{
				Model:           "llama3",
				Message:         ollamaMessage{Role: "assistant", Content: "Hello!"},
				Done:            true,
				EvalCount:       10,
				PromptEvalCount: 5,
			},
			serverCode:  http.StatusOK,
			wantContent: "Hello!",
		},
		{
			name:       "server error",
			serverCode: http.StatusInternalServerError,
			wantErr:    true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/api/chat" {
					http.NotFound(w, r)
					return
				}
				w.WriteHeader(tc.serverCode)
				if tc.serverCode == http.StatusOK {
					_ = json.NewEncoder(w).Encode(tc.serverResp)
				}
			}))
			defer srv.Close()

			p := NewOllamaProvider(srv.URL, newTestLogger())
			resp, err := p.Chat(context.Background(), ChatRequest{
				Model:    "llama3",
				Messages: []Message{{Role: "user", Content: "Hi"}},
			})

			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if resp.Message.Content != tc.wantContent {
				t.Errorf("content = %q, want %q", resp.Message.Content, tc.wantContent)
			}
			if resp.Message.Role != "assistant" {
				t.Errorf("role = %q, want %q", resp.Message.Role, "assistant")
			}
			if resp.Usage.PromptTokens != tc.serverResp.PromptEvalCount {
				t.Errorf("prompt tokens = %d, want %d", resp.Usage.PromptTokens, tc.serverResp.PromptEvalCount)
			}
			if resp.Usage.CompletionTokens != tc.serverResp.EvalCount {
				t.Errorf("completion tokens = %d, want %d", resp.Usage.CompletionTokens, tc.serverResp.EvalCount)
			}
			if resp.Usage.TotalTokens != tc.serverResp.EvalCount+tc.serverResp.PromptEvalCount {
				t.Errorf("total tokens = %d, want %d", resp.Usage.TotalTokens, tc.serverResp.EvalCount+tc.serverResp.PromptEvalCount)
			}
		})
	}
}

func TestOllamaChatUsage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(ollamaChatResponse{
			Model:           "llama3",
			Message:         ollamaMessage{Role: "assistant", Content: "42"},
			Done:            true,
			EvalCount:       7,
			PromptEvalCount: 3,
		})
	}))
	defer srv.Close()

	p := NewOllamaProvider(srv.URL, newTestLogger())
	resp, err := p.Chat(context.Background(), ChatRequest{
		Model:    "llama3",
		Messages: []Message{{Role: "user", Content: "?"}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Usage.TotalTokens != 10 {
		t.Errorf("total tokens = %d, want 10", resp.Usage.TotalTokens)
	}
}

// --- TestOllamaChatStream --------------------------------------------------

func TestOllamaChatStream(t *testing.T) {
	chunks := []ollamaChatResponse{
		{Model: "llama3", Message: ollamaMessage{Role: "assistant", Content: "Hel"}, Done: false},
		{Model: "llama3", Message: ollamaMessage{Role: "assistant", Content: "lo"}, Done: false},
		{Model: "llama3", Message: ollamaMessage{Role: "assistant", Content: "!"}, Done: true, EvalCount: 3, PromptEvalCount: 5},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/chat" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/x-ndjson")
		flusher, _ := w.(http.Flusher)
		for _, c := range chunks {
			data, _ := json.Marshal(c)
			_, _ = fmt.Fprintf(w, "%s\n", data)
			if flusher != nil {
				flusher.Flush()
			}
		}
	}))
	defer srv.Close()

	p := NewOllamaProvider(srv.URL, newTestLogger())
	reader, err := p.ChatStream(context.Background(), ChatRequest{
		Model:    "llama3",
		Messages: []Message{{Role: "user", Content: "Hello"}},
		Stream:   true,
	})
	if err != nil {
		t.Fatalf("ChatStream error: %v", err)
	}

	var deltas []string
	var lastUsage *Usage
	for {
		ev, ok := reader.Next()
		if !ok {
			break
		}
		deltas = append(deltas, ev.Delta)
		if ev.Done {
			lastUsage = ev.Usage
		}
	}
	if err := reader.Err(); err != nil {
		t.Fatalf("stream error: %v", err)
	}

	want := []string{"Hel", "lo", "!"}
	if len(deltas) != len(want) {
		t.Fatalf("got %d deltas, want %d: %v", len(deltas), len(want), deltas)
	}
	for i, d := range deltas {
		if d != want[i] {
			t.Errorf("delta[%d] = %q, want %q", i, d, want[i])
		}
	}

	if lastUsage == nil {
		t.Fatal("expected usage on final event, got nil")
	}
	if lastUsage.CompletionTokens != 3 {
		t.Errorf("completion tokens = %d, want 3", lastUsage.CompletionTokens)
	}
	if lastUsage.PromptTokens != 5 {
		t.Errorf("prompt tokens = %d, want 5", lastUsage.PromptTokens)
	}
	if lastUsage.TotalTokens != 8 {
		t.Errorf("total tokens = %d, want 8", lastUsage.TotalTokens)
	}
}

func TestOllamaChatStreamIncrementalText(t *testing.T) {
	chunks := []ollamaChatResponse{
		{Model: "m", Message: ollamaMessage{Role: "assistant", Content: "A"}, Done: false},
		{Model: "m", Message: ollamaMessage{Role: "assistant", Content: "B"}, Done: false},
		{Model: "m", Message: ollamaMessage{Role: "assistant", Content: "C"}, Done: true},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for _, c := range chunks {
			data, _ := json.Marshal(c)
			_, _ = fmt.Fprintf(w, "%s\n", data)
		}
	}))
	defer srv.Close()

	p := NewOllamaProvider(srv.URL, newTestLogger())
	reader, err := p.ChatStream(context.Background(), ChatRequest{
		Model:    "m",
		Messages: []Message{{Role: "user", Content: "?"}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var sb strings.Builder
	for {
		ev, ok := reader.Next()
		if !ok {
			break
		}
		sb.WriteString(ev.Delta)
	}
	if reader.Err() != nil {
		t.Fatalf("stream error: %v", reader.Err())
	}
	if sb.String() != "ABC" {
		t.Errorf("concatenated = %q, want %q", sb.String(), "ABC")
	}
}

func TestOllamaChatStreamServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal error", http.StatusInternalServerError)
	}))
	defer srv.Close()

	p := NewOllamaProvider(srv.URL, newTestLogger())
	_, err := p.ChatStream(context.Background(), ChatRequest{
		Model:    "m",
		Messages: []Message{{Role: "user", Content: "?"}},
	})
	if err == nil {
		t.Fatal("expected error for server error, got nil")
	}
}

// --- TestOllamaListModels -------------------------------------------------

func TestOllamaListModels(t *testing.T) {
	models := []ollamaModelEntry{
		{
			Name:    "llama3:latest",
			Size:    4661224960,
			Details: ollamaModelDetail{ParameterSize: "8B"},
			ModelInfo: map[string]any{
				"general.context_length": float64(8192),
			},
		},
		{
			Name:      "mistral:latest",
			Size:      1234567890,
			Details:   ollamaModelDetail{ParameterSize: "7B"},
			ModelInfo: nil,
		},
	}

	srv := httptest.NewServer(ollamaTagsHandler(models))
	defer srv.Close()

	p := NewOllamaProvider(srv.URL, newTestLogger())
	got, err := p.ListModels(context.Background())
	if err != nil {
		t.Fatalf("ListModels error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d models, want 2", len(got))
	}

	tests := []struct {
		idx          int
		wantID       string
		wantCtx      int
		wantSize     int64
		wantProvider string
		wantCaps     []string
	}{
		{0, "llama3:latest", 8192, 4661224960, "ollama", []string{"chat"}},
		{1, "mistral:latest", ollamaDefaultContext, 1234567890, "ollama", []string{"chat"}},
	}
	for _, tc := range tests {
		m := got[tc.idx]
		if m.ID != tc.wantID {
			t.Errorf("[%d] ID = %q, want %q", tc.idx, m.ID, tc.wantID)
		}
		if m.ContextSize != tc.wantCtx {
			t.Errorf("[%d] ContextSize = %d, want %d", tc.idx, m.ContextSize, tc.wantCtx)
		}
		if m.SizeBytes != tc.wantSize {
			t.Errorf("[%d] SizeBytes = %d, want %d", tc.idx, m.SizeBytes, tc.wantSize)
		}
		if m.Provider != tc.wantProvider {
			t.Errorf("[%d] Provider = %q, want %q", tc.idx, m.Provider, tc.wantProvider)
		}
		if len(m.Capabilities) != len(tc.wantCaps) {
			t.Errorf("[%d] Capabilities len = %d, want %d", tc.idx, len(m.Capabilities), len(tc.wantCaps))
		}
		for i, cap := range tc.wantCaps {
			if i >= len(m.Capabilities) || m.Capabilities[i] != cap {
				t.Errorf("[%d] Capability[%d] = %q, want %q", tc.idx, i, m.Capabilities[i], cap)
			}
		}
	}
}

func TestOllamaListModelsServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer srv.Close()

	p := NewOllamaProvider(srv.URL, newTestLogger())
	_, err := p.ListModels(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// --- TestOllamaPing -------------------------------------------------------

func TestOllamaPing(t *testing.T) {
	tests := []struct {
		name       string
		serverCode int
		wantErr    bool
	}{
		{"reachable", http.StatusOK, false},
		{"server error", http.StatusInternalServerError, true},
		{"not found", http.StatusNotFound, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if tc.serverCode == http.StatusOK {
					_ = json.NewEncoder(w).Encode(ollamaTagsResponse{})
				} else {
					http.Error(w, "error", tc.serverCode)
				}
			}))
			defer srv.Close()

			p := NewOllamaProvider(srv.URL, newTestLogger())
			err := p.Ping(context.Background())
			if tc.wantErr && err == nil {
				t.Error("expected error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestOllamaPingUnreachable(t *testing.T) {
	// Use a port that is not listening.
	p := NewOllamaProvider("http://127.0.0.1:19999", newTestLogger())
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	err := p.Ping(ctx)
	if err == nil {
		t.Fatal("expected error for unreachable Ollama, got nil")
	}
	// Must not panic — reaching here means the test passed.
}

// --- TestOllamaUnreachable ------------------------------------------------

func TestOllamaChatUnreachable(t *testing.T) {
	p := NewOllamaProvider("http://127.0.0.1:19998", newTestLogger())
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	_, err := p.Chat(ctx, ChatRequest{
		Model:    "llama3",
		Messages: []Message{{Role: "user", Content: "hi"}},
	})
	if err == nil {
		t.Fatal("expected error for unreachable Ollama, got nil")
	}
}

func TestOllamaChatStreamUnreachable(t *testing.T) {
	p := NewOllamaProvider("http://127.0.0.1:19997", newTestLogger())
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	_, err := p.ChatStream(ctx, ChatRequest{
		Model:    "llama3",
		Messages: []Message{{Role: "user", Content: "hi"}},
	})
	if err == nil {
		t.Fatal("expected error for unreachable Ollama, got nil")
	}
}

// --- TestOllamaName -------------------------------------------------------

func TestOllamaName(t *testing.T) {
	p := NewOllamaProvider("", newTestLogger())
	if p.Name() != "ollama" {
		t.Errorf("Name() = %q, want %q", p.Name(), "ollama")
	}
}

// --- TestOllamaDefaultURL -------------------------------------------------

func TestOllamaDefaultURL(t *testing.T) {
	p := NewOllamaProvider("", newTestLogger())
	if p.url != ollamaDefaultURL {
		t.Errorf("url = %q, want %q", p.url, ollamaDefaultURL)
	}
}

// --- TestOllamaProviderInterface ------------------------------------------

// Compile-time check that *OllamaProvider satisfies Provider.
var _ Provider = (*OllamaProvider)(nil)

// --- TestExtractContextSize -----------------------------------------------

func TestExtractContextSize(t *testing.T) {
	tests := []struct {
		name    string
		info    map[string]any
		wantCtx int
	}{
		{"nil map", nil, ollamaDefaultContext},
		{"missing key", map[string]any{"other": "val"}, ollamaDefaultContext},
		{"float64 value", map[string]any{"general.context_length": float64(8192)}, 8192},
		{"zero value", map[string]any{"general.context_length": float64(0)}, ollamaDefaultContext},
		{"negative value", map[string]any{"general.context_length": float64(-1)}, ollamaDefaultContext},
		{"int value", map[string]any{"general.context_length": int(16384)}, 16384},
		{"int64 value", map[string]any{"general.context_length": int64(32768)}, 32768},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := extractContextSize(tc.info)
			if got != tc.wantCtx {
				t.Errorf("extractContextSize = %d, want %d", got, tc.wantCtx)
			}
		})
	}
}

// --- TestOllamaDiscover ---------------------------------------------------

func TestOllamaDiscover(t *testing.T) {
	t.Run("configured URL reachable", func(t *testing.T) {
		srv := httptest.NewServer(ollamaTagsHandler(nil))
		defer srv.Close()

		p := NewOllamaProvider(srv.URL, newTestLogger())
		url, err := p.Discover(context.Background(), false)
		if err != nil {
			t.Fatalf("Discover error: %v", err)
		}
		if url != srv.URL {
			t.Errorf("url = %q, want %q", url, srv.URL)
		}
	})

	t.Run("configured URL unreachable returns error", func(t *testing.T) {
		p := NewOllamaProvider("http://127.0.0.1:19996", newTestLogger())
		_, err := p.Discover(context.Background(), false)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})
}

func TestDiscoveryCandidates(t *testing.T) {
	got := discoveryCandidates("http://custom-host:11434/")
	want := []string{
		"http://custom-host:11434",
		"http://localhost:11434",
		"http://127.0.0.1:11434",
		"http://[::1]:11434",
		"http://ollama.local:11434",
	}

	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d: %v", len(got), len(want), got)
	}
	for i, candidate := range want {
		if got[i] != candidate {
			t.Errorf("[%d] = %q, want %q", i, got[i], candidate)
		}
	}
}
