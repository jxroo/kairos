package server

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"

	"github.com/jxroo/kairos/internal/memory"
)

// requireStore returns true if s.store is non-nil, otherwise writes a 503
// response and returns false.
func (s *Server) requireStore(w http.ResponseWriter) bool {
	if s.store == nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, `{"error":"memory service unavailable"}`, http.StatusServiceUnavailable)
		return false
	}
	return true
}

func (s *Server) handleCreateMemory(w http.ResponseWriter, r *http.Request) {
	if !s.requireStore(w) {
		return
	}

	var in memory.CreateMemoryInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}

	if strings.TrimSpace(in.Content) == "" {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, `{"error":"content is required"}`, http.StatusBadRequest)
		return
	}

	mem, err := s.store.Create(r.Context(), in)
	if err != nil {
		s.logger.Error("failed to create memory", zap.Error(err))
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, `{"error":"internal server error"}`, http.StatusInternalServerError)
		return
	}

	if s.searchSvc != nil {
		go func() {
			if err := s.searchSvc.IndexMemory(r.Context(), mem); err != nil {
				s.logger.Error("failed to index memory", zap.String("id", mem.ID), zap.Error(err))
			}
		}()
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(mem)
}

func (s *Server) handleSearchMemories(w http.ResponseWriter, r *http.Request) {
	if s.searchSvc == nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, `{"error":"search service unavailable"}`, http.StatusServiceUnavailable)
		return
	}

	q := r.URL.Query()

	query := q.Get("query")

	limit := 5
	if l := q.Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
			limit = parsed
		}
	}

	var tags []string
	if t := q.Get("tags"); t != "" {
		for _, tag := range strings.Split(t, ",") {
			if trimmed := strings.TrimSpace(tag); trimmed != "" {
				tags = append(tags, trimmed)
			}
		}
	}

	minRelevance := 0.0
	if mr := q.Get("min_relevance"); mr != "" {
		if parsed, err := strconv.ParseFloat(mr, 64); err == nil {
			minRelevance = parsed
		}
	}

	results, err := s.searchSvc.Search(r.Context(), memory.SearchQuery{
		Query:        query,
		Limit:        limit,
		Tags:         tags,
		MinRelevance: minRelevance,
	})
	if err != nil {
		s.logger.Error("search failed", zap.Error(err))
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, `{"error":"internal server error"}`, http.StatusInternalServerError)
		return
	}

	if results == nil {
		results = []memory.SearchResult{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(results)
}

func (s *Server) handleGetMemory(w http.ResponseWriter, r *http.Request) {
	if !s.requireStore(w) {
		return
	}

	id := chi.URLParam(r, "id")

	mem, err := s.store.Get(r.Context(), id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			w.Header().Set("Content-Type", "application/json")
			http.Error(w, `{"error":"memory not found"}`, http.StatusNotFound)
			return
		}
		s.logger.Error("failed to get memory", zap.String("id", id), zap.Error(err))
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, `{"error":"internal server error"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(mem)
}

func (s *Server) handleUpdateMemory(w http.ResponseWriter, r *http.Request) {
	if !s.requireStore(w) {
		return
	}

	id := chi.URLParam(r, "id")

	var in memory.UpdateMemoryInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}

	mem, err := s.store.Update(r.Context(), id, in)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			w.Header().Set("Content-Type", "application/json")
			http.Error(w, `{"error":"memory not found"}`, http.StatusNotFound)
			return
		}
		s.logger.Error("failed to update memory", zap.String("id", id), zap.Error(err))
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, `{"error":"internal server error"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(mem)
}

func (s *Server) handleDeleteMemory(w http.ResponseWriter, r *http.Request) {
	if !s.requireStore(w) {
		return
	}

	id := chi.URLParam(r, "id")

	if err := s.store.Delete(r.Context(), id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			w.Header().Set("Content-Type", "application/json")
			http.Error(w, `{"error":"memory not found"}`, http.StatusNotFound)
			return
		}
		s.logger.Error("failed to delete memory", zap.String("id", id), zap.Error(err))
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, `{"error":"internal server error"}`, http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
