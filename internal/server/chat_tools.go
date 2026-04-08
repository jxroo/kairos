package server

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"go.uber.org/zap"

	"github.com/jxroo/kairos/internal/inference"
	"github.com/jxroo/kairos/internal/memory"
	"github.com/jxroo/kairos/internal/rag"
)

// ChatToolSet returns the tool specifications and executor for the agentic
// tool-use loop in chat completions. It bridges the same service calls used
// by the MCP handlers, but without MCP protocol dependency.
func ChatToolSet(
	store *memory.Store,
	searchSvc *memory.SearchService,
	ragSearch *rag.RAGSearchService,
	logger *zap.Logger,
) ([]inference.ToolSpec, inference.ToolExecutor) {
	var specs []inference.ToolSpec

	if store != nil && searchSvc != nil {
		specs = append(specs,
			inference.ToolSpec{
				Type: "function",
				Function: inference.ToolFunctionSpec{
					Name:        "kairos_remember",
					Description: "Save an important piece of information to long-term memory. Use this when the user shares facts, preferences, deadlines, or anything worth remembering for future conversations.",
					Parameters: map[string]any{
						"type": "object",
						"properties": map[string]any{
							"content": map[string]any{
								"type":        "string",
								"description": "The information to remember",
							},
							"tags": map[string]any{
								"type":        "array",
								"items":       map[string]any{"type": "string"},
								"description": "Optional tags for categorization",
							},
							"importance": map[string]any{
								"type":        "string",
								"enum":        []string{"low", "normal", "high"},
								"description": "Importance level (default: normal)",
							},
						},
						"required": []string{"content"},
					},
				},
			},
			inference.ToolSpec{
				Type: "function",
				Function: inference.ToolFunctionSpec{
					Name:        "kairos_recall",
					Description: "Search long-term memory for relevant information. Only use this when the automatically provided Relevant Memories section does not contain what you need, or when the user explicitly asks to search memories.",
					Parameters: map[string]any{
						"type": "object",
						"properties": map[string]any{
							"query": map[string]any{
								"type":        "string",
								"description": "Search query",
							},
							"limit": map[string]any{
								"type":        "integer",
								"description": "Max results to return (default: 5)",
							},
						},
						"required": []string{"query"},
					},
				},
			},
		)
	}

	if ragSearch != nil {
		specs = append(specs, inference.ToolSpec{
			Type: "function",
			Function: inference.ToolFunctionSpec{
				Name:        "kairos_search_files",
				Description: "Search the user's indexed documents and files. Only use this when the automatically provided Relevant Documents section does not answer the question, or when the user explicitly asks about their files.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"query": map[string]any{
							"type":        "string",
							"description": "Search query",
						},
						"limit": map[string]any{
							"type":        "integer",
							"description": "Max results to return (default: 5)",
						},
					},
					"required": []string{"query"},
				},
			},
		})
	}

	executor := func(ctx context.Context, name string, argsJSON string) (string, error) {
		var args map[string]any
		if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
			return "", fmt.Errorf("parsing tool arguments: %w", err)
		}

		switch name {
		case "kairos_remember":
			return executeRemember(ctx, store, searchSvc, args, logger)
		case "kairos_recall":
			return executeRecall(ctx, searchSvc, args)
		case "kairos_search_files":
			return executeSearchFiles(ctx, ragSearch, args)
		default:
			return "", fmt.Errorf("unknown tool: %s", name)
		}
	}

	return specs, executor
}

func executeRemember(ctx context.Context, store *memory.Store, searchSvc *memory.SearchService, args map[string]any, logger *zap.Logger) (string, error) {
	content, _ := args["content"].(string)
	if content == "" {
		return "error: content is required", nil
	}

	importance, _ := args["importance"].(string)

	var tags []string
	if rawTags, ok := args["tags"]; ok {
		switch v := rawTags.(type) {
		case []any:
			for _, item := range v {
				if s, ok := item.(string); ok && s != "" {
					tags = append(tags, strings.TrimSpace(s))
				}
			}
		case []string:
			tags = v
		}
	}

	mem, err := store.Create(ctx, memory.CreateMemoryInput{
		Content:    content,
		Importance: importance,
		Tags:       tags,
		Source:     "chat_tool",
	})
	if err != nil {
		return fmt.Sprintf("error: failed to create memory: %v", err), nil
	}

	if searchSvc != nil {
		if err := searchSvc.IndexMemory(ctx, mem); err != nil {
			logger.Error("chat tool: index memory failed", zap.String("id", mem.ID), zap.Error(err))
		}
	}

	return fmt.Sprintf("Memory stored (id: %s)", mem.ID), nil
}

func executeRecall(ctx context.Context, searchSvc *memory.SearchService, args map[string]any) (string, error) {
	query, _ := args["query"].(string)
	if query == "" {
		return "error: query is required", nil
	}

	limit := 5
	if l, ok := args["limit"].(float64); ok && l > 0 {
		limit = int(l)
	}

	if searchSvc == nil {
		return "error: search service unavailable", nil
	}

	results, err := searchSvc.Search(ctx, memory.SearchQuery{
		Query:        query,
		Limit:        limit,
		MinRelevance: 0.3,
	})
	if err != nil {
		return fmt.Sprintf("error: search failed: %v", err), nil
	}

	if len(results) == 0 {
		return "No memories found.", nil
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
	return sb.String(), nil
}

func executeSearchFiles(ctx context.Context, ragSearch *rag.RAGSearchService, args map[string]any) (string, error) {
	query, _ := args["query"].(string)
	if query == "" {
		return "error: query is required", nil
	}

	limit := 5
	if l, ok := args["limit"].(float64); ok && l > 0 {
		limit = int(l)
	}

	if ragSearch == nil {
		return "error: RAG search service unavailable", nil
	}

	results, err := ragSearch.Search(ctx, rag.RAGSearchQuery{
		Query: query,
		Limit: limit,
	})
	if err != nil {
		return fmt.Sprintf("error: search failed: %v", err), nil
	}

	if len(results) == 0 {
		return "No documents found.", nil
	}

	var sb strings.Builder
	for _, r := range results {
		fmt.Fprintf(&sb, "### %s (score: %.2f)\n", r.Document.Filename, r.FinalScore)
		sb.WriteString(r.Chunk.Content)
		sb.WriteString("\n\n")
	}
	return sb.String(), nil
}
