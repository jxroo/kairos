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

// testLogger returns a no-op zap logger suitable for unit tests.
func testLogger() *zap.Logger {
	return zap.NewNop()
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

// newTestServer starts a mock llama.cpp-compatible server and returns it along
// with a LlamaCppProvider already pointing at the server URL.
func newTestServer(t *testing.T, mux *http.ServeMux) (*httptest.Server, *LlamaCppProvider) {
	t.Helper()
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, NewLlamaCppProvider(srv.URL, testLogger())
}

func mustMarshal(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return b
}

// ---------------------------------------------------------------------------
// Chat tests
// ---------------------------------------------------------------------------

func TestLlamaCppChat(t *testing.T) {
	tests := []struct {
		name       string
		serverFunc http.HandlerFunc
		req        ChatRequest
		wantErr    bool
		check      func(t *testing.T, resp *ChatResponse)
	}{
		{
			name: "valid response",
			serverFunc: func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost || r.URL.Path != "/v1/chat/completions" {
					http.NotFound(w, r)
					return
				}
				var req ChatRequest
				if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
					http.Error(w, "bad request", http.StatusBadRequest)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				resp := llamaCppChatResponse{
					ID:    "chatcmpl-abc",
					Model: req.Model,
					Choices: []llamaCppChoice{
						{
							Index:        0,
							Message:      Message{Role: "assistant", Content: "Hello there!"},
							FinishReason: "stop",
						},
					},
					Usage: Usage{PromptTokens: 5, CompletionTokens: 3, TotalTokens: 8},
				}
				json.NewEncoder(w).Encode(resp)
			},
			req: ChatRequest{
				Model:    "default",
				Messages: []Message{{Role: "user", Content: "Hello"}},
			},
			check: func(t *testing.T, resp *ChatResponse) {
				if resp.ID != "chatcmpl-abc" {
					t.Errorf("ID = %q, want chatcmpl-abc", resp.ID)
				}
				if resp.Message.Role != "assistant" {
					t.Errorf("role = %q, want assistant", resp.Message.Role)
				}
				if resp.Message.Content != "Hello there!" {
					t.Errorf("content = %q, want 'Hello there!'", resp.Message.Content)
				}
				if resp.Usage.TotalTokens != 8 {
					t.Errorf("total tokens = %d, want 8", resp.Usage.TotalTokens)
				}
			},
		},
		{
			name: "server returns 500",
			serverFunc: func(w http.ResponseWriter, r *http.Request) {
				http.Error(w, "internal server error", http.StatusInternalServerError)
			},
			req:     ChatRequest{Model: "default", Messages: []Message{{Role: "user", Content: "Hi"}}},
			wantErr: true,
		},
		{
			name: "empty choices returns error",
			serverFunc: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				resp := llamaCppChatResponse{ID: "chatcmpl-empty", Choices: []llamaCppChoice{}}
				json.NewEncoder(w).Encode(resp)
			},
			req:     ChatRequest{Model: "default", Messages: []Message{{Role: "user", Content: "Hi"}}},
			wantErr: true,
		},
		{
			name: "malformed JSON returns error",
			serverFunc: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				fmt.Fprint(w, "{ not valid json }")
			},
			req:     ChatRequest{Model: "default", Messages: []Message{{Role: "user", Content: "Hi"}}},
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mux := http.NewServeMux()
			mux.HandleFunc("/v1/chat/completions", tc.serverFunc)
			_, provider := newTestServer(t, mux)

			resp, err := provider.Chat(context.Background(), tc.req)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tc.check != nil {
				tc.check(t, resp)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// ChatStream tests
// ---------------------------------------------------------------------------

func TestLlamaCppChatStream(t *testing.T) {
	tests := []struct {
		name        string
		serverFunc  http.HandlerFunc
		req         ChatRequest
		wantErr     bool // error from ChatStream() itself
		wantDeltas  []string
		wantDone    bool
		wantUsage   bool
	}{
		{
			name: "streaming incremental events",
			serverFunc: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				w.Header().Set("Cache-Control", "no-cache")
				flusher, _ := w.(http.Flusher)

				stop := "stop"
				chunks := []SSEChunk{
					{
						ID: "chatcmpl-1",
						Choices: []SSEChoice{
							{Index: 0, Delta: SSEDelta{Content: "Hello"}, FinishReason: nil},
						},
					},
					{
						ID: "chatcmpl-1",
						Choices: []SSEChoice{
							{Index: 0, Delta: SSEDelta{Content: " world"}, FinishReason: &stop},
						},
						Usage: &Usage{PromptTokens: 3, CompletionTokens: 2, TotalTokens: 5},
					},
				}

				for _, c := range chunks {
					b := mustMarshal(t, c)
					fmt.Fprintf(w, "data: %s\n\n", b)
					if flusher != nil {
						flusher.Flush()
					}
				}
				fmt.Fprint(w, "data: [DONE]\n\n")
				if flusher != nil {
					flusher.Flush()
				}
			},
			req:        ChatRequest{Model: "default", Messages: []Message{{Role: "user", Content: "Hi"}}},
			wantDeltas: []string{"Hello", " world"},
			wantDone:   true,
			wantUsage:  true,
		},
		{
			name: "server returns 503",
			serverFunc: func(w http.ResponseWriter, r *http.Request) {
				http.Error(w, "service unavailable", http.StatusServiceUnavailable)
			},
			req:     ChatRequest{Model: "default", Messages: []Message{{Role: "user", Content: "Hi"}}},
			wantErr: true,
		},
		{
			name: "stream with only DONE sentinel",
			serverFunc: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				fmt.Fprint(w, "data: [DONE]\n\n")
			},
			req:        ChatRequest{Model: "default", Messages: []Message{{Role: "user", Content: "Hi"}}},
			wantDeltas: []string{},
		},
		{
			name: "non-data SSE lines are ignored",
			serverFunc: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				// comment line and empty line before actual data
				fmt.Fprint(w, ": keep-alive\n\n")
				stop := "stop"
				chunk := SSEChunk{
					ID: "chatcmpl-2",
					Choices: []SSEChoice{
						{Index: 0, Delta: SSEDelta{Content: "Hi"}, FinishReason: &stop},
					},
				}
				b := mustMarshal(t, chunk)
				fmt.Fprintf(w, "data: %s\n\n", b)
				fmt.Fprint(w, "data: [DONE]\n\n")
			},
			req:        ChatRequest{Model: "default", Messages: []Message{{Role: "user", Content: "Hey"}}},
			wantDeltas: []string{"Hi"},
			wantDone:   true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mux := http.NewServeMux()
			mux.HandleFunc("/v1/chat/completions", tc.serverFunc)
			_, provider := newTestServer(t, mux)

			reader, err := provider.ChatStream(context.Background(), tc.req)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error from ChatStream, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			var deltas []string
			var sawDone bool
			var sawUsage bool
			for {
				ev, ok := reader.Next()
				if !ok {
					break
				}
				deltas = append(deltas, ev.Delta)
				if ev.Done {
					sawDone = true
					if ev.Usage != nil {
						sawUsage = true
					}
				}
			}
			if err := reader.Err(); err != nil {
				t.Fatalf("stream error: %v", err)
			}

			if len(deltas) != len(tc.wantDeltas) {
				t.Fatalf("got %d deltas, want %d: %v", len(deltas), len(tc.wantDeltas), deltas)
			}
			for i, want := range tc.wantDeltas {
				if deltas[i] != want {
					t.Errorf("delta[%d] = %q, want %q", i, deltas[i], want)
				}
			}
			if tc.wantDone && !sawDone {
				t.Error("expected a Done event, saw none")
			}
			if tc.wantUsage && !sawUsage {
				t.Error("expected usage in final event, saw none")
			}
		})
	}
}

// ---------------------------------------------------------------------------
// ListModels tests
// ---------------------------------------------------------------------------

func TestLlamaCppListModels(t *testing.T) {
	tests := []struct {
		name       string
		serverFunc http.HandlerFunc
		wantErr    bool
		wantIDs    []string
	}{
		{
			name: "returns single model",
			serverFunc: func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/v1/models" {
					http.NotFound(w, r)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				resp := llamaCppModelsResponse{
					Object: "list",
					Data: []llamaCppModel{
						{ID: "default", Object: "model", OwnedBy: "llama.cpp"},
					},
				}
				json.NewEncoder(w).Encode(resp)
			},
			wantIDs: []string{"default"},
		},
		{
			name: "returns multiple models",
			serverFunc: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				resp := llamaCppModelsResponse{
					Object: "list",
					Data: []llamaCppModel{
						{ID: "llama-3", Object: "model", OwnedBy: "llama.cpp"},
						{ID: "mistral-7b", Object: "model", OwnedBy: "llama.cpp"},
					},
				}
				json.NewEncoder(w).Encode(resp)
			},
			wantIDs: []string{"llama-3", "mistral-7b"},
		},
		{
			name: "returns empty list",
			serverFunc: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				resp := llamaCppModelsResponse{Object: "list", Data: []llamaCppModel{}}
				json.NewEncoder(w).Encode(resp)
			},
			wantIDs: []string{},
		},
		{
			name: "server returns 500",
			serverFunc: func(w http.ResponseWriter, r *http.Request) {
				http.Error(w, "server error", http.StatusInternalServerError)
			},
			wantErr: true,
		},
		{
			name: "malformed JSON",
			serverFunc: func(w http.ResponseWriter, r *http.Request) {
				fmt.Fprint(w, "not-json")
			},
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mux := http.NewServeMux()
			mux.HandleFunc("/v1/models", tc.serverFunc)
			_, provider := newTestServer(t, mux)

			models, err := provider.ListModels(context.Background())
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if len(models) != len(tc.wantIDs) {
				t.Fatalf("got %d models, want %d", len(models), len(tc.wantIDs))
			}
			for i, wantID := range tc.wantIDs {
				if models[i].ID != wantID {
					t.Errorf("models[%d].ID = %q, want %q", i, models[i].ID, wantID)
				}
				if models[i].Provider != "llamacpp" {
					t.Errorf("models[%d].Provider = %q, want llamacpp", i, models[i].Provider)
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Ping tests
// ---------------------------------------------------------------------------

func TestLlamaCppPing(t *testing.T) {
	tests := []struct {
		name       string
		serverFunc http.HandlerFunc
		wantErr    bool
	}{
		{
			name: "ping succeeds when models endpoint is healthy",
			serverFunc: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(llamaCppModelsResponse{
					Object: "list",
					Data:   []llamaCppModel{{ID: "default"}},
				})
			},
			wantErr: false,
		},
		{
			name: "ping fails when server returns 500",
			serverFunc: func(w http.ResponseWriter, r *http.Request) {
				http.Error(w, "down", http.StatusInternalServerError)
			},
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mux := http.NewServeMux()
			mux.HandleFunc("/v1/models", tc.serverFunc)
			_, provider := newTestServer(t, mux)

			err := provider.Ping(context.Background())
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Unreachable server tests
// ---------------------------------------------------------------------------

func TestLlamaCppUnreachable(t *testing.T) {
	// Use a provider pointing at a port that nothing is listening on.
	provider := NewLlamaCppProvider("http://127.0.0.1:19999", testLogger())
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	t.Run("Chat returns error when unreachable", func(t *testing.T) {
		_, err := provider.Chat(ctx, ChatRequest{
			Model:    "default",
			Messages: []Message{{Role: "user", Content: "Hi"}},
		})
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("ChatStream returns error when unreachable", func(t *testing.T) {
		_, err := provider.ChatStream(ctx, ChatRequest{
			Model:    "default",
			Messages: []Message{{Role: "user", Content: "Hi"}},
		})
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("ListModels returns error when unreachable", func(t *testing.T) {
		_, err := provider.ListModels(ctx)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("Ping returns error when unreachable", func(t *testing.T) {
		err := provider.Ping(ctx)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})
}

// ---------------------------------------------------------------------------
// Provider interface / constructor tests
// ---------------------------------------------------------------------------

func TestLlamaCppProviderName(t *testing.T) {
	p := NewLlamaCppProvider("http://localhost:8080", testLogger())
	if p.Name() != "llamacpp" {
		t.Errorf("Name() = %q, want llamacpp", p.Name())
	}
}

func TestNewLlamaCppProviderTrimsTrailingSlash(t *testing.T) {
	p := NewLlamaCppProvider("http://localhost:8080/", testLogger())
	if strings.HasSuffix(p.baseURL, "/") {
		t.Errorf("baseURL should not have trailing slash, got %q", p.baseURL)
	}
}
