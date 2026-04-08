package mcp

import (
	"testing"

	"github.com/jxroo/kairos/internal/memory"
	"github.com/jxroo/kairos/internal/rag"
	"go.uber.org/zap"

	_ "github.com/mattn/go-sqlite3"
)

func TestNew_RegistersTools(t *testing.T) {
	dbPath := t.TempDir() + "/test.db"
	store, err := memory.NewStore(dbPath, zap.NewNop())
	if err != nil {
		t.Fatalf("creating store: %v", err)
	}
	defer store.Close()

	s := New(store, nil, nil, nil, nil, rag.NewProgress(), zap.NewNop())
	if s.MCPServer() == nil {
		t.Fatal("MCPServer() returned nil")
	}
}
