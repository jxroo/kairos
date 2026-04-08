package tools

import (
	"context"
	"errors"
	"sync"
	"testing"
)

func dummyHandler(_ context.Context, _ map[string]any) (*ToolResult, error) {
	return &ToolResult{Content: "ok"}, nil
}

func TestRegistry_RegisterAndGet(t *testing.T) {
	r := NewRegistry()
	def := ToolDefinition{Name: "test-tool", Description: "A test tool"}

	if err := r.Register(def, dummyHandler); err != nil {
		t.Fatalf("Register: %v", err)
	}

	got, handler, err := r.Get("test-tool")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Name != "test-tool" {
		t.Errorf("got name %q, want %q", got.Name, "test-tool")
	}
	res, _ := handler(context.Background(), nil)
	if res.Content != "ok" {
		t.Errorf("handler returned %q, want %q", res.Content, "ok")
	}
}

func TestRegistry_DuplicateReturnsErrToolExists(t *testing.T) {
	r := NewRegistry()
	def := ToolDefinition{Name: "dup"}
	_ = r.Register(def, dummyHandler)

	err := r.Register(def, dummyHandler)
	if !errors.Is(err, ErrToolExists) {
		t.Errorf("got %v, want ErrToolExists", err)
	}
}

func TestRegistry_UnknownReturnsErrToolNotFound(t *testing.T) {
	r := NewRegistry()
	_, _, err := r.Get("nonexistent")
	if !errors.Is(err, ErrToolNotFound) {
		t.Errorf("got %v, want ErrToolNotFound", err)
	}
}

func TestRegistry_List(t *testing.T) {
	r := NewRegistry()
	_ = r.Register(ToolDefinition{Name: "a"}, dummyHandler)
	_ = r.Register(ToolDefinition{Name: "b"}, dummyHandler)

	defs := r.List()
	if len(defs) != 2 {
		t.Fatalf("List returned %d items, want 2", len(defs))
	}

	names := map[string]bool{}
	for _, d := range defs {
		names[d.Name] = true
	}
	if !names["a"] || !names["b"] {
		t.Errorf("List missing expected tools: %v", names)
	}
}

func TestRegistry_Unregister(t *testing.T) {
	r := NewRegistry()
	_ = r.Register(ToolDefinition{Name: "rm"}, dummyHandler)

	if err := r.Unregister("rm"); err != nil {
		t.Fatalf("Unregister: %v", err)
	}
	_, _, err := r.Get("rm")
	if !errors.Is(err, ErrToolNotFound) {
		t.Errorf("after Unregister: got %v, want ErrToolNotFound", err)
	}
}

func TestRegistry_UnregisterNotFound(t *testing.T) {
	r := NewRegistry()
	err := r.Unregister("ghost")
	if !errors.Is(err, ErrToolNotFound) {
		t.Errorf("got %v, want ErrToolNotFound", err)
	}
}

func TestRegistry_ConcurrentAccess(t *testing.T) {
	r := NewRegistry()
	var wg sync.WaitGroup
	for i := range 50 {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			name := "tool-" + string(rune('A'+n%26))
			_ = r.Register(ToolDefinition{Name: name}, dummyHandler)
			_, _, _ = r.Get(name)
			_ = r.List()
		}(i)
	}
	wg.Wait()
}
