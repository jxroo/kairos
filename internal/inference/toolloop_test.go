package inference

import (
	"context"
	"fmt"
	"testing"
	"time"
)

// toolLoopMockProvider is a test Provider that returns scripted responses.
type toolLoopMockProvider struct {
	responses []*ChatResponse
	callCount int
}

func (m *toolLoopMockProvider) Name() string { return "mock" }

func (m *toolLoopMockProvider) Chat(_ context.Context, req ChatRequest) (*ChatResponse, error) {
	// When ToolChoice is "none", simulate forced text response.
	if req.ToolChoice == "none" {
		return &ChatResponse{
			Message:      Message{Role: "assistant", Content: "fallback response"},
			FinishReason: "stop",
		}, nil
	}
	if m.callCount >= len(m.responses) {
		return &ChatResponse{
			Message:      Message{Role: "assistant", Content: "fallback response"},
			FinishReason: "stop",
		}, nil
	}
	resp := m.responses[m.callCount]
	m.callCount++
	return resp, nil
}

func (m *toolLoopMockProvider) ChatStream(ctx context.Context, req ChatRequest) (*StreamReader, error) {
	ch := make(chan StreamEvent, 2)
	go func() {
		defer close(ch)
		resp, _ := m.Chat(ctx, req)
		ev := StreamEvent{Delta: resp.Message.Content, Done: true}
		if len(resp.Message.ToolCalls) > 0 {
			ev.ToolCalls = resp.Message.ToolCalls
		}
		ch <- ev
	}()
	return NewStreamReader(ch), nil
}

func (m *toolLoopMockProvider) ListModels(_ context.Context) ([]Model, error) {
	return []Model{{ID: "test-model"}}, nil
}

func (m *toolLoopMockProvider) Ping(_ context.Context) error { return nil }

func TestRunToolLoop(t *testing.T) {
	tests := []struct {
		name      string
		responses []*ChatResponse
		executor  ToolExecutor
		cfg       ToolLoopConfig
		wantSteps int
		wantText  string
		wantErr   bool
	}{
		{
			name: "no tool calls returns immediately",
			responses: []*ChatResponse{
				{Message: Message{Role: "assistant", Content: "Hello!"}, FinishReason: "stop"},
			},
			executor:  func(_ context.Context, _, _ string) (string, error) { return "", nil },
			wantSteps: 0,
			wantText:  "Hello!",
		},
		{
			name: "single tool call then text response",
			responses: []*ChatResponse{
				{
					Message: Message{
						Role: "assistant",
						ToolCalls: []ToolCall{
							{ID: "call_1", Type: "function", Function: FunctionCall{Name: "kairos_remember", Arguments: `{"content":"test"}`}},
						},
					},
					FinishReason: "tool_calls",
				},
				{Message: Message{Role: "assistant", Content: "Done!"}, FinishReason: "stop"},
			},
			executor: func(_ context.Context, name, _ string) (string, error) {
				return "Memory stored (id: abc)", nil
			},
			wantSteps: 1,
			wantText:  "Done!",
		},
		{
			name: "multiple tool calls in one response",
			responses: []*ChatResponse{
				{
					Message: Message{
						Role: "assistant",
						ToolCalls: []ToolCall{
							{ID: "call_1", Type: "function", Function: FunctionCall{Name: "kairos_recall", Arguments: `{"query":"deadline"}`}},
							{ID: "call_2", Type: "function", Function: FunctionCall{Name: "kairos_search_files", Arguments: `{"query":"readme"}`}},
						},
					},
					FinishReason: "tool_calls",
				},
				{Message: Message{Role: "assistant", Content: "Found it!"}, FinishReason: "stop"},
			},
			executor: func(_ context.Context, name, _ string) (string, error) {
				return "result for " + name, nil
			},
			wantSteps: 2,
			wantText:  "Found it!",
		},
		{
			name: "max iterations forces text response",
			responses: func() []*ChatResponse {
				// Always returns tool calls, forcing max iterations.
				out := make([]*ChatResponse, 10)
				for i := range out {
					out[i] = &ChatResponse{
						Message: Message{
							Role: "assistant",
							ToolCalls: []ToolCall{
								{ID: fmt.Sprintf("call_%d", i), Type: "function", Function: FunctionCall{Name: "kairos_recall", Arguments: fmt.Sprintf(`{"query":"q%d"}`, i)}},
							},
						},
						FinishReason: "tool_calls",
					}
				}
				return out
			}(),
			executor: func(_ context.Context, _, _ string) (string, error) {
				return "result", nil
			},
			cfg:       ToolLoopConfig{MaxIterations: 2},
			wantSteps: 2,
			wantText:  "fallback response",
		},
		{
			name: "max tool calls forces text response",
			responses: func() []*ChatResponse {
				out := make([]*ChatResponse, 10)
				for i := range out {
					calls := make([]ToolCall, 5)
					for j := range calls {
						calls[j] = ToolCall{
							ID:       fmt.Sprintf("call_%d_%d", i, j),
							Type:     "function",
							Function: FunctionCall{Name: "kairos_recall", Arguments: fmt.Sprintf(`{"query":"q%d_%d"}`, i, j)},
						}
					}
					out[i] = &ChatResponse{
						Message:      Message{Role: "assistant", ToolCalls: calls},
						FinishReason: "tool_calls",
					}
				}
				return out
			}(),
			executor: func(_ context.Context, _, _ string) (string, error) {
				return "result", nil
			},
			cfg:       ToolLoopConfig{MaxToolCalls: 3, MaxIterations: 10},
			wantSteps: 3,
			wantText:  "fallback response",
		},
		{
			name: "duplicate tool call detection",
			responses: []*ChatResponse{
				{
					Message: Message{
						Role: "assistant",
						ToolCalls: []ToolCall{
							{ID: "call_1", Type: "function", Function: FunctionCall{Name: "kairos_recall", Arguments: `{"query":"x"}`}},
						},
					},
					FinishReason: "tool_calls",
				},
				{
					Message: Message{
						Role: "assistant",
						ToolCalls: []ToolCall{
							{ID: "call_2", Type: "function", Function: FunctionCall{Name: "kairos_recall", Arguments: `{"query":"x"}`}},
						},
					},
					FinishReason: "tool_calls",
				},
				{
					Message: Message{
						Role: "assistant",
						ToolCalls: []ToolCall{
							{ID: "call_3", Type: "function", Function: FunctionCall{Name: "kairos_recall", Arguments: `{"query":"x"}`}},
						},
					},
					FinishReason: "tool_calls",
				},
				{Message: Message{Role: "assistant", Content: "Final"}, FinishReason: "stop"},
			},
			executor: func(_ context.Context, _, _ string) (string, error) {
				return "result", nil
			},
			wantSteps: 3,
			wantText:  "Final",
		},
		{
			name: "provider error during loop",
			responses: []*ChatResponse{
				{
					Message: Message{
						Role: "assistant",
						ToolCalls: []ToolCall{
							{ID: "call_1", Type: "function", Function: FunctionCall{Name: "kairos_recall", Arguments: `{"query":"x"}`}},
						},
					},
					FinishReason: "tool_calls",
				},
			},
			executor: func(_ context.Context, _, _ string) (string, error) {
				return "", fmt.Errorf("tool failed")
			},
			wantSteps: 1,
			wantText:  "fallback response",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider := &toolLoopMockProvider{responses: tt.responses}

			cfg := tt.cfg
			cfg.Tools = []ToolSpec{{Type: "function", Function: ToolFunctionSpec{Name: "test"}}}
			cfg.Execute = tt.executor
			if cfg.Timeout == 0 {
				cfg.Timeout = 5 * time.Second
			}

			result, err := RunToolLoop(context.Background(), provider, ChatRequest{Model: "test"}, cfg)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if len(result.Steps) != tt.wantSteps {
				t.Errorf("steps: got %d, want %d", len(result.Steps), tt.wantSteps)
			}

			if result.Response.Message.Content != tt.wantText {
				t.Errorf("response text: got %q, want %q", result.Response.Message.Content, tt.wantText)
			}
		})
	}
}

func TestRunToolLoop_TimeoutExceeded(t *testing.T) {
	provider := &toolLoopMockProvider{
		responses: []*ChatResponse{
			{
				Message: Message{
					Role: "assistant",
					ToolCalls: []ToolCall{
						{ID: "call_1", Type: "function", Function: FunctionCall{Name: "slow_tool", Arguments: `{}`}},
					},
				},
				FinishReason: "tool_calls",
			},
		},
	}

	cfg := ToolLoopConfig{
		Timeout: 50 * time.Millisecond,
		Tools:   []ToolSpec{{Type: "function", Function: ToolFunctionSpec{Name: "test"}}},
		Execute: func(ctx context.Context, _, _ string) (string, error) {
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-time.After(5 * time.Second):
				return "done", nil
			}
		},
	}

	result, err := RunToolLoop(context.Background(), provider, ChatRequest{Model: "test"}, cfg)
	// The tool itself should return ctx.Err, then the next Chat call will fail on ctx.Done.
	if err == nil && result != nil {
		// The tool error gets embedded as result text, not as a loop error.
		if len(result.Steps) > 0 && result.Steps[0].Result == "" {
			t.Error("expected tool error result for timeout")
		}
	}
}

func TestRunToolLoopStream(t *testing.T) {
	provider := &toolLoopMockProvider{
		responses: []*ChatResponse{
			{
				Message: Message{
					Role: "assistant",
					ToolCalls: []ToolCall{
						{ID: "call_1", Type: "function", Function: FunctionCall{Name: "kairos_remember", Arguments: `{"content":"hi"}`}},
					},
				},
				FinishReason: "tool_calls",
			},
			// Second call will use ChatStream, which uses Chat internally in our mock.
			{Message: Message{Role: "assistant", Content: "Remembered!"}, FinishReason: "stop"},
		},
	}

	cfg := ToolLoopConfig{
		Timeout: 5 * time.Second,
		Tools:   []ToolSpec{{Type: "function", Function: ToolFunctionSpec{Name: "test"}}},
		Execute: func(_ context.Context, _, _ string) (string, error) {
			return "stored", nil
		},
	}

	ch := RunToolLoopStream(context.Background(), provider, ChatRequest{Model: "test"}, cfg)

	var (
		gotToolCall   bool
		gotToolResult bool
		gotStream     bool
		gotDone       bool
	)

	for ev := range ch {
		switch ev.Type {
		case "tool_call":
			gotToolCall = true
		case "tool_result":
			gotToolResult = true
		case "stream":
			gotStream = true
		case "done":
			gotDone = true
		case "error":
			t.Fatalf("unexpected error: %v", ev.Error)
		}
	}

	if !gotToolCall {
		t.Error("expected tool_call event")
	}
	if !gotToolResult {
		t.Error("expected tool_result event")
	}
	if !gotStream {
		t.Error("expected stream event")
	}
	if !gotDone {
		t.Error("expected done event")
	}
}
