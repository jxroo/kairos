package mcp

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jxroo/kairos/internal/memory"
	"github.com/jxroo/kairos/internal/rag"
	"github.com/jxroo/kairos/internal/tools"
	mcpgo "github.com/mark3labs/mcp-go/mcp"
	"go.uber.org/zap"

	_ "github.com/mattn/go-sqlite3"
)

func setupTestServer(t *testing.T) *Server {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := memory.NewStore(dbPath, zap.NewNop())
	if err != nil {
		t.Fatalf("creating store: %v", err)
	}
	t.Cleanup(func() { store.Close() })

	embedder := memory.NewFallbackEmbedder()
	idx, err := memory.NewFallbackIndex(filepath.Join(t.TempDir(), "vec.gob"))
	if err != nil {
		t.Fatalf("creating index: %v", err)
	}
	t.Cleanup(func() { idx.Close() })

	searchSvc := memory.NewSearchService(store, embedder, idx, zap.NewNop())

	// Set up a minimal executor with calc.
	registry := tools.NewRegistry()
	perms := tools.NewPermissionChecker(nil, zap.NewNop())
	audit := tools.NewAuditLogger(store.DB(), zap.NewNop())
	sandbox := tools.NewSandboxRunner(perms, 5*time.Second, zap.NewNop())
	tools.RegisterBuiltins(registry, perms, sandbox, zap.NewNop())
	executor := tools.NewExecutor(registry, perms, sandbox, audit, 5*time.Second, zap.NewNop())

	return New(store, searchSvc, nil, executor, nil, rag.NewProgress(), zap.NewNop())
}

func callTool(name string, args map[string]any) mcpgo.CallToolRequest {
	return mcpgo.CallToolRequest{
		Params: mcpgo.CallToolParams{
			Name:      name,
			Arguments: args,
		},
	}
}

func TestHandleRemember(t *testing.T) {
	s := setupTestServer(t)
	ctx := context.Background()

	result, err := s.handleRemember(ctx, callTool("kairos_remember", map[string]any{
		"content":    "Test memory content",
		"tags":       []any{"test", "mcp"},
		"importance": "high",
	}))
	if err != nil {
		t.Fatalf("handleRemember: %v", err)
	}
	if result.IsError {
		t.Errorf("unexpected error: %v", result.Content)
	}
	text := result.Content[0].(mcpgo.TextContent).Text
	if !strings.Contains(text, "Memory stored") {
		t.Errorf("unexpected result: %s", text)
	}
}

func TestHandleRemember_EmptyContent(t *testing.T) {
	s := setupTestServer(t)
	result, _ := s.handleRemember(context.Background(), callTool("kairos_remember", map[string]any{}))
	if !result.IsError {
		t.Error("expected error for empty content")
	}
}

func TestHandleRecall(t *testing.T) {
	s := setupTestServer(t)
	ctx := context.Background()

	// Store a memory first.
	s.handleRemember(ctx, callTool("kairos_remember", map[string]any{
		"content": "The capital of France is Paris",
		"tags":    []any{"geography"},
	}))
	s.handleRemember(ctx, callTool("kairos_remember", map[string]any{
		"content": "France deployment notes",
		"tags":    []any{"ops"},
	}))

	result, err := s.handleRecall(ctx, callTool("kairos_recall", map[string]any{
		"query":         "France",
		"limit":         float64(5),
		"tags":          []any{"geography"},
		"min_relevance": float64(0.1),
	}))
	if err != nil {
		t.Fatalf("handleRecall: %v", err)
	}
	if result.IsError {
		t.Errorf("unexpected error: %v", result.Content)
	}
	text := result.Content[0].(mcpgo.TextContent).Text
	if !strings.Contains(text, "capital of France is Paris") {
		t.Fatalf("expected filtered memory in result: %s", text)
	}
	if strings.Contains(text, "deployment notes") {
		t.Fatalf("unexpected tag-mismatched memory in result: %s", text)
	}
}

func TestHandleRunTool_Calc(t *testing.T) {
	s := setupTestServer(t)
	result, err := s.handleRunTool(context.Background(), callTool("kairos_run_tool", map[string]any{
		"tool_name": "calc",
		"arguments": map[string]any{"expression": "2+2"},
	}))
	if err != nil {
		t.Fatalf("handleRunTool: %v", err)
	}
	text := result.Content[0].(mcpgo.TextContent).Text
	if text != "4" {
		t.Errorf("calc(2+2) = %q, want %q", text, "4")
	}
}

func TestHandleRunTool_NotFound(t *testing.T) {
	s := setupTestServer(t)
	result, _ := s.handleRunTool(context.Background(), callTool("kairos_run_tool", map[string]any{
		"tool_name": "nonexistent",
	}))
	if !result.IsError {
		t.Error("expected error for nonexistent tool")
	}
}

func TestHandleConversations(t *testing.T) {
	s := setupTestServer(t)
	ctx := context.Background()

	conv, _ := s.store.CreateConversation(ctx, "Test Chat", "gpt-4")
	if err := s.store.AddMessage(ctx, conv.ID, memory.ConversationMessage{
		Role:    "user",
		Content: "Follow up on database migration",
	}); err != nil {
		t.Fatalf("AddMessage: %v", err)
	}

	result, err := s.handleConversations(ctx, callTool("kairos_conversations", map[string]any{
		"query": "migration",
	}))
	if err != nil {
		t.Fatalf("handleConversations: %v", err)
	}
	text := result.Content[0].(mcpgo.TextContent).Text
	if !strings.Contains(text, "Test Chat") {
		t.Errorf("expected 'Test Chat' in result: %s", text)
	}
}

func TestHandleStatus(t *testing.T) {
	s := setupTestServer(t)
	result, err := s.handleStatus(context.Background(), callTool("kairos_status", map[string]any{}))
	if err != nil {
		t.Fatalf("handleStatus: %v", err)
	}
	text := result.Content[0].(mcpgo.TextContent).Text
	if !strings.Contains(text, "version") {
		t.Errorf("expected version in status: %s", text)
	}
	if !strings.Contains(text, "memory_count") {
		t.Errorf("expected memory_count in status: %s", text)
	}
}
