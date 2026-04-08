package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jxroo/kairos/internal/memory"
	"github.com/jxroo/kairos/internal/rag"
	mcpgo "github.com/mark3labs/mcp-go/mcp"
	"go.uber.org/zap"
)

func textResult(text string) *mcpgo.CallToolResult {
	return &mcpgo.CallToolResult{
		Content: []mcpgo.Content{mcpgo.NewTextContent(text)},
	}
}

func errorResult(text string) *mcpgo.CallToolResult {
	return &mcpgo.CallToolResult{
		Content: []mcpgo.Content{mcpgo.NewTextContent(text)},
		IsError: true,
	}
}

func parseStringSliceArg(raw any) []string {
	switch v := raw.(type) {
	case []string:
		return append([]string(nil), v...)
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok {
				s = strings.TrimSpace(s)
				if s != "" {
					out = append(out, s)
				}
			}
		}
		return out
	case string:
		if v == "" {
			return nil
		}
		parts := strings.Split(v, ",")
		out := make([]string, 0, len(parts))
		for _, part := range parts {
			part = strings.TrimSpace(part)
			if part != "" {
				out = append(out, part)
			}
		}
		return out
	default:
		return nil
	}
}

func (s *Server) handleRemember(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	args := req.GetArguments()

	content, _ := args["content"].(string)
	if content == "" {
		return errorResult("content is required"), nil
	}

	tags := parseStringSliceArg(args["tags"])
	importance, _ := args["importance"].(string)
	memContext, _ := args["context"].(string)

	input := memory.CreateMemoryInput{
		Content:    content,
		Context:    memContext,
		Importance: importance,
		Tags:       tags,
		Source:     "mcp",
	}

	mem, err := s.store.Create(ctx, input)
	if err != nil {
		s.logger.Error("kairos_remember: create failed", zap.Error(err))
		return errorResult(fmt.Sprintf("failed to create memory: %v", err)), nil
	}

	// Index for semantic search.
	if s.searchSvc != nil {
		if err := s.searchSvc.IndexMemory(ctx, mem); err != nil {
			s.logger.Error("kairos_remember: index failed", zap.String("id", mem.ID), zap.Error(err))
		}
	}

	return textResult(fmt.Sprintf("Memory stored (id: %s)", mem.ID)), nil
}

func (s *Server) handleRecall(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	args := req.GetArguments()

	query, _ := args["query"].(string)
	if query == "" {
		return errorResult("query is required"), nil
	}

	limit := 5
	if l, ok := args["limit"].(float64); ok && l > 0 {
		limit = int(l)
	}
	tags := parseStringSliceArg(args["tags"])

	minRelevance := 0.3
	if mr, ok := args["min_relevance"].(float64); ok && mr >= 0 {
		minRelevance = mr
	}

	if s.searchSvc == nil {
		return errorResult("search service unavailable"), nil
	}

	results, err := s.searchSvc.Search(ctx, memory.SearchQuery{
		Query:        query,
		Limit:        limit,
		Tags:         tags,
		MinRelevance: minRelevance,
	})
	if err != nil {
		return errorResult(fmt.Sprintf("search failed: %v", err)), nil
	}

	if len(results) == 0 {
		return textResult("No memories found."), nil
	}

	var sb strings.Builder
	for i, r := range results {
		fmt.Fprintf(&sb, "### Memory %d (score: %.2f)\n", i+1, r.FinalScore)
		sb.WriteString(r.Memory.Content)
		if len(r.Memory.Tags) > 0 {
			fmt.Fprintf(&sb, "\nTags: %s", strings.Join(r.Memory.Tags, ", "))
		}
		sb.WriteString("\n\n")
	}

	return textResult(sb.String()), nil
}

func (s *Server) handleSearchFiles(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	args := req.GetArguments()

	query, _ := args["query"].(string)
	if query == "" {
		return errorResult("query is required"), nil
	}

	limit := 5
	if l, ok := args["limit"].(float64); ok && l > 0 {
		limit = int(l)
	}

	if s.ragSearch == nil {
		return errorResult("RAG search service unavailable"), nil
	}

	results, err := s.ragSearch.Search(ctx, rag.RAGSearchQuery{
		Query: query,
		Limit: limit,
	})
	if err != nil {
		return errorResult(fmt.Sprintf("search failed: %v", err)), nil
	}

	if len(results) == 0 {
		return textResult("No documents found."), nil
	}

	var sb strings.Builder
	for _, r := range results {
		fmt.Fprintf(&sb, "### %s (score: %.2f)\n", r.Document.Filename, r.FinalScore)
		sb.WriteString(r.Chunk.Content)
		sb.WriteString("\n\n")
	}

	return textResult(sb.String()), nil
}

func (s *Server) handleRunTool(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	args := req.GetArguments()

	toolName, _ := args["tool_name"].(string)
	if toolName == "" {
		return errorResult("tool_name is required"), nil
	}

	var toolArgs map[string]any
	if a, ok := args["arguments"].(map[string]any); ok {
		toolArgs = a
	}

	if s.executor == nil {
		return errorResult("tool executor unavailable"), nil
	}

	result, err := s.executor.Execute(ctx, toolName, toolArgs, "mcp")
	if err != nil {
		return errorResult(fmt.Sprintf("tool execution failed: %v", err)), nil
	}

	out := &mcpgo.CallToolResult{
		Content: []mcpgo.Content{mcpgo.NewTextContent(result.Content)},
		IsError: result.IsError,
	}
	return out, nil
}

func (s *Server) handleConversations(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	args := req.GetArguments()

	query, _ := args["query"].(string)
	limit := 20
	if l, ok := args["limit"].(float64); ok && l > 0 {
		limit = int(l)
	}

	var (
		convs []memory.Conversation
		err   error
	)
	if strings.TrimSpace(query) != "" {
		convs, err = s.store.SearchConversations(ctx, query, limit, 0)
	} else {
		convs, err = s.store.ListConversations(ctx, limit, 0)
	}
	if err != nil {
		return errorResult(fmt.Sprintf("listing conversations: %v", err)), nil
	}

	if len(convs) == 0 {
		return textResult("No conversations found."), nil
	}

	var sb strings.Builder
	for _, c := range convs {
		title := c.Title
		if title == "" {
			title = "(untitled)"
		}
		fmt.Fprintf(&sb, "- **%s** [%s] model=%s updated=%s\n",
			title, c.ID[:8], c.Model, c.UpdatedAt.Format("2006-01-02 15:04"))
	}

	return textResult(sb.String()), nil
}

func (s *Server) handleStatus(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	status := map[string]any{
		"version": "0.4.0",
	}

	// Memory count.
	mems, err := s.store.List(ctx, memory.ListOptions{Limit: 1})
	if err == nil {
		// Use a large limit to get total count.
		allMems, _ := s.store.List(ctx, memory.ListOptions{Limit: 10000})
		status["memory_count"] = len(allMems)
		_ = mems
	}

	// RAG progress.
	if s.progress != nil {
		ps := s.progress.Status()
		status["index"] = map[string]any{
			"state":         ps.State,
			"total_files":   ps.TotalFiles,
			"indexed_files": ps.IndexedFiles,
			"percent":       ps.Percent,
		}
	}

	// Models.
	if s.infManager != nil {
		models, err := s.infManager.ListModels(ctx)
		if err == nil {
			modelNames := make([]string, len(models))
			for i, m := range models {
				modelNames[i] = m.Name
			}
			status["models"] = modelNames
		}
	}

	b, _ := json.MarshalIndent(status, "", "  ")
	return textResult(string(b)), nil
}
