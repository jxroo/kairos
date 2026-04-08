package server

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/jxroo/kairos/internal/inference"
	"github.com/jxroo/kairos/internal/memory"
)

// openAIChoice is a single choice in an OpenAI chat.completion response.
type openAIChoice struct {
	Index        int               `json:"index"`
	Message      inference.Message `json:"message"`
	FinishReason string            `json:"finish_reason"`
}

// openAIUsage is the usage block in an OpenAI chat.completion response.
type openAIUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// openAIChatResponse is the full OpenAI-format chat completion response body.
type openAIChatResponse struct {
	ID      string         `json:"id"`
	Object  string         `json:"object"`
	Model   string         `json:"model"`
	Choices []openAIChoice `json:"choices"`
	Usage   openAIUsage    `json:"usage"`
}

// openAIModelEntry is a single model entry in a /v1/models response.
type openAIModelEntry struct {
	ID           string   `json:"id"`
	Object       string   `json:"object"`
	OwnedBy      string   `json:"owned_by"`
	ContextSize  int      `json:"context_length,omitempty"`
	SizeBytes    int64    `json:"size_bytes,omitempty"`
	Capabilities []string `json:"capabilities,omitempty"`
}

// openAIModelsResponse is the /v1/models response body.
type openAIModelsResponse struct {
	Object string             `json:"object"`
	Data   []openAIModelEntry `json:"data"`
}

// handleListModels implements GET /v1/models.
func (s *Server) handleListModels(w http.ResponseWriter, r *http.Request) {
	if s.inferenceManager == nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, `{"error":"inference not available"}`, http.StatusServiceUnavailable)
		return
	}

	models, err := s.inferenceManager.ListModels(r.Context())
	if err != nil {
		s.logger.Error("listing models failed", zap.Error(err))
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, `{"error":"failed to list models"}`, http.StatusInternalServerError)
		return
	}

	entries := make([]openAIModelEntry, 0, len(models))
	for _, m := range models {
		ownedBy := m.Provider
		if ownedBy == "" {
			ownedBy = "kairos"
		}
		entries = append(entries, openAIModelEntry{
			ID:           m.ID,
			Object:       "model",
			OwnedBy:      ownedBy,
			ContextSize:  m.ContextSize,
			SizeBytes:    m.SizeBytes,
			Capabilities: m.Capabilities,
		})
	}

	resp := openAIModelsResponse{
		Object: "list",
		Data:   entries,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// handleChatCompletions implements POST /v1/chat/completions.
func (s *Server) handleChatCompletions(w http.ResponseWriter, r *http.Request) {
	if s.inferenceManager == nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, `{"error":"inference not available"}`, http.StatusServiceUnavailable)
		return
	}

	var req inference.ChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, fmt.Sprintf(`{"error":"invalid request body: %v"}`, err), http.StatusBadRequest)
		return
	}

	ctx := r.Context()

	var (
		conv *memory.Conversation
		err  error
	)

	// Resolve or create conversation.
	convID := r.Header.Get("X-Conversation-Id")
	if convID != "" && s.store != nil {
		conv, err = s.store.GetConversation(ctx, convID)
		if err != nil {
			if !errors.Is(err, sql.ErrNoRows) {
				s.logger.Warn("loading conversation failed", zap.Error(err))
			}
			convID = ""
		}
	}

	if convID != "" && s.store != nil {
		if req.Model == "" && conv != nil && conv.Model != "" {
			req.Model = conv.Model
		}
		// Load existing conversation history and prepend it.
		msgs, err := s.store.GetMessages(ctx, convID)
		if err != nil {
			if !errors.Is(err, sql.ErrNoRows) {
				s.logger.Warn("loading conversation messages failed", zap.Error(err))
			}
			// If not found, start a fresh conversation.
			convID = ""
		} else {
			history := make([]inference.Message, 0, len(msgs))
			for _, m := range msgs {
				history = append(history, inference.Message{Role: m.Role, Content: m.Content})
			}
			req.Messages = append(history, req.Messages...)
		}
	}

	_, resolvedModel, err := s.inferenceManager.ResolveModelInfo(ctx, req.Model)
	if err != nil {
		if errors.Is(err, inference.ErrNoProviders) || errors.Is(err, inference.ErrModelNotFound) {
			w.Header().Set("Content-Type", "application/json")
			http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), http.StatusServiceUnavailable)
			return
		}
		s.logger.Error("resolving model failed", zap.Error(err))
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, `{"error":"failed to resolve model"}`, http.StatusInternalServerError)
		return
	}
	req.Model = resolvedModel.ID

	if convID == "" && s.store != nil {
		conv, err := s.store.CreateConversation(ctx, "", req.Model)
		if err != nil {
			s.logger.Warn("creating conversation failed", zap.Error(err))
		} else {
			convID = conv.ID
		}
	}

	// Extract the last user message BEFORE context assembly modifies the list.
	var userContent string
	for i := len(req.Messages) - 1; i >= 0; i-- {
		if req.Messages[i].Role == "user" {
			userContent = req.Messages[i].Content
			break
		}
	}

	// Build tool set for agentic loop (before context assembly so we can account for tool token cost).
	toolSpecs, toolExecutor := ChatToolSet(s.store, s.searchSvc, s.ragSearchSvc, s.logger)
	toolLoopEnabled := s.inferenceConfig != nil && s.inferenceConfig.ToolLoop.Enabled && len(toolSpecs) > 0

	// Assemble context (inject memory + RAG).
	if s.contextAssembler != nil {
		opts := inference.AssembleOpts{}
		if s.inferenceConfig != nil {
			opts.SystemPrompt = s.inferenceConfig.SystemPrompt
			opts.MaxTokens = s.inferenceConfig.ContextSize
			opts.ReservedTokens = s.inferenceConfig.ResponseReserve
		}
		if resolvedModel.ContextSize > 0 {
			opts.MaxTokens = resolvedModel.ContextSize
		}
		// Account for tool definitions in token budget.
		if toolLoopEnabled {
			opts.ToolOverheadTokens = inference.EstimateToolTokens(toolSpecs)
		}
		// Use request max_tokens as response reserve if specified.
		if req.MaxTokens > 0 {
			opts.ReservedTokens = req.MaxTokens
		}
		assembled, err := s.contextAssembler.Assemble(ctx, req.Messages, opts)
		if err != nil {
			s.logger.Warn("context assembly failed, continuing without enrichment", zap.Error(err))
		} else {
			req.Messages = assembled
		}
	}

	if convID != "" && s.store != nil && userContent != "" {
		if err := s.store.AddMessage(ctx, convID, memory.ConversationMessage{
			Role:    "user",
			Content: userContent,
		}); err != nil {
			s.logger.Warn("persisting user message failed", zap.Error(err))
		}
	}

	if convID != "" {
		w.Header().Set("X-Conversation-Id", convID)
	}

	// Reuse the provider already resolved above instead of a second ResolveModelInfo call.
	provider, _, _ := s.inferenceManager.ResolveModelInfo(ctx, req.Model)

	if req.Stream {
		s.handleChatStream(w, r, req, convID, provider, toolSpecs, toolExecutor, toolLoopEnabled)
		return
	}

	var (
		chatResp *inference.ChatResponse
		steps    []inference.ToolStep
	)

	if toolLoopEnabled && provider != nil {
		tlCfg := inference.ToolLoopConfig{
			Tools:   toolSpecs,
			Execute: toolExecutor,
			Logger:  s.logger,
		}
		if s.inferenceConfig != nil {
			tlCfg.MaxIterations = s.inferenceConfig.ToolLoop.MaxIterations
			tlCfg.MaxToolCalls = s.inferenceConfig.ToolLoop.MaxToolCalls
			tlCfg.Timeout = s.inferenceConfig.ToolLoop.Timeout
		}
		result, err := inference.RunToolLoop(ctx, provider, req, tlCfg)
		if err != nil {
			if errors.Is(err, inference.ErrNoProviders) || errors.Is(err, inference.ErrModelNotFound) {
				w.Header().Set("Content-Type", "application/json")
				http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), http.StatusServiceUnavailable)
				return
			}
			s.logger.Error("tool loop failed", zap.Error(err))
			w.Header().Set("Content-Type", "application/json")
			http.Error(w, `{"error":"chat completion failed"}`, http.StatusInternalServerError)
			return
		}
		chatResp = result.Response
		steps = result.Steps
	} else {
		// Non-streaming path without tool loop.
		var err error
		chatResp, err = s.inferenceManager.Chat(ctx, req)
		if err != nil {
			if errors.Is(err, inference.ErrNoProviders) || errors.Is(err, inference.ErrModelNotFound) {
				w.Header().Set("Content-Type", "application/json")
				http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), http.StatusServiceUnavailable)
				return
			}
			s.logger.Error("chat completion failed", zap.Error(err))
			w.Header().Set("Content-Type", "application/json")
			http.Error(w, `{"error":"chat completion failed"}`, http.StatusInternalServerError)
			return
		}
	}

	// Persist tool steps.
	if convID != "" && s.store != nil {
		for _, step := range steps {
			metadata, _ := json.Marshal(map[string]string{
				"tool_call_id": step.Call.ID,
				"tool_name":    step.Call.Function.Name,
			})
			_ = s.store.AddMessage(ctx, convID, memory.ConversationMessage{
				Role:     "tool",
				Content:  step.Result,
				Metadata: string(metadata),
			})
		}
	}

	// Persist assistant response.
	if convID != "" && s.store != nil {
		if err := s.store.AddMessage(ctx, convID, memory.ConversationMessage{
			Role:    chatResp.Message.Role,
			Content: chatResp.Message.Content,
			Tokens:  chatResp.Usage.CompletionTokens,
		}); err != nil {
			s.logger.Warn("persisting assistant message failed", zap.Error(err))
		}
	}

	responseID := "chatcmpl-" + uuid.New().String()
	if chatResp.ID != "" {
		responseID = chatResp.ID
	}

	finishReason := chatResp.FinishReason
	if finishReason == "" {
		finishReason = "stop"
	}

	openAIResp := openAIChatResponse{
		ID:     responseID,
		Object: "chat.completion",
		Model:  chatResp.Model,
		Choices: []openAIChoice{
			{
				Index:        0,
				Message:      chatResp.Message,
				FinishReason: finishReason,
			},
		},
		Usage: openAIUsage{
			PromptTokens:     chatResp.Usage.PromptTokens,
			CompletionTokens: chatResp.Usage.CompletionTokens,
			TotalTokens:      chatResp.Usage.TotalTokens,
		},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(openAIResp)
}

// handleChatStream handles the streaming branch of POST /v1/chat/completions.
func (s *Server) handleChatStream(
	w http.ResponseWriter, r *http.Request,
	req inference.ChatRequest, convID string,
	provider inference.Provider,
	toolSpecs []inference.ToolSpec, toolExecutor inference.ToolExecutor,
	toolLoopEnabled bool,
) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, `{"error":"streaming not supported"}`, http.StatusInternalServerError)
		return
	}

	ctx := r.Context()
	responseID := "chatcmpl-" + uuid.New().String()

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	if toolLoopEnabled && provider != nil {
		tlCfg := inference.ToolLoopConfig{
			Tools:   toolSpecs,
			Execute: toolExecutor,
			Logger:  s.logger,
		}
		if s.inferenceConfig != nil {
			tlCfg.MaxIterations = s.inferenceConfig.ToolLoop.MaxIterations
			tlCfg.MaxToolCalls = s.inferenceConfig.ToolLoop.MaxToolCalls
			tlCfg.Timeout = s.inferenceConfig.ToolLoop.Timeout
		}

		events := inference.RunToolLoopStream(ctx, provider, req, tlCfg)

		var accumulated strings.Builder
		for ev := range events {
			switch ev.Type {
			case "tool_call":
				data, _ := json.Marshal(map[string]any{
					"object":    "kairos.tool_call",
					"tool_call": ev.ToolCall,
				})
				fmt.Fprintf(w, "data: %s\n\n", data)
				flusher.Flush()

				// Persist tool call as message.
				if convID != "" && s.store != nil && ev.ToolCall != nil {
					metadata, _ := json.Marshal(map[string]string{
						"tool_call_id": ev.ToolCall.ID,
						"tool_name":    ev.ToolCall.Function.Name,
						"type":         "tool_call",
					})
					_ = s.store.AddMessage(ctx, convID, memory.ConversationMessage{
						Role:     "assistant",
						Content:  fmt.Sprintf("[tool_call: %s(%s)]", ev.ToolCall.Function.Name, ev.ToolCall.Function.Arguments),
						Metadata: string(metadata),
					})
				}

			case "tool_result":
				data, _ := json.Marshal(map[string]any{
					"object":  "kairos.tool_result",
					"content": ev.ToolResult,
				})
				fmt.Fprintf(w, "data: %s\n\n", data)
				flusher.Flush()

				// Persist tool result.
				if convID != "" && s.store != nil && ev.ToolResult != nil {
					_ = s.store.AddMessage(ctx, convID, memory.ConversationMessage{
						Role:    "tool",
						Content: *ev.ToolResult,
					})
				}

			case "stream":
				if ev.Stream != nil {
					accumulated.WriteString(ev.Stream.Delta)
					chunk := inference.StreamToSSEChunk(*ev.Stream, responseID)
					data, err := json.Marshal(chunk)
					if err != nil {
						s.logger.Warn("marshaling SSE chunk", zap.Error(err))
						continue
					}
					fmt.Fprintf(w, "data: %s\n\n", data)
					flusher.Flush()
				}

			case "error":
				if ev.Error != nil {
					s.logger.Error("tool loop stream error", zap.Error(ev.Error))
					data, _ := json.Marshal(map[string]string{
						"object": "error",
						"error":  ev.Error.Error(),
					})
					fmt.Fprintf(w, "data: %s\n\n", data)
					flusher.Flush()
				}

			case "done":
				// Will be handled after loop
			}
		}

		fmt.Fprint(w, "data: [DONE]\n\n")
		flusher.Flush()

		// Persist accumulated assistant response.
		if convID != "" && s.store != nil && accumulated.Len() > 0 {
			if err := s.store.AddMessage(ctx, convID, memory.ConversationMessage{
				Role:    "assistant",
				Content: accumulated.String(),
			}); err != nil {
				s.logger.Warn("persisting streaming assistant message failed", zap.Error(err))
			}
		}
		return
	}

	// Non-tool-loop streaming path.
	reader, err := s.inferenceManager.ChatStream(ctx, req)
	if err != nil {
		if errors.Is(err, inference.ErrNoProviders) || errors.Is(err, inference.ErrModelNotFound) {
			w.Header().Set("Content-Type", "application/json")
			http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), http.StatusServiceUnavailable)
			return
		}
		s.logger.Error("chat stream failed", zap.Error(err))
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, `{"error":"chat stream failed"}`, http.StatusInternalServerError)
		return
	}

	// Read events, accumulate text for persistence, and forward as SSE.
	var accumulated strings.Builder
	for {
		ev, ok := reader.Next()
		if !ok {
			break
		}
		accumulated.WriteString(ev.Delta)

		chunk := inference.StreamToSSEChunk(ev, responseID)
		data, err := json.Marshal(chunk)
		if err != nil {
			s.logger.Warn("marshaling SSE chunk", zap.Error(err))
			break
		}
		if _, err := fmt.Fprintf(w, "data: %s\n\n", data); err != nil {
			s.logger.Warn("writing SSE data", zap.Error(err))
			break
		}
		flusher.Flush()
	}
	fmt.Fprint(w, "data: [DONE]\n\n")
	flusher.Flush()

	// Persist accumulated assistant response.
	if convID != "" && s.store != nil && accumulated.Len() > 0 {
		if err := s.store.AddMessage(ctx, convID, memory.ConversationMessage{
			Role:    "assistant",
			Content: accumulated.String(),
		}); err != nil {
			s.logger.Warn("persisting streaming assistant message failed", zap.Error(err))
		}
	}
}
