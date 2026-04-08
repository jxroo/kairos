package server

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"go.uber.org/zap"

	"github.com/jxroo/kairos/internal/rag"
)

func (s *Server) handleListDocuments(w http.ResponseWriter, r *http.Request) {
	if s.documentLister == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "documents unavailable")
		return
	}

	docs, err := s.documentLister.ListDocuments(r.Context())
	if err != nil {
		s.logger.Error("listing documents failed", zap.Error(err))
		writeJSONError(w, http.StatusInternalServerError, "internal server error")
		return
	}

	statusFilter := strings.TrimSpace(r.URL.Query().Get("status"))
	filtered := make([]rag.Document, 0, len(docs))
	for _, doc := range docs {
		if statusFilter != "" && string(doc.Status) != statusFilter {
			continue
		}
		filtered = append(filtered, doc)
	}

	offset := 0
	if o := r.URL.Query().Get("offset"); o != "" {
		if parsed, err := strconv.Atoi(o); err == nil && parsed >= 0 {
			offset = parsed
		}
	}

	limit := len(filtered)
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
			limit = parsed
		}
	}

	if offset > len(filtered) {
		offset = len(filtered)
	}
	end := offset + limit
	if end > len(filtered) {
		end = len(filtered)
	}
	if offset < end {
		filtered = filtered[offset:end]
	} else {
		filtered = []rag.Document{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(filtered)
}

func (s *Server) handleIndexStatus(w http.ResponseWriter, r *http.Request) {
	if s.progress == nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, `{"error":"index service unavailable"}`, http.StatusServiceUnavailable)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(s.progress.Status())
}

func (s *Server) handleIndexRebuild(w http.ResponseWriter, r *http.Request) {
	if s.rebuildFunc == nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, `{"error":"rebuild unavailable"}`, http.StatusServiceUnavailable)
		return
	}

	go func() {
		if err := s.rebuildFunc(); err != nil {
			s.logger.Error("rebuild failed", zap.Error(err))
		}
	}()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]string{"status": "rebuild started"})
}

func (s *Server) handleSearchDocuments(w http.ResponseWriter, r *http.Request) {
	if s.ragSearchSvc == nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, `{"error":"document search unavailable"}`, http.StatusServiceUnavailable)
		return
	}

	q := r.URL.Query()
	query := q.Get("query")

	limit := 10
	if l := q.Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
			limit = parsed
		}
	}

	var fileTypes []string
	if ft := q.Get("file_types"); ft != "" {
		for _, t := range strings.Split(ft, ",") {
			if trimmed := strings.TrimSpace(t); trimmed != "" {
				fileTypes = append(fileTypes, trimmed)
			}
		}
	}

	results, err := s.ragSearchSvc.Search(r.Context(), rag.RAGSearchQuery{
		Query:     query,
		Limit:     limit,
		FileTypes: fileTypes,
	})
	if err != nil {
		s.logger.Error("document search failed", zap.Error(err))
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, `{"error":"internal server error"}`, http.StatusInternalServerError)
		return
	}

	if results == nil {
		results = []rag.ChunkSearchResult{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(results)
}
