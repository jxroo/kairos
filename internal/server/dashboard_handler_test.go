package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"go.uber.org/zap"

	"github.com/jxroo/kairos/internal/config"
)

func dashboardFS() fstest.MapFS {
	return fstest.MapFS{
		"index.html": &fstest.MapFile{
			Data: []byte("<html>Coming Soon</html>"),
		},
	}
}

func TestDashboardEnabled(t *testing.T) {
	logger := zap.NewNop()
	cfg := &config.DashboardConfig{Enabled: true}
	srv := New(logger, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, cfg, dashboardFS(), RuntimeInfo{})

	req := httptest.NewRequest(http.MethodGet, "/dashboard/", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "Coming Soon") {
		t.Errorf("expected 'Coming Soon' in body, got %q", w.Body.String())
	}
}

func TestDashboardSPAFallback(t *testing.T) {
	logger := zap.NewNop()
	cfg := &config.DashboardConfig{Enabled: true}
	srv := New(logger, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, cfg, dashboardFS(), RuntimeInfo{})

	req := httptest.NewRequest(http.MethodGet, "/dashboard/nonexistent", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 (SPA fallback), got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "Coming Soon") {
		t.Errorf("expected SPA fallback to index.html, got %q", w.Body.String())
	}
}

func TestDashboardDisabled(t *testing.T) {
	logger := zap.NewNop()
	srv := New(logger, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, RuntimeInfo{})

	req := httptest.NewRequest(http.MethodGet, "/dashboard/", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404 when dashboard disabled, got %d", w.Code)
	}
}

func TestDashboardRedirect(t *testing.T) {
	logger := zap.NewNop()
	cfg := &config.DashboardConfig{Enabled: true}
	srv := New(logger, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, cfg, dashboardFS(), RuntimeInfo{})

	req := httptest.NewRequest(http.MethodGet, "/dashboard", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusMovedPermanently {
		t.Errorf("expected 301 redirect, got %d", w.Code)
	}
	loc := w.Header().Get("Location")
	if loc != "/dashboard/" {
		t.Errorf("expected redirect to /dashboard/, got %q", loc)
	}
}
