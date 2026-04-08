package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jxroo/kairos/internal/memory"
)

func TestHandleListConversations(t *testing.T) {
	srv, store := setupTestServer(t)

	// Create a couple of conversations.
	_, err := store.CreateConversation(t.Context(), "conv one", "llama3:latest")
	if err != nil {
		t.Fatalf("failed to create conversation: %v", err)
	}
	_, err = store.CreateConversation(t.Context(), "conv two", "mistral:latest")
	if err != nil {
		t.Fatalf("failed to create conversation: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/conversations/", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	var convs []memory.Conversation
	if err := json.NewDecoder(w.Body).Decode(&convs); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if len(convs) < 2 {
		t.Errorf("expected at least 2 conversations, got %d", len(convs))
	}
}

func TestHandleListConversationsEmpty(t *testing.T) {
	srv, _ := setupTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/conversations/", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	var convs []memory.Conversation
	if err := json.NewDecoder(w.Body).Decode(&convs); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	// Empty list (may be nil slice after decode but that's fine).
}

func TestHandleGetConversation(t *testing.T) {
	srv, store := setupTestServer(t)

	conv, err := store.CreateConversation(t.Context(), "my conversation", "llama3:latest")
	if err != nil {
		t.Fatalf("failed to create conversation: %v", err)
	}

	// Add some messages.
	if err := store.AddMessage(t.Context(), conv.ID, memory.ConversationMessage{
		Role:    "user",
		Content: "Hello",
	}); err != nil {
		t.Fatalf("failed to add message: %v", err)
	}
	if err := store.AddMessage(t.Context(), conv.ID, memory.ConversationMessage{
		Role:    "assistant",
		Content: "Hi there!",
	}); err != nil {
		t.Fatalf("failed to add message: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/conversations/"+conv.ID, nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp conversationResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.ID != conv.ID {
		t.Errorf("expected ID=%q, got %q", conv.ID, resp.ID)
	}
	if resp.Title != conv.Title {
		t.Errorf("expected title=%q, got %q", conv.Title, resp.Title)
	}
	if len(resp.Messages) != 2 {
		t.Errorf("expected 2 messages, got %d", len(resp.Messages))
	}
	if resp.Messages[0].Role != "user" {
		t.Errorf("expected first message role=user, got %q", resp.Messages[0].Role)
	}
	if resp.Messages[1].Role != "assistant" {
		t.Errorf("expected second message role=assistant, got %q", resp.Messages[1].Role)
	}
}

func TestHandleGetConversationNotFound(t *testing.T) {
	srv, _ := setupTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/conversations/nonexistent-id", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected status 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleDeleteConversation(t *testing.T) {
	srv, store := setupTestServer(t)

	conv, err := store.CreateConversation(t.Context(), "to delete", "llama3:latest")
	if err != nil {
		t.Fatalf("failed to create conversation: %v", err)
	}

	req := httptest.NewRequest(http.MethodDelete, "/conversations/"+conv.ID, nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("expected status 204, got %d: %s", w.Code, w.Body.String())
	}

	// Verify it's gone.
	req2 := httptest.NewRequest(http.MethodGet, "/conversations/"+conv.ID, nil)
	w2 := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w2, req2)

	if w2.Code != http.StatusNotFound {
		t.Errorf("expected 404 after delete, got %d", w2.Code)
	}
}

func TestHandleDeleteConversationNotFound(t *testing.T) {
	srv, _ := setupTestServer(t)

	req := httptest.NewRequest(http.MethodDelete, "/conversations/nonexistent-id", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected status 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleListConversationsWithPagination(t *testing.T) {
	srv, store := setupTestServer(t)

	for i := 0; i < 5; i++ {
		_, err := store.CreateConversation(t.Context(), "conv", "llama3:latest")
		if err != nil {
			t.Fatalf("failed to create conversation: %v", err)
		}
	}

	req := httptest.NewRequest(http.MethodGet, "/conversations/?limit=2&offset=0", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	var convs []memory.Conversation
	if err := json.NewDecoder(w.Body).Decode(&convs); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if len(convs) != 2 {
		t.Errorf("expected 2 conversations with limit=2, got %d", len(convs))
	}
}

func TestHandleSearchConversations(t *testing.T) {
	srv, store := setupTestServer(t)

	conv, err := store.CreateConversation(t.Context(), "Phase 3 planning", "llama3:latest")
	if err != nil {
		t.Fatalf("failed to create conversation: %v", err)
	}
	if err := store.AddMessage(t.Context(), conv.ID, memory.ConversationMessage{
		Role:    "user",
		Content: "Need to fix inference routing",
	}); err != nil {
		t.Fatalf("failed to add message: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/conversations/search?q=inference", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	var convs []memory.Conversation
	if err := json.NewDecoder(w.Body).Decode(&convs); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if len(convs) != 1 || convs[0].ID != conv.ID {
		t.Fatalf("unexpected search results: %+v", convs)
	}
}

func TestHandleSearchConversationsMissingQuery(t *testing.T) {
	srv, _ := setupTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/conversations/search", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d: %s", w.Code, w.Body.String())
	}
}
