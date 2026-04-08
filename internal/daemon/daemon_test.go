package daemon

import (
	"context"
	"net/http"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/jxroo/kairos/internal/config"
	"github.com/jxroo/kairos/internal/server"
)

func TestDaemonStartStop(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := &config.Config{
		Server: config.ServerConfig{Host: "127.0.0.1", Port: 0},
		Log:    config.LogConfig{Level: "info", Format: "json"},
		Data:   config.DataConfig{Dir: tmpDir},
	}
	logger := zap.NewNop()
	srv := server.New(logger, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, server.RuntimeInfo{})

	d := New(cfg, srv, logger)

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() { errCh <- d.Run(ctx) }()

	time.Sleep(100 * time.Millisecond)

	addr := d.Addr()
	if addr == "" {
		t.Fatal("expected non-empty address")
	}

	resp, err := http.Get("http://" + addr + "/health")
	if err != nil {
		t.Fatalf("health check failed: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	cancel()
	if err := <-errCh; err != nil {
		t.Errorf("Run() returned error: %v", err)
	}
}
