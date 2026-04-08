package mcp

import (
	"context"
	"testing"

	"github.com/jxroo/kairos/internal/config"
	"github.com/jxroo/kairos/internal/tools"
	"go.uber.org/zap"
)

func TestExternalClient_ConnectFailureNonFatal(t *testing.T) {
	registry := tools.NewRegistry()
	ec := NewExternalClient(registry, zap.NewNop())

	// Connecting to a nonexistent server should not return an error.
	err := ec.ConnectAll(context.Background(), []config.ExternalMCPServer{
		{Name: "fake", Command: "/nonexistent/binary", Args: []string{}},
	})
	if err != nil {
		t.Errorf("expected nil error for failed connection, got %v", err)
	}

	// No tools should be registered.
	if len(registry.List()) != 0 {
		t.Errorf("expected 0 tools, got %d", len(registry.List()))
	}
}

func TestExternalClient_CloseEmpty(t *testing.T) {
	registry := tools.NewRegistry()
	ec := NewExternalClient(registry, zap.NewNop())

	if err := ec.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
}
