package server

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"go.uber.org/zap"

	"github.com/jxroo/kairos/internal/inference"
	"github.com/jxroo/kairos/internal/memory"
)

// setupTestServerWithInference creates a Server with a real Store and a Manager
// whose Ollama provider is pointed at the given mock httptest.Server URL.
func setupTestServerWithInference(t *testing.T, mockOllamaURL string) (*Server, *memory.Store) {
	t.Helper()
	logger := zap.NewNop()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := memory.NewStore(dbPath, logger)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	t.Cleanup(func() { store.Close() })

	mgr := inference.NewManager(logger)
	p := inference.NewOllamaProvider(mockOllamaURL, logger)
	mgr.Register(p)

	assembler := inference.NewContextAssembler(nil, nil, logger)

	srv := New(logger, store, nil, nil, nil, nil, mgr, assembler, nil, nil, nil, nil, nil, RuntimeInfo{})
	return srv, store
}

// newMockOllamaServer creates a minimal httptest.Server that handles the Ollama
// /api/tags (models list) and /api/chat endpoints.
func newMockOllamaServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/tags":
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"models": []map[string]interface{}{
					{"name": "llama3:latest", "size": int64(1000), "details": map[string]interface{}{}, "model_info": nil},
				},
			})
		case "/api/chat":
			// Check whether the client wants streaming.
			var reqBody map[string]interface{}
			json.NewDecoder(r.Body).Decode(&reqBody)

			w.Header().Set("Content-Type", "application/json")
			if stream, _ := reqBody["stream"].(bool); stream {
				// Streaming response: newline-delimited JSON.
				chunks := []map[string]interface{}{
					{"model": "llama3:latest", "message": map[string]interface{}{"role": "assistant", "content": "Hello"}, "done": false},
					{"model": "llama3:latest", "message": map[string]interface{}{"role": "assistant", "content": "!"}, "done": true, "eval_count": 5, "prompt_eval_count": 3},
				}
				flusher, _ := w.(http.Flusher)
				for _, c := range chunks {
					data, _ := json.Marshal(c)
					fmt.Fprintf(w, "%s\n", data)
					if flusher != nil {
						flusher.Flush()
					}
				}
			} else {
				json.NewEncoder(w).Encode(map[string]interface{}{
					"model":             "llama3:latest",
					"message":           map[string]interface{}{"role": "assistant", "content": "Hi there!"},
					"done":              true,
					"eval_count":        10,
					"prompt_eval_count": 5,
				})
			}
		default:
			http.NotFound(w, r)
		}
	}))
}

// ---- Tests -----------------------------------------------------------------

func TestHandleListModelsNilManager(t *testing.T) {
	logger := zap.NewNop()
	srv := New(logger, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, RuntimeInfo{})

	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected status 503, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleListModels(t *testing.T) {
	mockSrv := newMockOllamaServer(t)
	defer mockSrv.Close()

	srv, _ := setupTestServerWithInference(t, mockSrv.URL)

	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp openAIModelsResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Object != "list" {
		t.Errorf("expected object=list, got %q", resp.Object)
	}
	if len(resp.Data) == 0 {
		t.Error("expected at least one model in data")
	}
	if resp.Data[0].Object != "model" {
		t.Errorf("expected data[0].object=model, got %q", resp.Data[0].Object)
	}
	if resp.Data[0].ID == "" {
		t.Error("expected non-empty model ID")
	}
	if resp.Data[0].ContextSize == 0 {
		t.Error("expected non-zero context length")
	}
	if len(resp.Data[0].Capabilities) == 0 || resp.Data[0].Capabilities[0] != "chat" {
		t.Errorf("expected chat capability, got %+v", resp.Data[0].Capabilities)
	}
}

func TestHandleChatCompletionsNilManager(t *testing.T) {
	logger := zap.NewNop()
	srv := New(logger, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, RuntimeInfo{})

	body, _ := json.Marshal(inference.ChatRequest{
		Model:    "llama3:latest",
		Messages: []inference.Message{{Role: "user", Content: "Hello"}},
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected status 503, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleChatCompletions(t *testing.T) {
	mockSrv := newMockOllamaServer(t)
	defer mockSrv.Close()

	srv, _ := setupTestServerWithInference(t, mockSrv.URL)

	body, _ := json.Marshal(inference.ChatRequest{
		Model:    "llama3:latest",
		Messages: []inference.Message{{Role: "user", Content: "Hello"}},
		Stream:   false,
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp openAIChatResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Object != "chat.completion" {
		t.Errorf("expected object=chat.completion, got %q", resp.Object)
	}
	if len(resp.Choices) == 0 {
		t.Fatal("expected at least one choice")
	}
	if resp.Choices[0].Message.Role != "assistant" {
		t.Errorf("expected role=assistant, got %q", resp.Choices[0].Message.Role)
	}
	if resp.Choices[0].Message.Content == "" {
		t.Error("expected non-empty content in response")
	}
	if resp.Choices[0].FinishReason != "stop" {
		t.Errorf("expected finish_reason=stop, got %q", resp.Choices[0].FinishReason)
	}
	if resp.ID == "" {
		t.Error("expected non-empty response ID")
	}

	// Check that X-Conversation-Id header is set.
	if convID := w.Header().Get("X-Conversation-Id"); convID == "" {
		t.Error("expected X-Conversation-Id header to be set")
	}
}

func TestHandleChatCompletionsInvalidJSON(t *testing.T) {
	mockSrv := newMockOllamaServer(t)
	defer mockSrv.Close()

	srv, _ := setupTestServerWithInference(t, mockSrv.URL)

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader("not-json"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}
}

func TestHandleChatCompletionsStream(t *testing.T) {
	mockSrv := newMockOllamaServer(t)
	defer mockSrv.Close()

	srv, _ := setupTestServerWithInference(t, mockSrv.URL)

	body, _ := json.Marshal(inference.ChatRequest{
		Model:    "llama3:latest",
		Messages: []inference.Message{{Role: "user", Content: "Hello"}},
		Stream:   true,
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	// Check content-type is SSE.
	ct := w.Header().Get("Content-Type")
	if ct != "text/event-stream" {
		t.Errorf("expected Content-Type=text/event-stream, got %q", ct)
	}

	// Verify the SSE body contains "data:" lines.
	body2 := w.Body.String()
	if !strings.Contains(body2, "data:") {
		t.Errorf("expected SSE data lines in body, got: %q", body2)
	}

	// Verify the final DONE marker.
	if !strings.Contains(body2, "[DONE]") {
		t.Errorf("expected [DONE] in SSE body, got: %q", body2)
	}

	// Parse SSE events and verify at least one has content.
	scanner := bufio.NewScanner(strings.NewReader(body2))
	var hasContent bool
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "data: ") && line != "data: [DONE]" {
			data := strings.TrimPrefix(line, "data: ")
			var chunk map[string]interface{}
			if err := json.Unmarshal([]byte(data), &chunk); err != nil {
				t.Errorf("failed to parse SSE chunk: %v", err)
				continue
			}
			choices, _ := chunk["choices"].([]interface{})
			if len(choices) > 0 {
				choice, _ := choices[0].(map[string]interface{})
				delta, _ := choice["delta"].(map[string]interface{})
				if content, _ := delta["content"].(string); content != "" {
					hasContent = true
				}
			}
		}
	}
	if !hasContent {
		t.Error("expected at least one SSE chunk with content")
	}
}

func TestHandleChatCompletionsWithConversationID(t *testing.T) {
	mockSrv := newMockOllamaServer(t)
	defer mockSrv.Close()

	srv, store := setupTestServerWithInference(t, mockSrv.URL)

	// Create a conversation first.
	conv, err := store.CreateConversation(t.Context(), "test", "llama3:latest")
	if err != nil {
		t.Fatalf("failed to create conversation: %v", err)
	}
	// Add a prior message.
	if err := store.AddMessage(t.Context(), conv.ID, memory.ConversationMessage{
		Role:    "user",
		Content: "Prior message",
	}); err != nil {
		t.Fatalf("failed to add message: %v", err)
	}

	body, _ := json.Marshal(inference.ChatRequest{
		Model:    "llama3:latest",
		Messages: []inference.Message{{Role: "user", Content: "Follow-up"}},
		Stream:   false,
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Conversation-Id", conv.ID)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	// The response should echo back the same conversation ID.
	respConvID := w.Header().Get("X-Conversation-Id")
	if respConvID != conv.ID {
		t.Errorf("expected X-Conversation-Id=%q, got %q", conv.ID, respConvID)
	}

	// Check messages were persisted.
	msgs, err := store.GetMessages(t.Context(), conv.ID)
	if err != nil {
		t.Fatalf("failed to get messages: %v", err)
	}
	// Should have: prior + follow-up user + assistant response = at least 3
	if len(msgs) < 3 {
		t.Errorf("expected at least 3 messages, got %d", len(msgs))
	}
}

func TestHandleChatCompletionsInvalidConversationIDStartsFresh(t *testing.T) {
	mockSrv := newMockOllamaServer(t)
	defer mockSrv.Close()

	srv, store := setupTestServerWithInference(t, mockSrv.URL)

	body, _ := json.Marshal(inference.ChatRequest{
		Messages: []inference.Message{{Role: "user", Content: "New thread"}},
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Conversation-Id", "missing-conversation")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	newConvID := w.Header().Get("X-Conversation-Id")
	if newConvID == "" || newConvID == "missing-conversation" {
		t.Fatalf("expected a fresh conversation ID, got %q", newConvID)
	}

	conv, err := store.GetConversation(t.Context(), newConvID)
	if err != nil {
		t.Fatalf("failed to load new conversation: %v", err)
	}
	if conv.Model != "llama3:latest" {
		t.Errorf("conversation model = %q, want llama3:latest", conv.Model)
	}

	msgs, err := store.GetMessages(t.Context(), newConvID)
	if err != nil {
		t.Fatalf("failed to load new messages: %v", err)
	}
	if len(msgs) != 2 {
		t.Fatalf("expected 2 persisted messages, got %d", len(msgs))
	}
}
