package server

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"

	"github.com/jxroo/kairos/internal/memory"
)

// conversationResponse is the shape returned by GET /conversations/{id},
// embedding the messages list alongside the conversation metadata.
type conversationResponse struct {
	ID        string                       `json:"id"`
	Title     string                       `json:"title"`
	Model     string                       `json:"model"`
	CreatedAt time.Time                    `json:"created_at"`
	UpdatedAt time.Time                    `json:"updated_at"`
	Messages  []memory.ConversationMessage `json:"messages"`
}

// handleListConversations implements GET /conversations.
func (s *Server) handleListConversations(w http.ResponseWriter, r *http.Request) {
	if s.store == nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, `{"error":"store unavailable"}`, http.StatusServiceUnavailable)
		return
	}

	q := r.URL.Query()

	limit := 50
	if l := q.Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
			limit = parsed
		}
	}

	offset := 0
	if o := q.Get("offset"); o != "" {
		if parsed, err := strconv.Atoi(o); err == nil && parsed >= 0 {
			offset = parsed
		}
	}

	convs, err := s.store.ListConversations(r.Context(), limit, offset)
	if err != nil {
		s.logger.Error("listing conversations failed", zap.Error(err))
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, `{"error":"internal server error"}`, http.StatusInternalServerError)
		return
	}

	// Ensure we always return a JSON array, never null.
	if convs == nil {
		convs = []memory.Conversation{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(convs)
}

// handleGetConversation implements GET /conversations/{id}.
func (s *Server) handleGetConversation(w http.ResponseWriter, r *http.Request) {
	if s.store == nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, `{"error":"store unavailable"}`, http.StatusServiceUnavailable)
		return
	}

	id := chi.URLParam(r, "id")
	ctx := r.Context()

	conv, err := s.store.GetConversation(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			w.Header().Set("Content-Type", "application/json")
			http.Error(w, `{"error":"conversation not found"}`, http.StatusNotFound)
			return
		}
		s.logger.Error("getting conversation failed", zap.Error(err))
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, `{"error":"internal server error"}`, http.StatusInternalServerError)
		return
	}

	msgs, err := s.store.GetMessages(ctx, id)
	if err != nil {
		s.logger.Error("getting messages failed", zap.Error(err))
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, `{"error":"internal server error"}`, http.StatusInternalServerError)
		return
	}

	if msgs == nil {
		msgs = []memory.ConversationMessage{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(conversationResponse{
		ID:        conv.ID,
		Title:     conv.Title,
		Model:     conv.Model,
		CreatedAt: conv.CreatedAt,
		UpdatedAt: conv.UpdatedAt,
		Messages:  msgs,
	})
}

// handleSearchConversations implements GET /conversations/search.
func (s *Server) handleSearchConversations(w http.ResponseWriter, r *http.Request) {
	if s.store == nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, `{"error":"store unavailable"}`, http.StatusServiceUnavailable)
		return
	}

	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if query == "" {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, `{"error":"missing query parameter q"}`, http.StatusBadRequest)
		return
	}

	limit := 50
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
			limit = parsed
		}
	}

	offset := 0
	if o := r.URL.Query().Get("offset"); o != "" {
		if parsed, err := strconv.Atoi(o); err == nil && parsed >= 0 {
			offset = parsed
		}
	}

	convs, err := s.store.SearchConversations(r.Context(), query, limit, offset)
	if err != nil {
		s.logger.Error("searching conversations failed", zap.Error(err))
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, `{"error":"internal server error"}`, http.StatusInternalServerError)
		return
	}

	if convs == nil {
		convs = []memory.Conversation{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(convs)
}

// handleDeleteConversation implements DELETE /conversations/{id}.
func (s *Server) handleDeleteConversation(w http.ResponseWriter, r *http.Request) {
	if s.store == nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, `{"error":"store unavailable"}`, http.StatusServiceUnavailable)
		return
	}

	id := chi.URLParam(r, "id")

	if err := s.store.DeleteConversation(r.Context(), id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			w.Header().Set("Content-Type", "application/json")
			http.Error(w, `{"error":"conversation not found"}`, http.StatusNotFound)
			return
		}
		s.logger.Error("deleting conversation failed", zap.Error(err))
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, `{"error":"internal server error"}`, http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
