package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"
	"go.uber.org/zap"

	"github.com/jxroo/kairos/internal/config"
	"github.com/jxroo/kairos/internal/inference"
	"github.com/jxroo/kairos/internal/logging"
	mcpkg "github.com/jxroo/kairos/internal/mcp"
	"github.com/jxroo/kairos/internal/memory"
	"github.com/jxroo/kairos/internal/rag"
	"github.com/jxroo/kairos/internal/tools"
	"github.com/jxroo/kairos/internal/vecbridge"
	mcpserver "github.com/mark3labs/mcp-go/server"
)

var mcpCmd = &cobra.Command{
	Use:   "mcp",
	Short: "Start Kairos as an MCP server (stdio transport)",
	Long:  `Runs Kairos as an MCP server over stdio for integration with Claude Desktop, Cursor, and other MCP clients.`,
	RunE:  runMCP,
}

func init() {
	mcpCmd.Flags().Bool("stdio", true, "Use stdio transport (default)")
	rootCmd.AddCommand(mcpCmd)
}

func runMCP(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}

	dataDir, err := config.DefaultDataDir()
	if err != nil {
		return fmt.Errorf("getting data directory: %w", err)
	}

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
	default:
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
	var progress *rag.Progress

	if cfg.RAG.Enabled {
		ragStore, err := rag.NewStore(dbPath, logger)
		if err != nil {
			logger.Warn("RAG store init failed", zap.Error(err))
		} else {
			defer ragStore.Close()

			ragIndexPath := filepath.Join(dataDir, "rag_vectors.gob")
			ragIndex, err := memory.NewFallbackIndex(ragIndexPath)
			if err == nil {
				defer ragIndex.Close()

				blevePath := filepath.Join(dataDir, "bleve_index")
				bleveIdx, err := rag.OpenOrCreateBleve(blevePath)
				if err == nil {
					defer bleveIdx.Close()
					ragSearchSvc = rag.NewRAGSearchService(ragStore, embedder, ragIndex, bleveIdx, logger)
					progress = rag.NewProgress()
				}
			}
		}
	}
	if progress == nil {
		progress = rag.NewProgress()
	}

	// Inference.
	infManager := inference.NewManager(logger)
	if err := infManager.Discover(ctx, &cfg.Inference); err != nil {
		logger.Warn("inference discovery failed", zap.Error(err))
	}

	// Tools.
	toolsDir := cfg.Tools.Dir
	if toolsDir == "" {
		toolsDir = filepath.Join(dataDir, "tools")
	}
	if err := os.MkdirAll(toolsDir, 0755); err != nil {
		logger.Warn("failed to create tools directory", zap.Error(err))
	}

	registry := tools.NewRegistry()
	perms := tools.NewPermissionChecker(cfg.Tools.Permissions, logger)
	audit := tools.NewAuditLogger(store.DB(), logger)
	timeout := time.Duration(cfg.Tools.DefaultTimeout) * time.Second
	sandbox := tools.NewSandboxRunner(perms, timeout, logger)

	if cfg.Tools.EnableBuiltins {
		_ = tools.RegisterBuiltins(registry, perms, sandbox, logger)
	}
	if err := tools.LoadToolsFromDir(toolsDir, registry, sandbox, logger); err != nil {
		logger.Warn("failed to load custom tools", zap.Error(err))
	}

	executor := tools.NewExecutor(registry, perms, sandbox, audit, timeout, logger)

	if len(cfg.MCP.ExternalServers) > 0 {
		extMCP := mcpkg.NewExternalClient(registry, logger)
		if err := extMCP.ConnectAll(ctx, cfg.MCP.ExternalServers); err != nil {
			logger.Warn("external MCP connection errors", zap.Error(err))
		}
		defer extMCP.Close()
	}

	mcpSrv := mcpkg.New(store, searchSvc, ragSearchSvc, executor, infManager, progress, logger)

	if !cfg.MCP.Enabled {
		return fmt.Errorf("MCP server is disabled in config")
	}
	if !transportIncludes(cfg.MCP.Transport, "stdio") {
		return fmt.Errorf("MCP transport %q does not include stdio", cfg.MCP.Transport)
	}

	logger.Info("starting MCP server over stdio")
	return mcpserver.ServeStdio(mcpSrv.MCPServer())
}
