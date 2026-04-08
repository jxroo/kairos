package inference

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
)

// StreamReader reads streaming events from a provider response.
type StreamReader struct {
	events <-chan StreamEvent
	err    error
	mu     sync.Mutex
}

// NewStreamReader creates a StreamReader from an event channel.
func NewStreamReader(events <-chan StreamEvent) *StreamReader {
	return &StreamReader{events: events}
}

// Next returns the next streaming event. ok is false when the stream is exhausted.
func (r *StreamReader) Next() (StreamEvent, bool) {
	ev, ok := <-r.events
	if !ok {
		return StreamEvent{}, false
	}
	return ev, true
}

// Err returns any error encountered during streaming.
func (r *StreamReader) Err() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.err
}

// setErr records an error (thread-safe).
func (r *StreamReader) setErr(err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.err = err
}

// SSEChunk is the OpenAI-format SSE data payload.
type SSEChunk struct {
	ID      string      `json:"id"`
	Object  string      `json:"object"`
	Choices []SSEChoice `json:"choices"`
	Usage   *Usage      `json:"usage,omitempty"`
}

// SSEChoice is a single choice within an SSE chunk.
type SSEChoice struct {
	Index        int        `json:"index"`
	Delta        SSEDelta   `json:"delta"`
	FinishReason *string    `json:"finish_reason"`
}

// SSEDelta is the incremental content within a streaming choice.
type SSEDelta struct {
	Role      string     `json:"role,omitempty"`
	Content   string     `json:"content,omitempty"`
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`
}

// StreamToSSEChunk converts a StreamEvent into an OpenAI-format SSE chunk.
func StreamToSSEChunk(ev StreamEvent, responseID string) SSEChunk {
	chunk := SSEChunk{
		ID:     responseID,
		Object: "chat.completion.chunk",
		Choices: []SSEChoice{
			{
				Index: 0,
				Delta: SSEDelta{
					Content:   ev.Delta,
					ToolCalls: ev.ToolCalls,
				},
			},
		},
	}
	if ev.Done {
		finishReason := "stop"
		if len(ev.ToolCalls) > 0 {
			finishReason = "tool_calls"
		}
		chunk.Choices[0].FinishReason = &finishReason
		chunk.Usage = ev.Usage
	}
	return chunk
}

// WriteSSE writes OpenAI-format SSE events to an http.ResponseWriter.
// Each event is: "data: {json}\n\n"
// Final event is: "data: [DONE]\n\n"
func WriteSSE(w http.ResponseWriter, flusher http.Flusher, reader *StreamReader, responseID, model string) error {
	for {
		ev, ok := reader.Next()
		if !ok {
			break
		}

		chunk := StreamToSSEChunk(ev, responseID)

		data, err := json.Marshal(chunk)
		if err != nil {
			return fmt.Errorf("marshaling SSE chunk: %w", err)
		}

		if _, err := fmt.Fprintf(w, "data: %s\n\n", data); err != nil {
			return fmt.Errorf("writing SSE data: %w", err)
		}
		flusher.Flush()
	}

	if _, err := fmt.Fprint(w, "data: [DONE]\n\n"); err != nil {
		return fmt.Errorf("writing SSE done: %w", err)
	}
	flusher.Flush()

	return reader.Err()
}
