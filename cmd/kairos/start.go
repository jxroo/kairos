package main

import (
	"context"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"go.uber.org/zap"

	"github.com/jxroo/kairos/dashboard"
	"github.com/jxroo/kairos/internal/config"
	"github.com/jxroo/kairos/internal/daemon"
	"github.com/jxroo/kairos/internal/inference"
	"github.com/jxroo/kairos/internal/logging"
	mcpkg "github.com/jxroo/kairos/internal/mcp"
	"github.com/jxroo/kairos/internal/memory"
	"github.com/jxroo/kairos/internal/rag"
	"github.com/jxroo/kairos/internal/server"
	"github.com/jxroo/kairos/internal/tools"
	"github.com/jxroo/kairos/internal/vecbridge"
	buildversion "github.com/jxroo/kairos/internal/version"
)

var startCmd = &cobra.Command{
	Use:   "start",
	Short: "Start the Kairos daemon",
	RunE:  runStart,
}

func init() {
	rootCmd.AddCommand(startCmd)
}

func runStart(cmd *cobra.Command, args []string) error {
	dataDir, err := config.DefaultDataDir()
	if err != nil {
		return fmt.Errorf("getting data directory: %w", err)
	}
	configPath := filepath.Join(dataDir, "config.toml")
	startedAt := time.Now().UTC()

	cfg, err := config.Load(dataDir)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	logDir := filepath.Join(dataDir, "logs")
	logger, err := logging.New(cfg.Log.Level, cfg.Log.Format, logDir)
	if err != nil {
		return fmt.Errorf("creating logger: %w", err)
	}
	defer logger.Sync()

	dbPath := filepath.Join(dataDir, "kairos.db")
	store, err := memory.NewStore(dbPath, logger)
	if err != nil {
		return fmt.Errorf("creating memory store: %w", err)
	}
	defer store.Close()

	var embedder memory.Embedder
	var index memory.VectorIndex

	switch cfg.Memory.Engine {
	case "rust":
		re, err := vecbridge.NewRustEmbedder(dataDir)
		if err != nil {
			logger.Warn("rust engine init failed, falling back", zap.Error(err))
			embedder = memory.NewFallbackEmbedder()
			idx, idxErr := memory.NewFallbackIndex(filepath.Join(dataDir, "vectors.gob"))
			if idxErr != nil {
				return fmt.Errorf("creating fallback vector index: %w", idxErr)
			}
			index = idx
		} else {
			embedder = re
			ri, riErr := vecbridge.NewRustIndex(dataDir)
			if riErr != nil {
				return fmt.Errorf("creating rust vector index: %w", riErr)
			}
			index = ri
		}
	default: // "fallback"
		embedder = memory.NewFallbackEmbedder()
		idx, idxErr := memory.NewFallbackIndex(filepath.Join(dataDir, "vectors.gob"))
		if idxErr != nil {
			return fmt.Errorf("creating vector index: %w", idxErr)
		}
		index = idx
	}
	defer index.Close()

	decayCfg := memory.DecayConfig{
		Factor:    cfg.Memory.Decay.Factor,
		Threshold: cfg.Memory.Decay.Threshold,
	}
	store.SetDecayConfig(decayCfg)
	searchSvc := memory.NewSearchService(store, embedder, index, logger, decayCfg)

	// RAG components.
	var ragSearchSvc *rag.RAGSearchService
	var ragIndexer *rag.Indexer
	var ragStore *rag.Store
	var progress *rag.Progress
	var watcher *rag.Watcher

	if cfg.RAG.Enabled {
		ragStore, err = rag.NewStore(dbPath, logger)
		if err != nil {
			return fmt.Errorf("creating RAG store: %w", err)
		}
		defer ragStore.Close()

		ragIndexPath := filepath.Join(dataDir, "rag_vectors.gob")
		ragIndex, err := memory.NewFallbackIndex(ragIndexPath)
		if err != nil {
			return fmt.Errorf("creating RAG vector index: %w", err)
		}
		defer ragIndex.Close()

		blevePath := filepath.Join(dataDir, "bleve_index")
		bleveIdx, err := rag.OpenOrCreateBleve(blevePath)
		if err != nil {
			return fmt.Errorf("creating bleve index: %w", err)
		}
		defer bleveIdx.Close()

		registry := rag.DefaultRegistry()
		chunker := rag.NewChunker(cfg.RAG.ChunkSize, cfg.RAG.ChunkOverlap)
		progress = rag.NewProgress()
		ragIndexer = rag.NewIndexer(ragStore, embedder, ragIndex, bleveIdx, registry, chunker, progress, &cfg.RAG, logger)
		ragSearchSvc = rag.NewRAGSearchService(ragStore, embedder, ragIndex, bleveIdx, logger)

		watcher = rag.NewWatcher(ragIndexer, &cfg.RAG, progress, logger)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Prune decayed memories at startup and every 24 hours.
	if n, err := store.PruneDecayed(ctx, decayCfg); err != nil {
		logger.Warn("startup memory prune failed", zap.Error(err))
	} else if n > 0 {
		logger.Info("pruned decayed memories at startup", zap.Int("count", n))
	}
	go func() {
		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if n, err := store.PruneDecayed(ctx, decayCfg); err != nil {
					logger.Warn("periodic memory prune failed", zap.Error(err))
				} else if n > 0 {
					logger.Info("pruned decayed memories", zap.Int("count", n))
				}
			}
		}
	}()

	// Inference components.
	infManager := inference.NewManager(logger)
	if err := infManager.Discover(ctx, &cfg.Inference); err != nil {
		logger.Warn("inference discovery failed", zap.Error(err))
		// Not fatal — daemon still starts, inference just unavailable.
	}

	var memorySrch inference.MemorySearcher = &memorySearchAdapter{
		svc:          searchSvc,
		minRelevance: cfg.Memory.Search.MinRelevance,
	}
	var ragSrch inference.RAGSearcher
	if ragSearchSvc != nil {
		ragSrch = &ragSearchAdapter{svc: ragSearchSvc}
	}
	assembler := inference.NewContextAssembler(memorySrch, ragSrch, logger)

	// Tool runtime.
	toolsDir := cfg.Tools.Dir
	if toolsDir == "" {
		toolsDir = filepath.Join(dataDir, "tools")
	}
	if err := os.MkdirAll(toolsDir, 0755); err != nil {
		logger.Warn("failed to create tools directory", zap.Error(err))
	}

	registry := tools.NewRegistry()
	perms := tools.NewPermissionChecker(cfg.Tools.Permissions, logger)
	auditLogger := tools.NewAuditLogger(store.DB(), logger)
	timeout := time.Duration(cfg.Tools.DefaultTimeout) * time.Second
	sandbox := tools.NewSandboxRunner(perms, timeout, logger)

	if cfg.Tools.EnableBuiltins {
		if err := tools.RegisterBuiltins(registry, perms, sandbox, logger); err != nil {
			logger.Warn("failed to register built-in tools", zap.Error(err))
		}
	}
	if err := tools.LoadToolsFromDir(toolsDir, registry, sandbox, logger); err != nil {
		logger.Warn("failed to load custom tools", zap.Error(err))
	}

	executor := tools.NewExecutor(registry, perms, sandbox, auditLogger, timeout, logger)

	// MCP server.
	var mcpHTTPHandler http.Handler
	if cfg.MCP.Enabled {
		mcpSrv := mcpkg.New(store, searchSvc, ragSearchSvc, executor, infManager, progress, logger)
		if transportIncludes(cfg.MCP.Transport, "sse") {
			mcpHTTPHandler = mcpSrv.SSEHandler(mcpkg.DefaultSSEBasePath)
		}
	}

	// External MCP clients.
	extMCP := mcpkg.NewExternalClient(registry, logger)
	if len(cfg.MCP.ExternalServers) > 0 {
		if err := extMCP.ConnectAll(ctx, cfg.MCP.ExternalServers); err != nil {
			logger.Warn("external MCP connection errors", zap.Error(err))
		}
		defer extMCP.Close()
	}

	// Dashboard.
	var dashCfg *config.DashboardConfig
	var dashFS fs.FS
	if cfg.Dashboard.Enabled {
		dashCfg = &cfg.Dashboard
		sub, err := fs.Sub(dashboard.Assets, "dist")
		if err != nil {
			logger.Warn("failed to load dashboard assets", zap.Error(err))
		} else {
			dashFS = sub
		}
	}

	srv := server.New(
		logger,
		store,
		searchSvc,
		ragSearchSvc,
		ragStore,
		progress,
		infManager,
		assembler,
		&cfg.Inference,
		executor,
		mcpHTTPHandler,
		dashCfg,
		dashFS,
		server.RuntimeInfo{
			Version:       buildversion.Version,
			StartedAt:     startedAt,
			ConfigPath:    configPath,
			StarterConfig: config.StarterTOML("rust"),
		},
	)

	if ragIndexer != nil {
		srv.SetRebuildFunc(func() error {
			return ragIndexer.RebuildAll(ctx, cfg.RAG.WatchPaths)
		})
	}

	d := daemon.New(cfg, srv, logger)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		logger.Info("received shutdown signal")
		cancel()
	}()

	if watcher != nil {
		if err := watcher.Start(ctx); err != nil {
			return fmt.Errorf("starting file watcher: %w", err)
		}
		defer func() { _ = watcher.Stop() }()
	}

	fmt.Printf("Kairos daemon starting on %s:%d\n", cfg.Server.Host, cfg.Server.Port)
	return d.Run(ctx)
}

// memorySearchAdapter bridges memory.SearchService to inference.MemorySearcher.
type memorySearchAdapter struct {
	svc          *memory.SearchService
	minRelevance float64
}

func (a *memorySearchAdapter) Search(ctx context.Context, query string, limit int) ([]inference.MemoryResult, error) {
	results, err := a.svc.Search(ctx, memory.SearchQuery{
		Query:        query,
		Limit:        limit,
		MinRelevance: a.minRelevance,
	})
	if err != nil {
		return nil, err
	}
	out := make([]inference.MemoryResult, len(results))
	for i, r := range results {
		out[i] = inference.MemoryResult{
			Content:    r.Memory.Content,
			Score:      r.FinalScore,
			Tags:       r.Memory.Tags,
			Importance: r.Memory.Importance,
			CreatedAt:  r.Memory.CreatedAt.Format("2006-01-02"),
		}
	}
	return out, nil
}

// ragSearchAdapter bridges rag.RAGSearchService to inference.RAGSearcher.
type ragSearchAdapter struct {
	svc *rag.RAGSearchService
}

func (a *ragSearchAdapter) Search(ctx context.Context, query string, limit int) ([]inference.RAGResult, error) {
	results, err := a.svc.Search(ctx, rag.RAGSearchQuery{
		Query: query,
		Limit: limit,
	})
	if err != nil {
		return nil, err
	}
	out := make([]inference.RAGResult, len(results))
	for i, r := range results {
		out[i] = inference.RAGResult{
			Content: r.Chunk.Content,
			Source:  r.Document.Filename,
			Score:   r.FinalScore,
		}
	}
	return out, nil
}
