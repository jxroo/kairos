package inference

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"go.uber.org/zap"
)

// LlamaCppProvider implements Provider for a llama.cpp server
// exposing OpenAI-compatible endpoints.
type LlamaCppProvider struct {
	baseURL string
	client  *http.Client
	logger  *zap.Logger
}

// NewLlamaCppProvider constructs a LlamaCppProvider targeting the given base URL
// (e.g. "http://localhost:8080").
func NewLlamaCppProvider(url string, logger *zap.Logger) *LlamaCppProvider {
	return &LlamaCppProvider{
		baseURL: strings.TrimRight(url, "/"),
		client:  &http.Client{Timeout: 5 * time.Minute},
		logger:  logger,
	}
}

// Name returns the provider identifier.
func (p *LlamaCppProvider) Name() string { return "llamacpp" }

// Chat sends a non-streaming chat completion request to the llama.cpp server.
func (p *LlamaCppProvider) Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	req.Stream = false

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshaling chat request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		p.baseURL+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("creating HTTP request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("sending chat request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("llamacpp chat: unexpected status %d", resp.StatusCode)
	}

	var raw llamaCppChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("decoding chat response: %w", err)
	}

	if len(raw.Choices) == 0 {
		return nil, fmt.Errorf("llamacpp chat: response contained no choices")
	}

	finishReason := raw.Choices[0].FinishReason
	if finishReason == "" {
		finishReason = "stop"
	}

	return &ChatResponse{
		ID:           raw.ID,
		Model:        raw.Model,
		Message:      raw.Choices[0].Message,
		FinishReason: finishReason,
		Usage:        raw.Usage,
	}, nil
}

// ChatStream sends a streaming chat completion request and returns a StreamReader
// that yields incremental events over a goroutine.
func (p *LlamaCppProvider) ChatStream(ctx context.Context, req ChatRequest) (*StreamReader, error) {
	req.Stream = true

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshaling stream request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		p.baseURL+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("creating HTTP request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("sending stream request: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("llamacpp stream: unexpected status %d", resp.StatusCode)
	}

	ch := make(chan StreamEvent, 32)
	reader := NewStreamReader(ch)

	go func() {
		defer resp.Body.Close()
		defer close(ch)

		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			line := scanner.Text()

			// SSE lines start with "data: "
			if !strings.HasPrefix(line, "data: ") {
				continue
			}
			payload := strings.TrimPrefix(line, "data: ")

			if payload == "[DONE]" {
				return
			}

			var chunk SSEChunk
			if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
				reader.setErr(fmt.Errorf("decoding SSE chunk: %w", err))
				return
			}

			if len(chunk.Choices) == 0 {
				continue
			}

			choice := chunk.Choices[0]
			ev := StreamEvent{
				Delta:     choice.Delta.Content,
				ToolCalls: choice.Delta.ToolCalls,
			}

			if choice.FinishReason != nil && *choice.FinishReason != "" {
				ev.Done = true
				ev.Usage = chunk.Usage
			}

			select {
			case ch <- ev:
			case <-ctx.Done():
				reader.setErr(fmt.Errorf("stream context canceled: %w", ctx.Err()))
				return
			}
		}

		if err := scanner.Err(); err != nil {
			reader.setErr(fmt.Errorf("reading SSE stream: %w", err))
		}
	}()

	return reader, nil
}

// ListModels queries the llama.cpp server for available models.
func (p *LlamaCppProvider) ListModels(ctx context.Context) ([]Model, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet,
		p.baseURL+"/v1/models", nil)
	if err != nil {
		return nil, fmt.Errorf("creating models request: %w", err)
	}

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("listing models: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("llamacpp list models: unexpected status %d", resp.StatusCode)
	}

	var raw llamaCppModelsResponse
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("decoding models response: %w", err)
	}

	models := make([]Model, 0, len(raw.Data))
	for _, m := range raw.Data {
		models = append(models, Model{
			ID:           m.ID,
			Name:         m.ID,
			Provider:     p.Name(),
			Capabilities: []string{"chat"},
		})
	}
	return models, nil
}

// Ping checks whether the llama.cpp server is reachable by listing models
// with a short timeout.
func (p *LlamaCppProvider) Ping(ctx context.Context) error {
	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	_, err := p.ListModels(pingCtx)
	if err != nil {
		return fmt.Errorf("ping llamacpp: %w", err)
	}
	return nil
}

// ---- internal wire types ------------------------------------------------

// llamaCppChatResponse mirrors the OpenAI chat.completion JSON structure.
type llamaCppChatResponse struct {
	ID      string           `json:"id"`
	Object  string           `json:"object"`
	Model   string           `json:"model"`
	Choices []llamaCppChoice `json:"choices"`
	Usage   Usage            `json:"usage"`
}

type llamaCppChoice struct {
	Index        int     `json:"index"`
	Message      Message `json:"message"`
	FinishReason string  `json:"finish_reason"`
}

// llamaCppModelsResponse mirrors the OpenAI list-models JSON structure.
type llamaCppModelsResponse struct {
	Object string          `json:"object"`
	Data   []llamaCppModel `json:"data"`
}

type llamaCppModel struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	OwnedBy string `json:"owned_by"`
}

// Ensure LlamaCppProvider satisfies the Provider interface at compile time.
var _ Provider = (*LlamaCppProvider)(nil)
