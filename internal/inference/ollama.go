package inference

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"go.uber.org/zap"
)

const (
	ollamaDefaultURL      = "http://localhost:11434"
	ollamaDefaultContext  = 4096
	ollamaDiscoverTimeout = 2 * time.Second
	ollamaPingTimeout     = 5 * time.Second
)

// OllamaProvider implements Provider using the Ollama HTTP API.
type OllamaProvider struct {
	url        string
	client     *http.Client // short-timeout client for ping/list
	chatClient *http.Client // no global timeout for inference — ctx controls deadline
	logger     *zap.Logger
}

// NewOllamaProvider creates an OllamaProvider targeting the given URL.
// If url is empty, the Ollama default (localhost:11434) is used.
func NewOllamaProvider(url string, logger *zap.Logger) *OllamaProvider {
	if url == "" {
		url = ollamaDefaultURL
	}
	return &OllamaProvider{
		url:        url,
		client:     &http.Client{Timeout: 120 * time.Second},
		chatClient: &http.Client{}, // no global timeout; callers pass context with deadline
		logger:     logger,
	}
}

// Name returns the provider name.
func (p *OllamaProvider) Name() string {
	return "ollama"
}

// Ping checks whether Ollama is reachable by calling GET /api/tags with a short timeout.
func (p *OllamaProvider) Ping(ctx context.Context) error {
	pingCtx, cancel := context.WithTimeout(ctx, ollamaPingTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(pingCtx, http.MethodGet, p.url+"/api/tags", nil)
	if err != nil {
		return fmt.Errorf("creating ping request: %w", err)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return fmt.Errorf("pinging ollama: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("ping: unexpected status %d", resp.StatusCode)
	}
	return nil
}

// Discover probes the configured URL (and optionally the default URL) to verify
// Ollama is reachable. It returns the reachable URL or an error.
func (p *OllamaProvider) Discover(ctx context.Context, autoDiscover bool) (string, error) {
	candidates := []string{p.url}
	if autoDiscover {
		candidates = discoveryCandidates(p.url)
	}

	for _, candidate := range candidates {
		discCtx, cancel := context.WithTimeout(ctx, ollamaDiscoverTimeout)
		err := p.probeURL(discCtx, candidate)
		cancel()
		if err == nil {
			return candidate, nil
		}
		if candidate != p.url {
			p.logger.Debug("ollama discovery candidate unreachable",
				zap.String("candidate", candidate),
				zap.Error(err),
			)
		}
	}

	return "", fmt.Errorf("ollama not reachable at %s", p.url)
}

func discoveryCandidates(configured string) []string {
	candidates := []string{configured, ollamaDefaultURL}
	if parsed, err := url.Parse(ollamaDefaultURL); err == nil {
		parsed.Host = "127.0.0.1:11434"
		candidates = append(candidates, parsed.String())
		parsed.Host = "[::1]:11434"
		candidates = append(candidates, parsed.String())
		parsed.Host = "ollama.local:11434"
		candidates = append(candidates, parsed.String())
	}

	seen := make(map[string]struct{}, len(candidates))
	out := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		candidate = strings.TrimRight(candidate, "/")
		if candidate == "" {
			continue
		}
		if _, ok := seen[candidate]; ok {
			continue
		}
		seen[candidate] = struct{}{}
		out = append(out, candidate)
	}
	return out
}

func (p *OllamaProvider) probeURL(ctx context.Context, url string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url+"/api/tags", nil)
	if err != nil {
		return fmt.Errorf("creating probe request: %w", err)
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return fmt.Errorf("probing %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("probe: unexpected status %d", resp.StatusCode)
	}
	return nil
}

// ollamaTagsResponse is the /api/tags response shape.
type ollamaTagsResponse struct {
	Models []ollamaModelEntry `json:"models"`
}

type ollamaModelEntry struct {
	Name      string            `json:"name"`
	Size      int64             `json:"size"`
	Details   ollamaModelDetail `json:"details"`
	ModelInfo map[string]any    `json:"model_info"`
}

type ollamaModelDetail struct {
	ParameterSize string `json:"parameter_size"`
}

// ListModels returns all models available in the connected Ollama instance.
func (p *OllamaProvider) ListModels(ctx context.Context) ([]Model, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.url+"/api/tags", nil)
	if err != nil {
		return nil, fmt.Errorf("creating list models request: %w", err)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("listing models: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("list models: status %d: %s", resp.StatusCode, string(body))
	}

	var tags ollamaTagsResponse
	if err := json.NewDecoder(resp.Body).Decode(&tags); err != nil {
		return nil, fmt.Errorf("decoding list models response: %w", err)
	}

	models := make([]Model, 0, len(tags.Models))
	for _, m := range tags.Models {
		models = append(models, Model{
			ID:           m.Name,
			Name:         m.Name,
			Provider:     "ollama",
			ContextSize:  extractContextSize(m.ModelInfo),
			SizeBytes:    m.Size,
			Capabilities: detectOllamaCapabilities(m),
		})
	}
	return models, nil
}

func detectOllamaCapabilities(entry ollamaModelEntry) []string {
	caps := []string{"chat"}
	name := strings.ToLower(entry.Name)
	arch := strings.ToLower(stringValue(entry.ModelInfo["general.architecture"]))
	projector := strings.ToLower(stringValue(entry.ModelInfo["projector.type"]))

	if strings.Contains(name, "vision") || strings.Contains(name, "llava") ||
		strings.Contains(arch, "llava") || projector != "" {
		caps = append(caps, "vision")
	}

	return caps
}

func stringValue(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprint(v)
}

// extractContextSize reads "general.context_length" from the model_info map.
// Falls back to ollamaDefaultContext if not present or unparseable.
func extractContextSize(info map[string]any) int {
	if info == nil {
		return ollamaDefaultContext
	}
	v, ok := info["general.context_length"]
	if !ok {
		return ollamaDefaultContext
	}
	switch val := v.(type) {
	case float64:
		if val > 0 {
			return int(val)
		}
	case int:
		if val > 0 {
			return val
		}
	case int64:
		if val > 0 {
			return int(val)
		}
	}
	return ollamaDefaultContext
}

// ollamaChatRequest is the body sent to POST /api/chat.
type ollamaChatRequest struct {
	Model    string          `json:"model"`
	Messages []ollamaMessage `json:"messages"`
	Stream   bool            `json:"stream"`
	Options  ollamaOptions   `json:"options,omitempty"`
	Tools    []ToolSpec      `json:"tools,omitempty"`
}

type ollamaMessage struct {
	Role      string           `json:"role"`
	Content   string           `json:"content"`
	ToolCalls []ollamaToolCall `json:"tool_calls,omitempty"`
}

type ollamaToolCall struct {
	Function ollamaFunctionCall `json:"function"`
}

type ollamaFunctionCall struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
}

type ollamaOptions struct {
	Temperature float64 `json:"temperature,omitempty"`
	NumPredict  int     `json:"num_predict,omitempty"`
}

// ollamaChatResponse is returned by Ollama for non-streaming requests,
// and is also the shape of each streaming JSON line.
type ollamaChatResponse struct {
	Model           string        `json:"model"`
	Message         ollamaMessage `json:"message"`
	Done            bool          `json:"done"`
	EvalCount       int           `json:"eval_count"`
	PromptEvalCount int           `json:"prompt_eval_count"`
	TotalDuration   int64         `json:"total_duration"`
}

func toOllamaMessages(msgs []Message) []ollamaMessage {
	out := make([]ollamaMessage, 0, len(msgs))
	for _, m := range msgs {
		om := ollamaMessage{Role: m.Role, Content: m.Content}
		// Convert tool calls from our format to Ollama format.
		for _, tc := range m.ToolCalls {
			var args map[string]any
			_ = json.Unmarshal([]byte(tc.Function.Arguments), &args)
			om.ToolCalls = append(om.ToolCalls, ollamaToolCall{
				Function: ollamaFunctionCall{
					Name:      tc.Function.Name,
					Arguments: args,
				},
			})
		}
		out = append(out, om)
	}
	return out
}

// Chat performs a single (non-streaming) chat completion via POST /api/chat.
func (p *OllamaProvider) Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	body := ollamaChatRequest{
		Model:    req.Model,
		Messages: toOllamaMessages(req.Messages),
		Stream:   false,
		Tools:    req.Tools,
		Options: ollamaOptions{
			Temperature: req.Temperature,
			NumPredict:  req.MaxTokens,
		},
	}

	data, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshaling chat request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.url+"/api/chat", bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("creating chat request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := p.chatClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("chat request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		errBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("chat: status %d: %s", resp.StatusCode, string(errBody))
	}

	var ollamaResp ollamaChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&ollamaResp); err != nil {
		return nil, fmt.Errorf("decoding chat response: %w", err)
	}

	msg := Message{
		Role:    ollamaResp.Message.Role,
		Content: ollamaResp.Message.Content,
	}

	finishReason := "stop"
	if len(ollamaResp.Message.ToolCalls) > 0 {
		finishReason = "tool_calls"
		msg.ToolCalls = ollamaToolCallsToGeneric(ollamaResp.Message.ToolCalls)
	}

	total := ollamaResp.EvalCount + ollamaResp.PromptEvalCount
	return &ChatResponse{
		Model:        ollamaResp.Model,
		Message:      msg,
		FinishReason: finishReason,
		Usage: Usage{
			PromptTokens:     ollamaResp.PromptEvalCount,
			CompletionTokens: ollamaResp.EvalCount,
			TotalTokens:      total,
		},
	}, nil
}

// ollamaToolCallsToGeneric converts Ollama tool calls to the generic format.
func ollamaToolCallsToGeneric(calls []ollamaToolCall) []ToolCall {
	out := make([]ToolCall, len(calls))
	for i, c := range calls {
		argsJSON, _ := json.Marshal(c.Function.Arguments)
		out[i] = ToolCall{
			ID:   fmt.Sprintf("call_%d", i),
			Type: "function",
			Function: FunctionCall{
				Name:      c.Function.Name,
				Arguments: string(argsJSON),
			},
		}
	}
	return out
}

// ChatStream performs a streaming chat completion via POST /api/chat with stream=true.
// It returns a *StreamReader whose channel is populated by a background goroutine.
func (p *OllamaProvider) ChatStream(ctx context.Context, req ChatRequest) (*StreamReader, error) {
	body := ollamaChatRequest{
		Model:    req.Model,
		Messages: toOllamaMessages(req.Messages),
		Stream:   true,
		Tools:    req.Tools,
		Options: ollamaOptions{
			Temperature: req.Temperature,
			NumPredict:  req.MaxTokens,
		},
	}

	data, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshaling stream request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.url+"/api/chat", bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("creating stream request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := p.chatClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("stream request: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("stream: status %d", resp.StatusCode)
	}

	events := make(chan StreamEvent, 64)
	reader := NewStreamReader(events)

	go func() {
		defer resp.Body.Close()
		defer close(events)

		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			line := scanner.Bytes()
			if len(line) == 0 {
				continue
			}

			var chunk ollamaChatResponse
			if err := json.Unmarshal(line, &chunk); err != nil {
				reader.setErr(fmt.Errorf("decoding stream chunk: %w", err))
				return
			}

			ev := StreamEvent{
				Delta: chunk.Message.Content,
				Done:  chunk.Done,
			}
			if len(chunk.Message.ToolCalls) > 0 {
				ev.ToolCalls = ollamaToolCallsToGeneric(chunk.Message.ToolCalls)
			}
			if chunk.Done {
				total := chunk.EvalCount + chunk.PromptEvalCount
				ev.Usage = &Usage{
					PromptTokens:     chunk.PromptEvalCount,
					CompletionTokens: chunk.EvalCount,
					TotalTokens:      total,
				}
			}

			select {
			case events <- ev:
			case <-ctx.Done():
				reader.setErr(fmt.Errorf("stream context canceled: %w", ctx.Err()))
				return
			}

			if chunk.Done {
				return
			}
		}

		if err := scanner.Err(); err != nil {
			reader.setErr(fmt.Errorf("reading stream: %w", err))
		}
	}()

	return reader, nil
}
