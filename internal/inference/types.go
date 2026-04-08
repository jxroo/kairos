package inference

// ChatRequest represents a request to an LLM provider.
type ChatRequest struct {
	Model       string     `json:"model"`
	Messages    []Message  `json:"messages"`
	Temperature float64    `json:"temperature,omitempty"`
	MaxTokens   int        `json:"max_tokens,omitempty"`
	Stream      bool       `json:"stream,omitempty"`
	Tools       []ToolSpec `json:"tools,omitempty"`
	ToolChoice  string     `json:"tool_choice,omitempty"` // "auto","none"
}

// Message represents a single conversation message.
type Message struct {
	Role       string     `json:"role"` // "system", "user", "assistant", "tool"
	Content    string     `json:"content"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"` // for role="tool"
}

// ToolCall represents a tool invocation requested by the LLM.
type ToolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"` // "function"
	Function FunctionCall `json:"function"`
}

// FunctionCall is the function name and arguments within a ToolCall.
type FunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"` // JSON string
}

// ToolSpec describes a tool available to the LLM.
type ToolSpec struct {
	Type     string           `json:"type"` // "function"
	Function ToolFunctionSpec `json:"function"`
}

// ToolFunctionSpec is the schema for a tool function.
type ToolFunctionSpec struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

// ChatResponse represents a non-streaming chat completion response.
type ChatResponse struct {
	ID           string  `json:"id"`
	Model        string  `json:"model"`
	Message      Message `json:"message"`
	Usage        Usage   `json:"usage"`
	FinishReason string  `json:"finish_reason,omitempty"` // "stop" or "tool_calls"
}

// Usage tracks token consumption for a response.
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// Model describes an available LLM model.
type Model struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Provider     string   `json:"provider"`
	ContextSize  int      `json:"context_length"`
	SizeBytes    int64    `json:"size_bytes,omitempty"`
	Capabilities []string `json:"capabilities,omitempty"`
}

// StreamEvent represents an incremental streaming chunk.
type StreamEvent struct {
	Delta     string     // incremental text
	Done      bool
	Usage     *Usage     // only on final event
	ToolCalls []ToolCall // streamed tool calls
}
