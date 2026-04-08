package mcp

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jxroo/kairos/internal/memory"
	"github.com/jxroo/kairos/internal/rag"
	"github.com/jxroo/kairos/internal/tools"
	mcpclient "github.com/mark3labs/mcp-go/client"
	mcpgo "github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
	"go.uber.org/zap"

	_ "github.com/mattn/go-sqlite3"
)

func callToolText(t *testing.T, client *mcpclient.Client, ctx context.Context, name string, args map[string]any) string {
	t.Helper()

	req := mcpgo.CallToolRequest{}
	req.Params.Name = name
	req.Params.Arguments = args

	result, err := client.CallTool(ctx, req)
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	if result.IsError {
		t.Fatalf("%s returned MCP error: %v", name, result.Content)
	}
	if len(result.Content) == 0 {
		t.Fatalf("%s returned no content", name)
	}

	text, ok := result.Content[0].(mcpgo.TextContent)
	if !ok {
		t.Fatalf("%s returned unexpected content type %T", name, result.Content[0])
	}

	return text.Text
}

// TestIntegration_MCPEndToEnd wires all services in-process, connects via MCP,
// and exercises: kairos_remember → kairos_recall → kairos_run_tool (calc) →
// kairos_status, then verifies audit log entries.
func TestIntegration_MCPEndToEnd(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "integration.db")
	logger := zap.NewNop()

	// 1. Create core services.
	store, err := memory.NewStore(dbPath, logger)
	if err != nil {
		t.Fatalf("creating store: %v", err)
	}
	defer store.Close()

	embedder := memory.NewFallbackEmbedder()
	vecIndex, err := memory.NewFallbackIndex(filepath.Join(tmpDir, "vec.gob"))
	if err != nil {
		t.Fatalf("creating vector index: %v", err)
	}
	defer vecIndex.Close()

	searchSvc := memory.NewSearchService(store, embedder, vecIndex, logger)

	// 2. Create tool system.
	registry := tools.NewRegistry()
	perms := tools.NewPermissionChecker(nil, logger) // deny all by default
	audit := tools.NewAuditLogger(store.DB(), logger)
	sandbox := tools.NewSandboxRunner(perms, 5*time.Second, logger)
	tools.RegisterBuiltins(registry, perms, sandbox, logger)
	executor := tools.NewExecutor(registry, perms, sandbox, audit, 5*time.Second, logger)

	// 3. Create MCP server.
	mcpSrv := New(store, searchSvc, nil, executor, nil, rag.NewProgress(), logger)

	// 4. Connect in-process MCP client.
	client, err := mcpclient.NewInProcessClient(mcpSrv.MCPServer())
	if err != nil {
		t.Fatalf("creating in-process client: %v", err)
	}
	defer client.Close()

	ctx := context.Background()
	initReq := mcpgo.InitializeRequest{}
	initReq.Params.ClientInfo = mcpgo.Implementation{Name: "test", Version: "0.0.1"}
	initReq.Params.ProtocolVersion = mcpgo.LATEST_PROTOCOL_VERSION

	if _, err := client.Initialize(ctx, initReq); err != nil {
		t.Fatalf("Initialize: %v", err)
	}

	// 5. List tools — should include all 6 Kairos tools.
	toolsList, err := client.ListTools(ctx, mcpgo.ListToolsRequest{})
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	toolNames := make(map[string]bool)
	for _, tool := range toolsList.Tools {
		toolNames[tool.Name] = true
	}
	for _, expected := range []string{"kairos_remember", "kairos_recall", "kairos_run_tool", "kairos_status", "kairos_conversations", "kairos_search_files"} {
		if !toolNames[expected] {
			t.Errorf("missing tool %q in ListTools", expected)
		}
	}

	// 6. kairos_remember — store a memory.
	rememberReq := mcpgo.CallToolRequest{}
	rememberReq.Params.Name = "kairos_remember"
	rememberReq.Params.Arguments = map[string]any{
		"content":    "Integration test memory: The answer is 42",
		"tags":       []any{"test", "integration"},
		"importance": "high",
	}
	rememberResult, err := client.CallTool(ctx, rememberReq)
	if err != nil {
		t.Fatalf("kairos_remember: %v", err)
	}
	if rememberResult.IsError {
		t.Fatalf("kairos_remember error: %v", rememberResult.Content)
	}
	rememberText := rememberResult.Content[0].(mcpgo.TextContent).Text
	if !strings.Contains(rememberText, "Memory stored") {
		t.Errorf("kairos_remember: unexpected result: %s", rememberText)
	}

	// 7. kairos_recall — search for the memory.
	recallText := callToolText(t, client, ctx, "kairos_recall", map[string]any{
		"query":         "answer 42",
		"limit":         float64(5),
		"tags":          []any{"test"},
		"min_relevance": float64(0.1),
	})
	if !strings.Contains(recallText, "Integration test memory: The answer is 42") {
		t.Fatalf("kairos_recall missing remembered content: %s", recallText)
	}
	if !strings.Contains(recallText, "Tags:") || !strings.Contains(recallText, "test") || !strings.Contains(recallText, "integration") {
		t.Fatalf("kairos_recall missing tags: %s", recallText)
	}

	// 8. kairos_run_tool — execute calc.
	runToolText := callToolText(t, client, ctx, "kairos_run_tool", map[string]any{
		"tool_name": "calc",
		"arguments": map[string]any{"expression": "6 * 7"},
	})
	if runToolText != "42" {
		t.Errorf("calc(6*7) = %q, want %q", runToolText, "42")
	}

	// 9. kairos_status — check system status.
	statusText := callToolText(t, client, ctx, "kairos_status", map[string]any{})
	var status map[string]any
	if err := json.Unmarshal([]byte(statusText), &status); err != nil {
		t.Fatalf("unmarshal kairos_status: %v\nraw: %s", err, statusText)
	}
	if got := status["version"]; got != "0.4.0" {
		t.Errorf("kairos_status version = %v, want %q", got, "0.4.0")
	}
	if got := status["memory_count"]; got != float64(1) {
		t.Errorf("kairos_status memory_count = %v, want %v", got, float64(1))
	}
	indexStatus, ok := status["index"].(map[string]any)
	if !ok {
		t.Fatalf("kairos_status missing index status: %v", status)
	}
	if _, ok := indexStatus["state"]; !ok {
		t.Errorf("kairos_status index missing state: %v", indexStatus)
	}

	// 10. Verify audit log entries.
	entries, err := audit.List(ctx, 50)
	if err != nil {
		t.Fatalf("audit.List: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("audit entries = %d, want 1", len(entries))
	}
	entry := entries[0]
	if entry.ToolName != "calc" {
		t.Errorf("audit tool_name = %q, want %q", entry.ToolName, "calc")
	}
	if entry.Result != "42" {
		t.Errorf("audit calc result = %q, want %q", entry.Result, "42")
	}
	if entry.Caller != "mcp" {
		t.Errorf("audit caller = %q, want %q", entry.Caller, "mcp")
	}
	if entry.IsError {
		t.Errorf("audit IsError = %v, want false", entry.IsError)
	}
	if !strings.Contains(entry.Arguments, `"expression":"6 * 7"`) {
		t.Errorf("audit arguments missing expression: %s", entry.Arguments)
	}
}

// TestIntegration_MCPStdioTransport verifies that ServeStdio-related plumbing
// compiles and the MCPServer is correctly configured.
func TestIntegration_MCPStdioTransport(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "stdio.db")
	logger := zap.NewNop()

	store, err := memory.NewStore(dbPath, logger)
	if err != nil {
		t.Fatalf("creating store: %v", err)
	}
	defer store.Close()

	mcpSrv := New(store, nil, nil, nil, nil, rag.NewProgress(), logger)

	// Just verify that MCPServer is properly created and ServeStdio would be
	// callable (we can't actually run it in a test).
	underlying := mcpSrv.MCPServer()
	if underlying == nil {
		t.Fatal("MCPServer() returned nil")
	}
	_ = mcpserver.NewStdioServer(underlying)
}
