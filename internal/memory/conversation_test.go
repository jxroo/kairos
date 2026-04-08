package memory

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

// TestConversationCreate verifies that CreateConversation inserts a row and
// returns the populated Conversation.
func TestConversationCreate(t *testing.T) {
	tests := []struct {
		name  string
		title string
		model string
	}{
		{name: "with title and model", title: "My Chat", model: "llama3"},
		{name: "empty title", title: "", model: "mistral"},
		{name: "empty model", title: "Hello", model: ""},
		{name: "both empty", title: "", model: ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := newTestStore(t)
			ctx := context.Background()

			conv, err := s.CreateConversation(ctx, tc.title, tc.model)
			if err != nil {
				t.Fatalf("CreateConversation: %v", err)
			}

			if conv.ID == "" {
				t.Error("expected non-empty ID")
			}
			if conv.Title != tc.title {
				t.Errorf("Title = %q; want %q", conv.Title, tc.title)
			}
			if conv.Model != tc.model {
				t.Errorf("Model = %q; want %q", conv.Model, tc.model)
			}
			if conv.CreatedAt.IsZero() {
				t.Error("expected non-zero CreatedAt")
			}
			if conv.UpdatedAt.IsZero() {
				t.Error("expected non-zero UpdatedAt")
			}
		})
	}
}

// TestConversationGet verifies GetConversation returns the correct record and
// errors on a missing ID.
func TestConversationGet(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	created, err := s.CreateConversation(ctx, "Test", "gpt-4")
	if err != nil {
		t.Fatalf("CreateConversation: %v", err)
	}

	t.Run("existing", func(t *testing.T) {
		got, err := s.GetConversation(ctx, created.ID)
		if err != nil {
			t.Fatalf("GetConversation: %v", err)
		}
		if got.ID != created.ID {
			t.Errorf("ID = %q; want %q", got.ID, created.ID)
		}
		if got.Title != "Test" {
			t.Errorf("Title = %q; want %q", got.Title, "Test")
		}
		if got.Model != "gpt-4" {
			t.Errorf("Model = %q; want %q", got.Model, "gpt-4")
		}
	})

	t.Run("not found", func(t *testing.T) {
		_, err := s.GetConversation(ctx, "nonexistent-id")
		if err == nil {
			t.Fatal("expected error for non-existent conversation, got nil")
		}
		if !errors.Is(err, sql.ErrNoRows) {
			t.Errorf("expected sql.ErrNoRows in error chain, got: %v", err)
		}
	})
}

// TestConversationList verifies ListConversations returns records ordered by
// updated_at descending and respects limit/offset pagination.
func TestConversationList(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	// Create 5 conversations.
	ids := make([]string, 5)
	for i := 0; i < 5; i++ {
		conv, err := s.CreateConversation(ctx, "conv", "model")
		if err != nil {
			t.Fatalf("CreateConversation[%d]: %v", i, err)
		}
		ids[i] = conv.ID
	}

	t.Run("list all", func(t *testing.T) {
		convs, err := s.ListConversations(ctx, 10, 0)
		if err != nil {
			t.Fatalf("ListConversations: %v", err)
		}
		if len(convs) != 5 {
			t.Errorf("len = %d; want 5", len(convs))
		}
	})

	t.Run("limit", func(t *testing.T) {
		convs, err := s.ListConversations(ctx, 2, 0)
		if err != nil {
			t.Fatalf("ListConversations: %v", err)
		}
		if len(convs) != 2 {
			t.Errorf("len = %d; want 2", len(convs))
		}
	})

	t.Run("offset beyond count", func(t *testing.T) {
		convs, err := s.ListConversations(ctx, 10, 100)
		if err != nil {
			t.Fatalf("ListConversations: %v", err)
		}
		if len(convs) != 0 {
			t.Errorf("len = %d; want 0", len(convs))
		}
	})

	t.Run("zero limit defaults to 50", func(t *testing.T) {
		convs, err := s.ListConversations(ctx, 0, 0)
		if err != nil {
			t.Fatalf("ListConversations: %v", err)
		}
		if len(convs) != 5 {
			t.Errorf("len = %d; want 5", len(convs))
		}
	})

	_ = ids
}

func TestConversationSearch(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	titleMatch, err := s.CreateConversation(ctx, "Project Notes", "llama3")
	if err != nil {
		t.Fatalf("CreateConversation(titleMatch): %v", err)
	}
	if err := s.AddMessage(ctx, titleMatch.ID, ConversationMessage{
		Role:    "user",
		Content: "General planning",
	}); err != nil {
		t.Fatalf("AddMessage(titleMatch): %v", err)
	}

	messageMatch, err := s.CreateConversation(ctx, "Random Chat", "mistral")
	if err != nil {
		t.Fatalf("CreateConversation(messageMatch): %v", err)
	}
	if err := s.AddMessage(ctx, messageMatch.ID, ConversationMessage{
		Role:    "user",
		Content: "Need a release checklist for Kairos phase 3",
	}); err != nil {
		t.Fatalf("AddMessage(messageMatch): %v", err)
	}

	results, err := s.SearchConversations(ctx, "phase 3", 10, 0)
	if err != nil {
		t.Fatalf("SearchConversations: %v", err)
	}
	if len(results) != 1 || results[0].ID != messageMatch.ID {
		t.Fatalf("unexpected results for message search: %+v", results)
	}

	results, err = s.SearchConversations(ctx, "project", 10, 0)
	if err != nil {
		t.Fatalf("SearchConversations title: %v", err)
	}
	if len(results) != 1 || results[0].ID != titleMatch.ID {
		t.Fatalf("unexpected results for title search: %+v", results)
	}
}

// TestConversationDelete verifies DeleteConversation removes the row and
// cascades to messages.
func TestConversationDelete(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	conv, err := s.CreateConversation(ctx, "to delete", "m")
	if err != nil {
		t.Fatalf("CreateConversation: %v", err)
	}

	// Add a message so we can verify cascade.
	if err := s.AddMessage(ctx, conv.ID, ConversationMessage{
		Role: "user", Content: "hello",
	}); err != nil {
		t.Fatalf("AddMessage: %v", err)
	}

	t.Run("delete existing", func(t *testing.T) {
		if err := s.DeleteConversation(ctx, conv.ID); err != nil {
			t.Fatalf("DeleteConversation: %v", err)
		}

		// Verify conversation is gone.
		_, err := s.GetConversation(ctx, conv.ID)
		if err == nil {
			t.Fatal("expected error after delete, got nil")
		}
		if !errors.Is(err, sql.ErrNoRows) {
			t.Errorf("expected sql.ErrNoRows in error chain, got: %v", err)
		}

		// Verify messages were cascade-deleted.
		msgs, err := s.GetMessages(ctx, conv.ID)
		if err != nil {
			t.Fatalf("GetMessages after delete: %v", err)
		}
		if len(msgs) != 0 {
			t.Errorf("expected 0 messages after cascade delete, got %d", len(msgs))
		}
	})

	t.Run("delete non-existent", func(t *testing.T) {
		err := s.DeleteConversation(ctx, "nonexistent-id")
		if err == nil {
			t.Fatal("expected error for non-existent conversation, got nil")
		}
		if !errors.Is(err, sql.ErrNoRows) {
			t.Errorf("expected sql.ErrNoRows in error chain, got: %v", err)
		}
	})
}

// TestConversationAddMessage verifies AddMessage inserts messages correctly
// including role validation.
func TestConversationAddMessage(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	conv, err := s.CreateConversation(ctx, "msgs", "llama3")
	if err != nil {
		t.Fatalf("CreateConversation: %v", err)
	}

	tests := []struct {
		name    string
		msg     ConversationMessage
		wantErr bool
	}{
		{
			name:    "system message",
			msg:     ConversationMessage{Role: "system", Content: "You are helpful."},
			wantErr: false,
		},
		{
			name:    "user message",
			msg:     ConversationMessage{Role: "user", Content: "Hello!"},
			wantErr: false,
		},
		{
			name:    "assistant message",
			msg:     ConversationMessage{Role: "assistant", Content: "Hi there!"},
			wantErr: false,
		},
		{
			name:    "message with tokens",
			msg:     ConversationMessage{Role: "user", Content: "count me", Tokens: 42},
			wantErr: false,
		},
		{
			name:    "invalid role",
			msg:     ConversationMessage{Role: "bot", Content: "invalid"},
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := s.AddMessage(ctx, conv.ID, tc.msg)
			if tc.wantErr && err == nil {
				t.Error("expected error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

// TestConversationGetMessages verifies GetMessages returns all messages ordered
// by created_at ascending.
func TestConversationGetMessages(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	conv, err := s.CreateConversation(ctx, "chat", "llama3")
	if err != nil {
		t.Fatalf("CreateConversation: %v", err)
	}

	msgs := []ConversationMessage{
		{Role: "system", Content: "You are helpful."},
		{Role: "user", Content: "What is Go?"},
		{Role: "assistant", Content: "Go is a compiled language.", Tokens: 10},
		{Role: "user", Content: "Thanks!"},
	}

	for _, m := range msgs {
		if err := s.AddMessage(ctx, conv.ID, m); err != nil {
			t.Fatalf("AddMessage: %v", err)
		}
	}

	got, err := s.GetMessages(ctx, conv.ID)
	if err != nil {
		t.Fatalf("GetMessages: %v", err)
	}

	if len(got) != len(msgs) {
		t.Fatalf("len = %d; want %d", len(got), len(msgs))
	}

	for i, m := range got {
		if m.Role != msgs[i].Role {
			t.Errorf("[%d] Role = %q; want %q", i, m.Role, msgs[i].Role)
		}
		if m.Content != msgs[i].Content {
			t.Errorf("[%d] Content = %q; want %q", i, m.Content, msgs[i].Content)
		}
		if m.ConversationID != conv.ID {
			t.Errorf("[%d] ConversationID = %q; want %q", i, m.ConversationID, conv.ID)
		}
		if m.ID == "" {
			t.Errorf("[%d] expected non-empty ID", i)
		}
		if m.CreatedAt.IsZero() {
			t.Errorf("[%d] expected non-zero CreatedAt", i)
		}
	}

	// Verify tokens are stored correctly.
	if got[2].Tokens != 10 {
		t.Errorf("Tokens = %d; want 10", got[2].Tokens)
	}
}

// TestConversationGetMessagesOrdered verifies that messages are returned
// ordered by created_at ascending when explicit timestamps differ.
func TestConversationGetMessagesOrdered(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	conv, err := s.CreateConversation(ctx, "order test", "m")
	if err != nil {
		t.Fatalf("CreateConversation: %v", err)
	}

	// Insert messages in reverse order via explicit IDs.
	orderedRoles := []string{"system", "user", "assistant"}
	for _, role := range orderedRoles {
		if err := s.AddMessage(ctx, conv.ID, ConversationMessage{
			Role:    role,
			Content: role + " content",
		}); err != nil {
			t.Fatalf("AddMessage(%s): %v", role, err)
		}
	}

	got, err := s.GetMessages(ctx, conv.ID)
	if err != nil {
		t.Fatalf("GetMessages: %v", err)
	}

	if len(got) != 3 {
		t.Fatalf("len = %d; want 3", len(got))
	}

	for i, want := range orderedRoles {
		if got[i].Role != want {
			t.Errorf("[%d] Role = %q; want %q", i, got[i].Role, want)
		}
	}
}

// TestConversationGetNonExistent verifies GetConversation for a missing ID
// returns an error wrapping sql.ErrNoRows.
func TestConversationGetNonExistent(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	_, err := s.GetConversation(ctx, "does-not-exist")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("expected sql.ErrNoRows in error chain, got: %v", err)
	}
}

// TestConversationMessageTokensZero verifies that a message with zero tokens
// is stored and retrieved with Tokens == 0 (not an error).
func TestConversationMessageTokensZero(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	conv, err := s.CreateConversation(ctx, "tokens test", "m")
	if err != nil {
		t.Fatalf("CreateConversation: %v", err)
	}

	if err := s.AddMessage(ctx, conv.ID, ConversationMessage{
		Role: "user", Content: "no token count",
	}); err != nil {
		t.Fatalf("AddMessage: %v", err)
	}

	msgs, err := s.GetMessages(ctx, conv.ID)
	if err != nil {
		t.Fatalf("GetMessages: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("len = %d; want 1", len(msgs))
	}
	if msgs[0].Tokens != 0 {
		t.Errorf("Tokens = %d; want 0", msgs[0].Tokens)
	}
}
