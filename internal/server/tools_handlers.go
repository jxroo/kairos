package server

import (
	"encoding/json"
	"net/http"
	"strconv"

	"go.uber.org/zap"
)

func (s *Server) handleListTools(w http.ResponseWriter, r *http.Request) {
	if s.executor == nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, `{"error":"tool service unavailable"}`, http.StatusServiceUnavailable)
		return
	}

	defs := s.executor.Registry().List()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(defs)
}

func (s *Server) handleExecuteTool(w http.ResponseWriter, r *http.Request) {
	if s.executor == nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, `{"error":"tool service unavailable"}`, http.StatusServiceUnavailable)
		return
	}

	var req struct {
		Name      string         `json:"name"`
		Arguments map[string]any `json:"arguments"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}

	if req.Name == "" {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, `{"error":"name is required"}`, http.StatusBadRequest)
		return
	}

	result, err := s.executor.Execute(r.Context(), req.Name, req.Arguments, "http")
	if err != nil {
		s.logger.Error("tool execution failed", zap.String("tool", req.Name), zap.Error(err))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

func (s *Server) handleToolAudit(w http.ResponseWriter, r *http.Request) {
	if s.executor == nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, `{"error":"tool service unavailable"}`, http.StatusServiceUnavailable)
		return
	}

	limit := 50
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
			limit = parsed
		}
	}

	entries, err := s.executor.Audit().List(r.Context(), limit)
	if err != nil {
		s.logger.Error("audit list failed", zap.Error(err))
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, `{"error":"internal server error"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(entries)
}
