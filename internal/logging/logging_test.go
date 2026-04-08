package logging

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNewLogger(t *testing.T) {
	tmpDir := t.TempDir()
	logDir := filepath.Join(tmpDir, "logs")

	logger, err := New("info", "json", logDir)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	if logger == nil {
		t.Fatal("expected non-nil logger")
	}
	if _, err := os.Stat(logDir); os.IsNotExist(err) {
		t.Error("expected log directory to be created")
	}
	logger.Info("test message")
	_ = logger.Sync()
}

func TestNewLoggerInvalidLevel(t *testing.T) {
	tmpDir := t.TempDir()
	_, err := New("invalid_level", "json", tmpDir)
	if err == nil {
		t.Error("expected error for invalid log level")
	}
}
