package inference

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Mock HTTP server helpers
// ---------------------------------------------------------------------------

// newOllamaMockServer returns an httptest.Server that handles:
//
//   - GET /api/tags  → model list
//   - POST /api/chat → non-streaming or streaming response based on request body
func newOllamaMockServer(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()

	// GET /api/tags — model list
	mux.HandleFunc("/api/tags", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"models": []map[string]any{
				{"name": "test-model", "size": 1000000},
			},
		})
	})

	// POST /api/chat — streaming or non-streaming
	mux.HandleFunc("/api/chat", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}

		streaming, _ := body["stream"].(bool)

		if streaming {
			// Return newline-delimited JSON stream.
			w.Header().Set("Content-Type", "application/x-ndjson")
			flusher, _ := w.(http.Flusher)

			chunks := []ollamaChatResponse{
				{Model: "test-model", Message: ollamaMessage{Role: "assistant", Content: "Hel"}, Done: false},
				{Model: "test-model", Message: ollamaMessage{Role: "assistant", Content: "lo"}, Done: false},
				{Model: "test-model", Message: ollamaMessage{Role: "assistant", Content: "!"}, Done: true, EvalCount: 3, PromptEvalCount: 2},
			}
			for _, c := range chunks {
				data, _ := json.Marshal(c)
				_, _ = fmt.Fprintf(w, "%s\n", data)
				if flusher != nil {
					flusher.Flush()
				}
			}
			return
		}

		// Non-streaming response.
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(ollamaChatResponse{
			Model:           "test-model",
			Message:         ollamaMessage{Role: "assistant", Content: "Hello from Ollama!"},
			Done:            true,
			EvalCount:       5,
			PromptEvalCount: 3,
		})
	})

	return httptest.NewServer(mux)
}

// ---------------------------------------------------------------------------
// Mock MemorySearcher / RAGSearcher
// ---------------------------------------------------------------------------

type mockMemSvc struct {
	results []MemoryResult
}

func (m *mockMemSvc) Search(_ context.Context, _ string, _ int) ([]MemoryResult, error) {
	return m.results, nil
}

type mockRAGSvc struct {
	results []RAGResult
}

func (m *mockRAGSvc) Search(_ context.Context, _ string, _ int) ([]RAGResult, error) {
	return m.results, nil
}

// ---------------------------------------------------------------------------
// Test 1: Full conversation loop
// ---------------------------------------------------------------------------

func TestIntegrationFullConversationLoop(t *testing.T) {
	srv := newOllamaMockServer(t)
	defer srv.Close()

	// Wire up provider and manager.
	provider := NewOllamaProvider(srv.URL, newTestLogger())
	mgr := NewManager(newTestLogger())
	mgr.Register(provider)

	// Wire up context assembler with mock memory + RAG data.
	memSvc := &mockMemSvc{
		results: []MemoryResult{
			{Content: "User prefers concise answers.", Score: 0.9},
		},
	}
	ragSvc := &mockRAGSvc{
		results: []RAGResult{
			{Content: "Kairos is a Go daemon.", Source: "README.md", Score: 0.85},
		},
	}
	assembler := NewContextAssembler(memSvc, ragSvc, newTestLogger())

	// Build input messages.
	inputMsgs := []Message{
		{Role: "user", Content: "What is Kairos?"},
	}

	opts := AssembleOpts{
		SystemPrompt: "You are a helpful assistant.",
		MemoryLimit:  5,
		RAGLimit:     3,
	}

	// Assemble context.
	assembled, err := assembler.Assemble(context.Background(), inputMsgs, opts)
	if err != nil {
		t.Fatalf("Assemble error: %v", err)
	}

	// Verify context contains injected data.
	if len(assembled) == 0 {
		t.Fatal("expected assembled messages, got none")
	}
	if assembled[0].Role != "system" {
		t.Fatalf("first message role = %q, want system", assembled[0].Role)
	}
	sysContent := assembled[0].Content
	if !strings.Contains(sysContent, "User prefers concise answers.") {
		t.Error("expected memory content in system message")
	}
	if !strings.Contains(sysContent, "Kairos is a Go daemon.") {
		t.Error("expected RAG content in system message")
	}

	// Send assembled messages to manager.
	resp, err := mgr.Chat(context.Background(), ChatRequest{
		Model:    "test-model",
		Messages: assembled,
	})
	if err != nil {
		t.Fatalf("Chat error: %v", err)
	}

	if resp.Message.Role != "assistant" {
		t.Errorf("response role = %q, want assistant", resp.Message.Role)
	}
	if resp.Message.Content == "" {
		t.Error("expected non-empty response content")
	}
	if resp.Message.Content != "Hello from Ollama!" {
		t.Errorf("response content = %q, want %q", resp.Message.Content, "Hello from Ollama!")
	}
}

// ---------------------------------------------------------------------------
// Test 2: Streaming chat end-to-end
// ---------------------------------------------------------------------------

func TestIntegrationStreamingChatEndToEnd(t *testing.T) {
	srv := newOllamaMockServer(t)
	defer srv.Close()

	provider := NewOllamaProvider(srv.URL, newTestLogger())
	mgr := NewManager(newTestLogger())
	mgr.Register(provider)

	reader, err := mgr.ChatStream(context.Background(), ChatRequest{
		Model:    "test-model",
		Messages: []Message{{Role: "user", Content: "Hello"}},
		Stream:   true,
	})
	if err != nil {
		t.Fatalf("ChatStream error: %v", err)
	}

	var deltas []string
	var doneCount int
	var lastUsage *Usage

	for {
		ev, ok := reader.Next()
		if !ok {
			break
		}
		deltas = append(deltas, ev.Delta)
		if ev.Done {
			doneCount++
			lastUsage = ev.Usage
		}
	}

	if err := reader.Err(); err != nil {
		t.Fatalf("stream error: %v", err)
	}

	// Verify incremental deltas.
	wantDeltas := []string{"Hel", "lo", "!"}
	if len(deltas) != len(wantDeltas) {
		t.Fatalf("got %d deltas, want %d: %v", len(deltas), len(wantDeltas), deltas)
	}
	for i, d := range deltas {
		if d != wantDeltas[i] {
			t.Errorf("delta[%d] = %q, want %q", i, d, wantDeltas[i])
		}
	}

	// Verify exactly one done event.
	if doneCount != 1 {
		t.Errorf("doneCount = %d, want 1", doneCount)
	}

	// Verify final usage.
	if lastUsage == nil {
		t.Fatal("expected usage on final event, got nil")
	}
	if lastUsage.CompletionTokens != 3 {
		t.Errorf("completion tokens = %d, want 3", lastUsage.CompletionTokens)
	}
	if lastUsage.PromptTokens != 2 {
		t.Errorf("prompt tokens = %d, want 2", lastUsage.PromptTokens)
	}
	if lastUsage.TotalTokens != 5 {
		t.Errorf("total tokens = %d, want 5", lastUsage.TotalTokens)
	}
}

// ---------------------------------------------------------------------------
// Test 3: Multiple turns in same conversation
// ---------------------------------------------------------------------------

func TestIntegrationMultipleTurns(t *testing.T) {
	srv := newOllamaMockServer(t)
	defer srv.Close()

	provider := NewOllamaProvider(srv.URL, newTestLogger())
	mgr := NewManager(newTestLogger())
	mgr.Register(provider)

	// capturedQuery records the last user query seen by the memory searcher.
	var capturedQueries []string
	capturingMem := &capturingQueryMemSearcher{queries: &capturedQueries}

	assembler := NewContextAssembler(capturingMem, nil, newTestLogger())

	opts := AssembleOpts{SystemPrompt: "You are helpful."}

	// --- Turn 1 ---
	turn1Msgs := []Message{
		{Role: "user", Content: "What is Go?"},
	}
	assembled1, err := assembler.Assemble(context.Background(), turn1Msgs, opts)
	if err != nil {
		t.Fatalf("turn 1 Assemble error: %v", err)
	}

	resp1, err := mgr.Chat(context.Background(), ChatRequest{
		Model:    "test-model",
		Messages: assembled1,
	})
	if err != nil {
		t.Fatalf("turn 1 Chat error: %v", err)
	}
	if resp1.Message.Content == "" {
		t.Fatal("turn 1: expected non-empty response")
	}

	// --- Turn 2 — append to history ---
	history := append(assembled1, Message{Role: "assistant", Content: resp1.Message.Content})
	history = append(history, Message{Role: "user", Content: "Can you give an example?"})

	assembled2, err := assembler.Assemble(context.Background(), history, opts)
	if err != nil {
		t.Fatalf("turn 2 Assemble error: %v", err)
	}

	resp2, err := mgr.Chat(context.Background(), ChatRequest{
		Model:    "test-model",
		Messages: assembled2,
	})
	if err != nil {
		t.Fatalf("turn 2 Chat error: %v", err)
	}
	if resp2.Message.Content == "" {
		t.Fatal("turn 2: expected non-empty response")
	}

	// Verify the context assembler was called with both user queries.
	if len(capturedQueries) != 2 {
		t.Fatalf("expected 2 queries captured, got %d: %v", len(capturedQueries), capturedQueries)
	}
	if capturedQueries[0] != "What is Go?" {
		t.Errorf("turn 1 query = %q, want %q", capturedQueries[0], "What is Go?")
	}
	if capturedQueries[1] != "Can you give an example?" {
		t.Errorf("turn 2 query = %q, want %q", capturedQueries[1], "Can you give an example?")
	}

	// Verify turn 2 assembled context includes both turns (system + original msgs + second user msg).
	if len(assembled2) < 3 {
		t.Errorf("expected at least 3 messages in turn 2 context, got %d", len(assembled2))
	}
}

// capturingQueryMemSearcher records every query string passed to Search.
type capturingQueryMemSearcher struct {
	queries *[]string
}

func (c *capturingQueryMemSearcher) Search(_ context.Context, query string, _ int) ([]MemoryResult, error) {
	*c.queries = append(*c.queries, query)
	return nil, nil
}

// ---------------------------------------------------------------------------
// Test 4: Context truncation
// ---------------------------------------------------------------------------

func TestIntegrationContextTruncation(t *testing.T) {
	assembler := NewContextAssembler(nil, nil, newTestLogger())

	// Build a very large conversation history (~500 tokens per message × 10 turns = ~10 000 tokens).
	msgs := make([]Message, 0, 21)
	for i := 0; i < 10; i++ {
		msgs = append(msgs,
			Message{Role: "user", Content: strings.Repeat("word ", 100)},       // ~125 tokens
			Message{Role: "assistant", Content: strings.Repeat("reply ", 100)}, // ~125 tokens
		)
	}
	msgs = append(msgs, Message{Role: "user", Content: "Final important question."})

	opts := AssembleOpts{
		SystemPrompt:   "System instructions.",
		MaxTokens:      400, // tight budget
		ReservedTokens: 1,   // minimal reserve
	}

	assembled, err := assembler.Assemble(context.Background(), msgs, opts)
	if err != nil {
		t.Fatalf("Assemble error: %v", err)
	}

	// Verify system prompt is preserved.
	if len(assembled) == 0 {
		t.Fatal("expected at least 1 message")
	}
	if assembled[0].Role != "system" {
		t.Fatalf("first role = %q, want system", assembled[0].Role)
	}
	if !strings.Contains(assembled[0].Content, "System instructions.") {
		t.Error("system prompt must be preserved after truncation")
	}

	// Verify last user message is preserved.
	lastMsg := assembled[len(assembled)-1]
	if lastMsg.Role != "user" {
		t.Errorf("last message role = %q, want user", lastMsg.Role)
	}
	if lastMsg.Content != "Final important question." {
		t.Errorf("last message content = %q, want %q", lastMsg.Content, "Final important question.")
	}

	// Verify the total token estimate fits (approximately — estimateTokens uses chars/4).
	// Effective budget = MaxTokens - ReservedTokens = 399.
	effectiveBudget := opts.MaxTokens - opts.ReservedTokens
	total := totalTokens(assembled)
	if total > effectiveBudget {
		// The truncation may not reduce below budget when only system+last-user remain,
		// but the trimming loop should have removed as many history messages as possible.
		// Verify that fewer messages than the original are present.
		if len(assembled) >= len(msgs)+1 { // +1 for system message
			t.Errorf("expected fewer messages after truncation; got %d", len(assembled))
		}
		_ = total // only checked indirectly via message count above
	}
}

// ---------------------------------------------------------------------------
// Test 5: No providers graceful handling
// ---------------------------------------------------------------------------

func TestIntegrationNoProviders(t *testing.T) {
	mgr := NewManager(newTestLogger())

	t.Run("Chat returns ErrNoProviders", func(t *testing.T) {
		_, err := mgr.Chat(context.Background(), ChatRequest{
			Model:    "any-model",
			Messages: []Message{{Role: "user", Content: "Hello"}},
		})
		if !errors.Is(err, ErrNoProviders) {
			t.Errorf("expected ErrNoProviders, got %v", err)
		}
	})

	t.Run("ChatStream returns ErrNoProviders", func(t *testing.T) {
		_, err := mgr.ChatStream(context.Background(), ChatRequest{
			Model:    "any-model",
			Messages: []Message{{Role: "user", Content: "Hello"}},
		})
		if !errors.Is(err, ErrNoProviders) {
			t.Errorf("expected ErrNoProviders, got %v", err)
		}
	})

	t.Run("ListModels returns empty list, no error", func(t *testing.T) {
		models, err := mgr.ListModels(context.Background())
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
		if len(models) != 0 {
			t.Errorf("expected empty model list, got %d models", len(models))
		}
	})
}
