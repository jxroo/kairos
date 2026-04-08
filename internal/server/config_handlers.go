package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"net/http"

	"go.uber.org/zap"

	"github.com/jxroo/kairos/internal/config"
)

type configResponse struct {
	Path     string `json:"path"`
	Content  string `json:"content,omitempty"`
	Writable bool   `json:"writable"`
}

type updateConfigRequest struct {
	Content string `json:"content"`
}

type updateConfigResponse struct {
	Path           string `json:"path"`
	ReloadRequired bool   `json:"reload_required"`
}

func (s *Server) handleGetConfig(w http.ResponseWriter, r *http.Request) {
	if s.runtimeInfo.ConfigPath == "" {
		writeJSONError(w, http.StatusServiceUnavailable, "config unavailable")
		return
	}

	content, err := os.ReadFile(s.runtimeInfo.ConfigPath)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			s.logger.Error("reading config failed", zap.Error(err))
			writeJSONError(w, http.StatusInternalServerError, "failed to read config")
			return
		}
		content = []byte(s.runtimeInfo.StarterConfig)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(configResponse{
		Path:     s.runtimeInfo.ConfigPath,
		Content:  string(content),
		Writable: true,
	})
}

func (s *Server) handleUpdateConfig(w http.ResponseWriter, r *http.Request) {
	if s.runtimeInfo.ConfigPath == "" {
		writeJSONError(w, http.StatusServiceUnavailable, "config unavailable")
		return
	}

	var req updateConfigRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if strings.TrimSpace(req.Content) == "" {
		writeJSONError(w, http.StatusBadRequest, "content is required")
		return
	}

	configDir := filepath.Dir(s.runtimeInfo.ConfigPath)
	if _, err := config.Parse([]byte(req.Content), configDir); err != nil {
		writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("invalid config: %v", err))
		return
	}

	if err := writeFileAtomically(s.runtimeInfo.ConfigPath, []byte(req.Content), 0644); err != nil {
		s.logger.Error("writing config failed", zap.Error(err))
		writeJSONError(w, http.StatusInternalServerError, "failed to write config")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(updateConfigResponse{
		Path:           s.runtimeInfo.ConfigPath,
		ReloadRequired: true,
	})
}

func writeFileAtomically(path string, content []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("creating config directory: %w", err)
	}

	tmp, err := os.CreateTemp(dir, "kairos-config-*.tmp")
	if err != nil {
		return fmt.Errorf("creating temp file: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	if _, err := tmp.Write(content); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("writing temp file: %w", err)
	}
	if err := tmp.Chmod(mode); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("setting temp file mode: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("closing temp file: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("replacing config: %w", err)
	}
	return nil
}

func writeJSONError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": message})
}
