package mcp

import (
	"github.com/jxroo/kairos/internal/inference"
	"github.com/jxroo/kairos/internal/memory"
	"github.com/jxroo/kairos/internal/rag"
	"github.com/jxroo/kairos/internal/tools"
	mcpgo "github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
	"go.uber.org/zap"
)

// Server wraps an mcp-go MCPServer and registers all Kairos tools.
type Server struct {
	mcpServer  *mcpserver.MCPServer
	store      *memory.Store
	searchSvc  *memory.SearchService
	ragSearch  *rag.RAGSearchService
	executor   *tools.Executor
	infManager *inference.Manager
	progress   *rag.Progress
	logger     *zap.Logger
}

// New creates an MCP Server and registers all Kairos MCP tools.
func New(
	store *memory.Store,
	searchSvc *memory.SearchService,
	ragSearch *rag.RAGSearchService,
	executor *tools.Executor,
	infManager *inference.Manager,
	progress *rag.Progress,
	logger *zap.Logger,
) *Server {
	s := &Server{
		mcpServer:  mcpserver.NewMCPServer("kairos", "0.4.0", mcpserver.WithToolCapabilities(true)),
		store:      store,
		searchSvc:  searchSvc,
		ragSearch:  ragSearch,
		executor:   executor,
		infManager: infManager,
		progress:   progress,
		logger:     logger,
	}
	s.registerTools()
	return s
}

// MCPServer returns the underlying mcp-go server for transport attachment.
func (s *Server) MCPServer() *mcpserver.MCPServer {
	return s.mcpServer
}

func (s *Server) registerTools() {
	s.mcpServer.AddTool(
		mcpgo.NewTool("kairos_remember",
			mcpgo.WithDescription("Store a new memory with content, tags, and importance level"),
			mcpgo.WithString("content", mcpgo.Description("The memory content to store"), mcpgo.Required()),
			mcpgo.WithArray("tags", mcpgo.Description("Tags for organizing the memory"), mcpgo.WithStringItems()),
			mcpgo.WithString("importance", mcpgo.Description("Importance level: low, normal, high")),
			mcpgo.WithString("context", mcpgo.Description("Additional context for the memory")),
		),
		s.handleRemember,
	)

	s.mcpServer.AddTool(
		mcpgo.NewTool("kairos_recall",
			mcpgo.WithDescription("Search memories by semantic similarity"),
			mcpgo.WithString("query", mcpgo.Description("Search query"), mcpgo.Required()),
			mcpgo.WithNumber("limit", mcpgo.Description("Maximum results to return")),
			mcpgo.WithArray("tags", mcpgo.Description("Required tags to filter by"), mcpgo.WithStringItems()),
			mcpgo.WithNumber("min_relevance", mcpgo.Description("Minimum final relevance score")),
		),
		s.handleRecall,
	)

	s.mcpServer.AddTool(
		mcpgo.NewTool("kairos_search_files",
			mcpgo.WithDescription("Search indexed files and documents using hybrid semantic + keyword search"),
			mcpgo.WithString("query", mcpgo.Description("Search query"), mcpgo.Required()),
			mcpgo.WithNumber("limit", mcpgo.Description("Maximum results to return")),
		),
		s.handleSearchFiles,
	)

	s.mcpServer.AddTool(
		mcpgo.NewTool("kairos_run_tool",
			mcpgo.WithDescription("Execute a registered tool by name with given arguments"),
			mcpgo.WithString("tool_name", mcpgo.Description("Name of the tool to execute"), mcpgo.Required()),
			mcpgo.WithObject("arguments", mcpgo.Description("Tool arguments as key-value pairs")),
		),
		s.handleRunTool,
	)

	s.mcpServer.AddTool(
		mcpgo.NewTool("kairos_conversations",
			mcpgo.WithDescription("List or search past conversations"),
			mcpgo.WithString("query", mcpgo.Description("Optional search query over titles and messages")),
			mcpgo.WithNumber("limit", mcpgo.Description("Maximum conversations to return")),
		),
		s.handleConversations,
	)

	s.mcpServer.AddTool(
		mcpgo.NewTool("kairos_status",
			mcpgo.WithDescription("Get Kairos system status including memory count, index stats, and available models"),
		),
		s.handleStatus,
	)
}
