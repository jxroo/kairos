package inference

import "context"

// Provider abstracts an LLM inference backend.
type Provider interface {
	Name() string
	Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error)
	ChatStream(ctx context.Context, req ChatRequest) (*StreamReader, error)
	ListModels(ctx context.Context) ([]Model, error)
	Ping(ctx context.Context) error
}
