package server

import (
	"context"
	"encoding/json"
	"io/fs"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"go.uber.org/zap"

	"github.com/jxroo/kairos/internal/config"
	"github.com/jxroo/kairos/internal/inference"
	"github.com/jxroo/kairos/internal/memory"
	"github.com/jxroo/kairos/internal/rag"
	"github.com/jxroo/kairos/internal/tools"
)

type Server struct {
	router           chi.Router
	logger           *zap.Logger
	store            *memory.Store
	searchSvc        *memory.SearchService
	ragSearchSvc     *rag.RAGSearchService
	documentLister   DocumentLister
	progress         *rag.Progress
	rebuildFunc      func() error
	inferenceManager *inference.Manager
	contextAssembler *inference.ContextAssembler
	inferenceConfig  *config.InferenceConfig
	executor         *tools.Executor
	mcpHandler       http.Handler
	dashboardCfg     *config.DashboardConfig
	dashboardFS      fs.FS
	runtimeInfo      RuntimeInfo
}

type DocumentLister interface {
	ListDocuments(ctx context.Context) ([]rag.Document, error)
}

type RuntimeInfo struct {
	Version       string
	StartedAt     time.Time
	ConfigPath    string
	StarterConfig string
}

func New(logger *zap.Logger, store *memory.Store, searchSvc *memory.SearchService, ragSearchSvc *rag.RAGSearchService, documentLister DocumentLister, progress *rag.Progress, infManager *inference.Manager, assembler *inference.ContextAssembler, infCfg *config.InferenceConfig, executor *tools.Executor, mcpHandler http.Handler, dashboardCfg *config.DashboardConfig, dashboardFS fs.FS, runtimeInfo RuntimeInfo) *Server {
	s := &Server{
		router:           chi.NewRouter(),
		logger:           logger,
		store:            store,
		searchSvc:        searchSvc,
		ragSearchSvc:     ragSearchSvc,
		documentLister:   documentLister,
		progress:         progress,
		inferenceManager: infManager,
		contextAssembler: assembler,
		inferenceConfig:  infCfg,
		executor:         executor,
		mcpHandler:       mcpHandler,
		dashboardCfg:     dashboardCfg,
		dashboardFS:      dashboardFS,
		runtimeInfo:      runtimeInfo,
	}
	s.routes()
	return s
}

// SetRebuildFunc sets the function called by POST /index/rebuild.
func (s *Server) SetRebuildFunc(fn func() error) {
	s.rebuildFunc = fn
}

func maxBodySize(limit int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Body != nil && (r.Method == http.MethodPost || r.Method == http.MethodPut) {
				r.Body = http.MaxBytesReader(w, r.Body, limit)
			}
			next.ServeHTTP(w, r)
		})
	}
}

func (s *Server) routes() {
	s.router.Use(middleware.Recoverer)
	s.router.Use(middleware.RequestID)
	s.router.Use(maxBodySize(1 << 20)) // 1MB
	s.router.Get("/health", s.handleHealth)
	s.router.Get("/config", s.handleGetConfig)
	s.router.Put("/config", s.handleUpdateConfig)
	s.router.Route("/memories", func(r chi.Router) {
		r.Post("/", s.handleCreateMemory)
		r.Get("/search", s.handleSearchMemories)
		r.Get("/{id}", s.handleGetMemory)
		r.Put("/{id}", s.handleUpdateMemory)
		r.Delete("/{id}", s.handleDeleteMemory)
	})
	s.router.Get("/index/status", s.handleIndexStatus)
	s.router.Post("/index/rebuild", s.handleIndexRebuild)
	s.router.Get("/documents", s.handleListDocuments)
	s.router.Get("/search/documents", s.handleSearchDocuments)
	s.router.Post("/v1/chat/completions", s.handleChatCompletions)
	s.router.Get("/v1/models", s.handleListModels)
	s.router.Route("/conversations", func(r chi.Router) {
		r.Get("/", s.handleListConversations)
		r.Get("/search", s.handleSearchConversations)
		r.Get("/{id}", s.handleGetConversation)
		r.Delete("/{id}", s.handleDeleteConversation)
	})
	s.router.Get("/tools", s.handleListTools)
	s.router.Post("/tools/execute", s.handleExecuteTool)
	s.router.Get("/tools/audit", s.handleToolAudit)
	if s.mcpHandler != nil {
		s.router.Handle("/mcp", s.mcpHandler)
		s.router.Handle("/mcp/*", s.mcpHandler)
	}
	if s.dashboardCfg != nil && s.dashboardCfg.Enabled && s.dashboardFS != nil {
		s.router.Handle("/dashboard", http.RedirectHandler("/dashboard/", http.StatusMovedPermanently))
		s.router.Handle("/dashboard/*", s.handleDashboard())
	}
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	uptime := "0s"
	if !s.runtimeInfo.StartedAt.IsZero() {
		uptime = time.Since(s.runtimeInfo.StartedAt).Round(time.Second).String()
	}
	version := s.runtimeInfo.Version
	if version == "" {
		version = "dev"
	}
	json.NewEncoder(w).Encode(map[string]string{
		"status":  "ok",
		"version": version,
		"uptime":  uptime,
	})
}

func (s *Server) Handler() http.Handler {
	return s.router
}
