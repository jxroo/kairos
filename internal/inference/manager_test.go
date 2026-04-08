package inference

import (
	"context"
	"errors"
	"testing"
)

// mockProvider is a test double that implements Provider.
type mockProvider struct {
	name     string
	models   []Model
	chatFn   func(ctx context.Context, req ChatRequest) (*ChatResponse, error)
	streamFn func(ctx context.Context, req ChatRequest) (*StreamReader, error)
	pingErr  error
}

func (m *mockProvider) Name() string { return m.name }

func (m *mockProvider) Ping(_ context.Context) error { return m.pingErr }

func (m *mockProvider) ListModels(_ context.Context) ([]Model, error) {
	return m.models, nil
}

func (m *mockProvider) Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	if m.chatFn != nil {
		return m.chatFn(ctx, req)
	}
	return &ChatResponse{Model: req.Model, Message: Message{Role: "assistant", Content: "ok"}}, nil
}

func (m *mockProvider) ChatStream(ctx context.Context, req ChatRequest) (*StreamReader, error) {
	if m.streamFn != nil {
		return m.streamFn(ctx, req)
	}
	ch := make(chan StreamEvent, 1)
	ch <- StreamEvent{Delta: "chunk", Done: true}
	close(ch)
	return NewStreamReader(ch), nil
}

// --------------------------------------------------------------------------
// Test: ListModels aggregates across all providers
// --------------------------------------------------------------------------

func TestManagerListModels(t *testing.T) {
	tests := []struct {
		name       string
		providers  []*mockProvider
		wantModels []string // IDs expected
	}{
		{
			name: "single provider",
			providers: []*mockProvider{
				{name: "p1", models: []Model{{ID: "m1"}, {ID: "m2"}}},
			},
			wantModels: []string{"m1", "m2"},
		},
		{
			name: "multiple providers",
			providers: []*mockProvider{
				{name: "p1", models: []Model{{ID: "m1"}}},
				{name: "p2", models: []Model{{ID: "m2"}, {ID: "m3"}}},
			},
			wantModels: []string{"m1", "m2", "m3"},
		},
		{
			name:       "no providers",
			providers:  nil,
			wantModels: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mgr := NewManager(newTestLogger())
			for _, p := range tt.providers {
				mgr.Register(p)
			}

			models, err := mgr.ListModels(context.Background())
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if len(models) != len(tt.wantModels) {
				t.Fatalf("got %d models, want %d", len(models), len(tt.wantModels))
			}
			got := make(map[string]bool)
			for _, m := range models {
				got[m.ID] = true
			}
			for _, id := range tt.wantModels {
				if !got[id] {
					t.Errorf("expected model %q not in result", id)
				}
			}
		})
	}
}

// --------------------------------------------------------------------------
// Test: Chat routes to the correct provider based on model name
// --------------------------------------------------------------------------

func TestManagerChatRoutesCorrectProvider(t *testing.T) {
	called := ""

	p1 := &mockProvider{
		name:   "p1",
		models: []Model{{ID: "model-a"}},
		chatFn: func(_ context.Context, req ChatRequest) (*ChatResponse, error) {
			called = "p1"
			return &ChatResponse{Model: req.Model}, nil
		},
	}
	p2 := &mockProvider{
		name:   "p2",
		models: []Model{{ID: "model-b"}},
		chatFn: func(_ context.Context, req ChatRequest) (*ChatResponse, error) {
			called = "p2"
			return &ChatResponse{Model: req.Model}, nil
		},
	}

	tests := []struct {
		model        string
		wantProvider string
	}{
		{"model-a", "p1"},
		{"model-b", "p2"},
	}

	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			called = ""
			mgr := NewManager(newTestLogger())
			mgr.Register(p1)
			mgr.Register(p2)

			_, err := mgr.Chat(context.Background(), ChatRequest{Model: tt.model})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if called != tt.wantProvider {
				t.Errorf("routed to %q, want %q", called, tt.wantProvider)
			}
		})
	}
}

// --------------------------------------------------------------------------
// Test: Empty model string uses first available model
// --------------------------------------------------------------------------

func TestManagerChatEmptyModelUsesFirst(t *testing.T) {
	called := ""
	resolvedModel := ""

	p1 := &mockProvider{
		name:   "p1",
		models: []Model{{ID: "first-model"}},
		chatFn: func(_ context.Context, req ChatRequest) (*ChatResponse, error) {
			called = "p1"
			resolvedModel = req.Model
			return &ChatResponse{Model: req.Model}, nil
		},
	}

	mgr := NewManager(newTestLogger())
	mgr.Register(p1)

	_, err := mgr.Chat(context.Background(), ChatRequest{Model: ""})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if called != "p1" {
		t.Errorf("expected p1 to be called, got %q", called)
	}
	if resolvedModel != "first-model" {
		t.Errorf("resolved model = %q, want first-model", resolvedModel)
	}
}

func TestManagerChatEmptyModelUsesConfiguredDefault(t *testing.T) {
	called := ""
	resolvedModel := ""

	p1 := &mockProvider{
		name:   "p1",
		models: []Model{{ID: "fallback-model"}},
		chatFn: func(_ context.Context, req ChatRequest) (*ChatResponse, error) {
			called = "p1"
			resolvedModel = req.Model
			return &ChatResponse{Model: req.Model}, nil
		},
	}
	p2 := &mockProvider{
		name:   "p2",
		models: []Model{{ID: "preferred-model"}},
		chatFn: func(_ context.Context, req ChatRequest) (*ChatResponse, error) {
			called = "p2"
			resolvedModel = req.Model
			return &ChatResponse{Model: req.Model}, nil
		},
	}

	mgr := NewManager(newTestLogger())
	mgr.Register(p1)
	mgr.Register(p2)
	mgr.SetDefaultModel("preferred-model")

	if _, err := mgr.Chat(context.Background(), ChatRequest{}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if called != "p2" {
		t.Errorf("called provider = %q, want p2", called)
	}
	if resolvedModel != "preferred-model" {
		t.Errorf("resolved model = %q, want preferred-model", resolvedModel)
	}
}

// --------------------------------------------------------------------------
// Test: No providers returns ErrNoProviders
// --------------------------------------------------------------------------

func TestManagerNoProvidersReturnsError(t *testing.T) {
	mgr := NewManager(newTestLogger())

	t.Run("Chat", func(t *testing.T) {
		_, err := mgr.Chat(context.Background(), ChatRequest{Model: "any"})
		if !errors.Is(err, ErrNoProviders) {
			t.Errorf("expected ErrNoProviders, got %v", err)
		}
	})

	t.Run("ChatStream", func(t *testing.T) {
		_, err := mgr.ChatStream(context.Background(), ChatRequest{Model: "any"})
		if !errors.Is(err, ErrNoProviders) {
			t.Errorf("expected ErrNoProviders, got %v", err)
		}
	})

	t.Run("ResolveModel", func(t *testing.T) {
		_, err := mgr.ResolveModel(context.Background(), "any")
		if !errors.Is(err, ErrNoProviders) {
			t.Errorf("expected ErrNoProviders, got %v", err)
		}
	})
}

// --------------------------------------------------------------------------
// Test: Unknown model returns ErrModelNotFound
// --------------------------------------------------------------------------

func TestManagerUnknownModelReturnsError(t *testing.T) {
	p := &mockProvider{
		name:   "p1",
		models: []Model{{ID: "known-model"}},
	}

	mgr := NewManager(newTestLogger())
	mgr.Register(p)

	t.Run("Chat", func(t *testing.T) {
		_, err := mgr.Chat(context.Background(), ChatRequest{Model: "unknown-model"})
		if !errors.Is(err, ErrModelNotFound) {
			t.Errorf("expected ErrModelNotFound, got %v", err)
		}
	})

	t.Run("ChatStream", func(t *testing.T) {
		_, err := mgr.ChatStream(context.Background(), ChatRequest{Model: "unknown-model"})
		if !errors.Is(err, ErrModelNotFound) {
			t.Errorf("expected ErrModelNotFound, got %v", err)
		}
	})

	t.Run("ResolveModel", func(t *testing.T) {
		_, err := mgr.ResolveModel(context.Background(), "unknown-model")
		if !errors.Is(err, ErrModelNotFound) {
			t.Errorf("expected ErrModelNotFound, got %v", err)
		}
	})
}

// --------------------------------------------------------------------------
// Test: ChatStream routes to correct provider
// --------------------------------------------------------------------------

func TestManagerChatStreamRoutes(t *testing.T) {
	called := ""

	makeStream := func(provider string) func(ctx context.Context, req ChatRequest) (*StreamReader, error) {
		return func(_ context.Context, _ ChatRequest) (*StreamReader, error) {
			called = provider
			ch := make(chan StreamEvent, 1)
			ch <- StreamEvent{Delta: "hello", Done: true}
			close(ch)
			return NewStreamReader(ch), nil
		}
	}

	p1 := &mockProvider{
		name:     "p1",
		models:   []Model{{ID: "stream-model-a"}},
		streamFn: makeStream("p1"),
	}
	p2 := &mockProvider{
		name:     "p2",
		models:   []Model{{ID: "stream-model-b"}},
		streamFn: makeStream("p2"),
	}

	tests := []struct {
		model        string
		wantProvider string
	}{
		{"stream-model-a", "p1"},
		{"stream-model-b", "p2"},
	}

	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			called = ""
			mgr := NewManager(newTestLogger())
			mgr.Register(p1)
			mgr.Register(p2)

			reader, err := mgr.ChatStream(context.Background(), ChatRequest{Model: tt.model})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			// Drain the stream.
			for {
				_, ok := reader.Next()
				if !ok {
					break
				}
			}
			if called != tt.wantProvider {
				t.Errorf("routed to %q, want %q", called, tt.wantProvider)
			}
		})
	}
}

// --------------------------------------------------------------------------
// Test: Register and ResolveModel
// --------------------------------------------------------------------------

func TestManagerRegisterAndResolveModel(t *testing.T) {
	mgr := NewManager(newTestLogger())

	p := &mockProvider{
		name:   "p1",
		models: []Model{{ID: "my-model", Name: "My Model"}},
	}
	mgr.Register(p)

	t.Run("resolve by ID", func(t *testing.T) {
		got, err := mgr.ResolveModel(context.Background(), "my-model")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.Name() != "p1" {
			t.Errorf("resolved to %q, want p1", got.Name())
		}
	})

	t.Run("resolve by Name", func(t *testing.T) {
		got, err := mgr.ResolveModel(context.Background(), "My Model")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.Name() != "p1" {
			t.Errorf("resolved to %q, want p1", got.Name())
		}
	})

	t.Run("resolve canonical model info", func(t *testing.T) {
		_, mdl, err := mgr.ResolveModelInfo(context.Background(), "My Model")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if mdl.ID != "my-model" {
			t.Errorf("model ID = %q, want my-model", mdl.ID)
		}
	})
}

// --------------------------------------------------------------------------
// Test: Discover skips providers with ping errors
// --------------------------------------------------------------------------

func TestManagerDiscoverNilConfigErrors(t *testing.T) {
	mgr := NewManager(newTestLogger())
	err := mgr.Discover(context.Background(), nil)
	if err == nil {
		t.Error("expected error for nil config, got nil")
	}
}

func TestManagerEmptyModelNoModelsAvailable(t *testing.T) {
	// Provider with zero models — empty string model should return ErrModelNotFound.
	p := &mockProvider{
		name:   "empty-provider",
		models: []Model{},
	}
	mgr := NewManager(newTestLogger())
	mgr.Register(p)

	_, err := mgr.ResolveModel(context.Background(), "")
	if !errors.Is(err, ErrModelNotFound) {
		t.Errorf("expected ErrModelNotFound, got %v", err)
	}
}
