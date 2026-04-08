package inference

import (
	"context"
	"crypto/sha256"
	"fmt"
	"strings"
	"time"

	"go.uber.org/zap"
)

// ToolExecutor is a function that executes a named tool with JSON arguments
// and returns the result as a string.
type ToolExecutor func(ctx context.Context, name string, argsJSON string) (string, error)

// ToolLoopConfig configures the agentic tool-use loop.
type ToolLoopConfig struct {
	MaxIterations int
	MaxToolCalls  int
	Timeout       time.Duration
	Tools         []ToolSpec
	Execute       ToolExecutor
	Logger        *zap.Logger
}

// ToolStep records a single tool invocation and its result.
type ToolStep struct {
	Call   ToolCall
	Result string
}

// ToolLoopResult contains the final response and all intermediate tool steps.
type ToolLoopResult struct {
	Response *ChatResponse
	Steps    []ToolStep
}

// ToolLoopEvent is emitted during streaming tool loop execution.
type ToolLoopEvent struct {
	Type       string       // "tool_call", "tool_result", "stream", "done", "error"
	ToolCall   *ToolCall    `json:"tool_call,omitempty"`
	ToolResult *string      `json:"tool_result,omitempty"`
	Stream     *StreamEvent `json:"stream,omitempty"`
	Error      error        `json:"-"`
}

func applyToolLoopDefaults(cfg *ToolLoopConfig) {
	if cfg.MaxIterations <= 0 {
		cfg.MaxIterations = 5
	}
	if cfg.MaxToolCalls <= 0 {
		cfg.MaxToolCalls = 10
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 5 * time.Minute
	}
	if cfg.Logger == nil {
		cfg.Logger = zap.NewNop()
	}
}

func toolCallHash(name, args string) string {
	h := sha256.Sum256([]byte(name + ":" + args))
	return fmt.Sprintf("%x", h[:8])
}

// RunToolLoop executes an agentic tool-use loop: repeatedly calling the LLM
// and executing requested tools until the LLM produces a text response or
// limits are reached.
func RunToolLoop(ctx context.Context, provider Provider, req ChatRequest, cfg ToolLoopConfig) (*ToolLoopResult, error) {
	applyToolLoopDefaults(&cfg)

	ctx, cancel := context.WithTimeout(ctx, cfg.Timeout)
	defer cancel()

	req.Tools = cfg.Tools
	req.ToolChoice = "auto"

	var (
		iterations     int
		totalToolCalls int
		steps          []ToolStep
		seen           = make(map[string]int)
	)

	for {
		resp, err := provider.Chat(ctx, req)
		if err != nil {
			return nil, fmt.Errorf("tool loop chat iteration %d: %w", iterations, err)
		}

		// If no tool calls or finish reason is stop, we're done.
		if resp.FinishReason != "tool_calls" || len(resp.Message.ToolCalls) == 0 {
			return &ToolLoopResult{Response: resp, Steps: steps}, nil
		}

		// Append the assistant message (with tool_calls) to conversation.
		req.Messages = append(req.Messages, resp.Message)

		// Execute each tool call.
		for _, tc := range resp.Message.ToolCalls {
			hash := toolCallHash(tc.Function.Name, tc.Function.Arguments)
			seen[hash]++
			if seen[hash] > 2 {
				// Duplicate call detected — return error result instead of executing.
				cfg.Logger.Warn("duplicate tool call detected, skipping",
					zap.String("tool", tc.Function.Name))
				req.Messages = append(req.Messages, Message{
					Role:       "tool",
					Content:    "error: duplicate call detected, skipping",
					ToolCallID: tc.ID,
				})
				steps = append(steps, ToolStep{Call: tc, Result: "error: duplicate call detected"})
				continue
			}

			totalToolCalls++
			if totalToolCalls > cfg.MaxToolCalls {
				cfg.Logger.Warn("max tool calls reached, forcing text response",
					zap.Int("total", totalToolCalls))
				goto forceFinish
			}

			result, execErr := cfg.Execute(ctx, tc.Function.Name, tc.Function.Arguments)
			if execErr != nil {
				result = fmt.Sprintf("error: %v", execErr)
			}

			req.Messages = append(req.Messages, Message{
				Role:       "tool",
				Content:    result,
				ToolCallID: tc.ID,
			})
			steps = append(steps, ToolStep{Call: tc, Result: result})
		}

		iterations++
		if iterations >= cfg.MaxIterations {
			cfg.Logger.Warn("max iterations reached, forcing text response",
				zap.Int("iterations", iterations))
			goto forceFinish
		}

		continue

	forceFinish:
		// Force a final text response with ToolChoice="none".
		req.ToolChoice = "none"
		req.Tools = nil
		finalResp, err := provider.Chat(ctx, req)
		if err != nil {
			return nil, fmt.Errorf("tool loop force finish: %w", err)
		}
		return &ToolLoopResult{Response: finalResp, Steps: steps}, nil
	}
}

// RunToolLoopStream executes the tool loop with streaming for all iterations.
// Text deltas are forwarded to the client in real-time. Tool calls are detected
// from the stream and executed before the next iteration.
// Events are sent to the returned channel.
func RunToolLoopStream(ctx context.Context, provider Provider, req ChatRequest, cfg ToolLoopConfig) <-chan ToolLoopEvent {
	applyToolLoopDefaults(&cfg)

	ch := make(chan ToolLoopEvent, 64)

	go func() {
		defer close(ch)

		ctx, cancel := context.WithTimeout(ctx, cfg.Timeout)
		defer cancel()

		req.Tools = cfg.Tools
		req.ToolChoice = "auto"

		var (
			iterations     int
			totalToolCalls int
			seen           = make(map[string]int)
		)

		for {
			// Use streaming for every iteration to avoid non-streaming timeouts
			// on slow models. Streaming returns headers immediately, so the HTTP
			// connection stays alive while the model generates tokens.
			reader, err := provider.ChatStream(ctx, req)
			if err != nil {
				ch <- ToolLoopEvent{Type: "error", Error: err}
				return
			}

			var (
				content   strings.Builder
				toolCalls []ToolCall
				usage     *Usage
			)

			for {
				ev, ok := reader.Next()
				if !ok {
					break
				}

				content.WriteString(ev.Delta)
				toolCalls = append(toolCalls, ev.ToolCalls...)
				if ev.Usage != nil {
					usage = ev.Usage
				}

				// Forward text deltas to client immediately for true streaming.
				// Hold back the Done event until we've checked for tool calls.
				// Models producing tool calls typically emit no text content,
				// so this optimistic forwarding is safe in practice.
				if !ev.Done && ev.Delta != "" {
					fwd := StreamEvent{Delta: ev.Delta}
					ch <- ToolLoopEvent{Type: "stream", Stream: &fwd}
				}
			}

			if err := reader.Err(); err != nil {
				ch <- ToolLoopEvent{Type: "error", Error: err}
				return
			}

			// No tool calls — send final Done event and finish.
			if len(toolCalls) == 0 {
				done := StreamEvent{Done: true, Usage: usage}
				ch <- ToolLoopEvent{Type: "stream", Stream: &done}
				ch <- ToolLoopEvent{Type: "done"}
				return
			}

			// Tool calls detected — append assistant message and execute tools.
			req.Messages = append(req.Messages, Message{
				Role:      "assistant",
				Content:   content.String(),
				ToolCalls: toolCalls,
			})

			limitReached := false
			for _, tc := range toolCalls {
				ch <- ToolLoopEvent{Type: "tool_call", ToolCall: &tc}

				hash := toolCallHash(tc.Function.Name, tc.Function.Arguments)
				seen[hash]++
				if seen[hash] > 2 {
					errMsg := "error: duplicate call detected, skipping"
					ch <- ToolLoopEvent{Type: "tool_result", ToolResult: &errMsg}
					req.Messages = append(req.Messages, Message{
						Role:       "tool",
						Content:    errMsg,
						ToolCallID: tc.ID,
					})
					continue
				}

				totalToolCalls++
				if totalToolCalls > cfg.MaxToolCalls {
					limitReached = true
					break
				}

				result, execErr := cfg.Execute(ctx, tc.Function.Name, tc.Function.Arguments)
				if execErr != nil {
					result = fmt.Sprintf("error: %v", execErr)
				}

				ch <- ToolLoopEvent{Type: "tool_result", ToolResult: &result}
				req.Messages = append(req.Messages, Message{
					Role:       "tool",
					Content:    result,
					ToolCallID: tc.ID,
				})
			}

			iterations++
			if limitReached || iterations >= cfg.MaxIterations {
				req.ToolChoice = "none"
				req.Tools = nil
				streamFinalResponse(ctx, provider, req, ch, cfg)
				return
			}
		}
	}()

	return ch
}

func streamFinalResponse(ctx context.Context, provider Provider, req ChatRequest, ch chan<- ToolLoopEvent, cfg ToolLoopConfig) {
	// Use streaming for the final response.
	req.Stream = true
	reader, err := provider.ChatStream(ctx, req)
	if err != nil {
		ch <- ToolLoopEvent{Type: "error", Error: err}
		return
	}

	for {
		ev, ok := reader.Next()
		if !ok {
			break
		}
		ch <- ToolLoopEvent{Type: "stream", Stream: &ev}
	}

	if err := reader.Err(); err != nil {
		ch <- ToolLoopEvent{Type: "error", Error: err}
		return
	}

	ch <- ToolLoopEvent{Type: "done"}
}
