package inference

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"go.uber.org/zap"

	"github.com/jxroo/kairos/internal/config"
)

// Sentinel errors returned by Manager operations.
var (
	ErrNoProviders   = errors.New("no inference providers available")
	ErrModelNotFound = errors.New("model not found")
)

// Manager coordinates multiple inference providers, routing requests to the
// appropriate backend based on the requested model.
type Manager struct {
	providers    []Provider
	logger       *zap.Logger
	defaultModel string
}

// NewManager creates an empty Manager. Providers must be added via Register or
// Discover before the Manager is usable.
func NewManager(logger *zap.Logger) *Manager {
	return &Manager{
		logger: logger,
	}
}

// Register adds a provider to the Manager.
func (m *Manager) Register(p Provider) {
	m.providers = append(m.providers, p)
}

// SetDefaultModel configures the preferred model used when requests omit one.
func (m *Manager) SetDefaultModel(model string) {
	m.defaultModel = strings.TrimSpace(model)
}

// Discover creates and pings providers based on the supplied InferenceConfig.
// Providers that are enabled in the config are instantiated, pinged, and
// registered if reachable. A failed ping is logged as a warning, not an error.
// Only fundamentally invalid configurations (e.g. a nil cfg) return an error.
func (m *Manager) Discover(ctx context.Context, cfg *config.InferenceConfig) error {
	if cfg == nil {
		return fmt.Errorf("discovering providers: config must not be nil")
	}

	m.defaultModel = strings.TrimSpace(cfg.DefaultModel)

	if cfg.Ollama.Enabled {
		p := NewOllamaProvider(cfg.Ollama.URL, m.logger)
		reachableURL, err := p.Discover(ctx, cfg.Ollama.AutoDiscover)
		if err != nil {
			m.logger.Warn("ollama provider unreachable, skipping",
				zap.String("url", cfg.Ollama.URL),
				zap.Error(err),
			)
		} else {
			// Use the discovered URL (may differ from configured if auto-discovered).
			if reachableURL != cfg.Ollama.URL {
				p = NewOllamaProvider(reachableURL, m.logger)
			}
			m.Register(p)
			m.logger.Info("registered ollama provider", zap.String("url", reachableURL))
		}
	}

	if cfg.LlamaCpp.Enabled {
		p := NewLlamaCppProvider(cfg.LlamaCpp.URL, m.logger)
		if err := p.Ping(ctx); err != nil {
			m.logger.Warn("llamacpp provider unreachable, skipping",
				zap.String("url", cfg.LlamaCpp.URL),
				zap.Error(err),
			)
		} else {
			m.Register(p)
			m.logger.Info("registered llamacpp provider", zap.String("url", cfg.LlamaCpp.URL))
		}
	}

	return nil
}

// ListModels aggregates the model lists from all registered providers.
func (m *Manager) ListModels(ctx context.Context) ([]Model, error) {
	var all []Model
	for _, p := range m.providers {
		models, err := p.ListModels(ctx)
		if err != nil {
			m.logger.Warn("listing models from provider failed",
				zap.String("provider", p.Name()),
				zap.Error(err),
			)
			continue
		}
		all = append(all, models...)
	}
	return all, nil
}

// ResolveModelInfo returns the provider and canonical model that should handle
// the request. When model is empty, the configured default model is preferred;
// otherwise the first available model is used.
func (m *Manager) ResolveModelInfo(ctx context.Context, model string) (Provider, Model, error) {
	if len(m.providers) == 0 {
		return nil, Model{}, ErrNoProviders
	}

	effectiveModel := strings.TrimSpace(model)
	if effectiveModel == "" && m.defaultModel != "" {
		effectiveModel = m.defaultModel
	}

	if effectiveModel == "" {
		for _, p := range m.providers {
			models, err := p.ListModels(ctx)
			if err != nil {
				m.logger.Warn("resolving default model: listing models failed",
					zap.String("provider", p.Name()),
					zap.Error(err),
				)
				continue
			}
			if len(models) > 0 {
				return p, models[0], nil
			}
		}
		return nil, Model{}, ErrModelNotFound
	}

	for _, p := range m.providers {
		models, err := p.ListModels(ctx)
		if err != nil {
			m.logger.Warn("resolving model: listing models failed",
				zap.String("provider", p.Name()),
				zap.Error(err),
			)
			continue
		}
		for _, mdl := range models {
			if mdl.ID == effectiveModel || mdl.Name == effectiveModel {
				return p, mdl, nil
			}
		}
	}

	return nil, Model{}, ErrModelNotFound
}

// ResolveModel returns the provider that owns the given model.
// If model is an empty string, the first model from the first provider is used.
// Returns ErrNoProviders if no providers are registered, ErrModelNotFound if
// no provider exposes the requested model.
func (m *Manager) ResolveModel(ctx context.Context, model string) (Provider, error) {
	p, _, err := m.ResolveModelInfo(ctx, model)
	return p, err
}

// Chat resolves the provider for the requested model and delegates the chat
// completion to that provider.
func (m *Manager) Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	p, resolved, err := m.ResolveModelInfo(ctx, req.Model)
	if err != nil {
		return nil, fmt.Errorf("chat: %w", err)
	}
	req.Model = resolved.ID
	resp, err := p.Chat(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("chat via %s: %w", p.Name(), err)
	}
	return resp, nil
}

// ChatStream resolves the provider for the requested model and initiates a
// streaming chat completion, returning a *StreamReader for incremental events.
func (m *Manager) ChatStream(ctx context.Context, req ChatRequest) (*StreamReader, error) {
	p, resolved, err := m.ResolveModelInfo(ctx, req.Model)
	if err != nil {
		return nil, fmt.Errorf("chat stream: %w", err)
	}
	req.Model = resolved.ID
	reader, err := p.ChatStream(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("chat stream via %s: %w", p.Name(), err)
	}
	return reader, nil
}
