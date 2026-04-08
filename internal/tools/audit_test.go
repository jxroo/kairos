package tools

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/jxroo/kairos/internal/memory"
	"go.uber.org/zap"

	_ "github.com/mattn/go-sqlite3"
)

func setupAuditDB(t *testing.T) *AuditLogger {
	t.Helper()
	dbPath := t.TempDir() + "/test.db"
	store, err := memory.NewStore(dbPath, zap.NewNop())
	if err != nil {
		t.Fatalf("creating store: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	return NewAuditLogger(store.DB(), zap.NewNop())
}

func TestAuditLogger_LogAndList(t *testing.T) {
	al := setupAuditDB(t)
	ctx := context.Background()

	err := al.Log(ctx, AuditEntry{
		ToolName:   "calc",
		Arguments:  `{"expression":"2+2"}`,
		Result:     "4",
		IsError:    false,
		DurationMs: 5,
		Caller:     "test",
	})
	if err != nil {
		t.Fatalf("Log: %v", err)
	}

	entries, err := al.List(ctx, 10)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("got %d entries, want 1", len(entries))
	}
	if entries[0].ToolName != "calc" || entries[0].Result != "4" {
		t.Errorf("unexpected entry: %+v", entries[0])
	}
}

func TestAuditLogger_DescendingOrder(t *testing.T) {
	al := setupAuditDB(t)
	ctx := context.Background()

	for _, name := range []string{"first", "second", "third"} {
		_ = al.Log(ctx, AuditEntry{ToolName: name, DurationMs: 1})
	}

	entries, err := al.List(ctx, 10)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("got %d entries, want 3", len(entries))
	}
	// Most recent first.
	if entries[0].ToolName != "third" {
		t.Errorf("expected 'third' first, got %q", entries[0].ToolName)
	}
}

func TestAuditLogger_LargeResultTruncation(t *testing.T) {
	al := setupAuditDB(t)
	ctx := context.Background()

	bigResult := strings.Repeat("x", 20*1024)
	_ = al.Log(ctx, AuditEntry{
		ToolName:   "big",
		Result:     bigResult,
		DurationMs: 1,
	})

	entries, err := al.List(ctx, 1)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(entries[0].Result) > maxResultLen+50 { // allow for truncation suffix
		t.Errorf("result not truncated: len=%d", len(entries[0].Result))
	}
	if !strings.HasSuffix(entries[0].Result, "...[truncated]") {
		t.Error("truncated result missing suffix")
	}
}

func TestAuditLogger_ConcurrentSafety(t *testing.T) {
	al := setupAuditDB(t)
	ctx := context.Background()

	var wg sync.WaitGroup
	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = al.Log(ctx, AuditEntry{ToolName: "concurrent", DurationMs: 1})
		}()
	}
	wg.Wait()

	entries, err := al.List(ctx, 100)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(entries) != 20 {
		t.Errorf("got %d entries, want 20", len(entries))
	}
}
