package tools

import (
	"errors"
	"sync"
)

var (
	// ErrToolNotFound is returned when a tool is not registered.
	ErrToolNotFound = errors.New("tool not found")
	// ErrToolExists is returned when attempting to register a duplicate tool.
	ErrToolExists = errors.New("tool already registered")
)

type registryEntry struct {
	def     ToolDefinition
	handler ToolHandler
}

// Registry is a thread-safe store of tool definitions and their handlers.
type Registry struct {
	mu    sync.RWMutex
	tools map[string]registryEntry
}

// NewRegistry creates an empty Registry.
func NewRegistry() *Registry {
	return &Registry{tools: make(map[string]registryEntry)}
}

// Register adds a tool to the registry. Returns ErrToolExists if the name is
// already taken.
func (r *Registry) Register(def ToolDefinition, handler ToolHandler) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.tools[def.Name]; ok {
		return ErrToolExists
	}
	r.tools[def.Name] = registryEntry{def: def, handler: handler}
	return nil
}

// Get retrieves a tool definition and handler by name.
func (r *Registry) Get(name string) (ToolDefinition, ToolHandler, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	e, ok := r.tools[name]
	if !ok {
		return ToolDefinition{}, nil, ErrToolNotFound
	}
	return e.def, e.handler, nil
}

// List returns all registered tool definitions.
func (r *Registry) List() []ToolDefinition {
	r.mu.RLock()
	defer r.mu.RUnlock()
	defs := make([]ToolDefinition, 0, len(r.tools))
	for _, e := range r.tools {
		defs = append(defs, e.def)
	}
	return defs
}

// Unregister removes a tool from the registry. Returns ErrToolNotFound if the
// tool does not exist.
func (r *Registry) Unregister(name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.tools[name]; !ok {
		return ErrToolNotFound
	}
	delete(r.tools, name)
	return nil
}
