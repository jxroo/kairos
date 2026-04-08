package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadDefaults(t *testing.T) {
	tmpDir := t.TempDir()
	cfg, err := Load(tmpDir)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg.Server.Host != "127.0.0.1" {
		t.Errorf("expected host 127.0.0.1, got %s", cfg.Server.Host)
	}
	if cfg.Server.Port != 7777 {
		t.Errorf("expected port 7777, got %d", cfg.Server.Port)
	}
	if cfg.Log.Level != "info" {
		t.Errorf("expected log level info, got %s", cfg.Log.Level)
	}
	if cfg.Log.Format != "json" {
		t.Errorf("expected log format json, got %s", cfg.Log.Format)
	}
	if cfg.Memory.Decay.Factor != 0.95 {
		t.Errorf("expected decay factor 0.95, got %f", cfg.Memory.Decay.Factor)
	}
	if cfg.Memory.Decay.Threshold != 0.01 {
		t.Errorf("expected decay threshold 0.01, got %f", cfg.Memory.Decay.Threshold)
	}
	if cfg.Memory.Search.Limit != 5 {
		t.Errorf("expected search limit 5, got %d", cfg.Memory.Search.Limit)
	}
	if cfg.Memory.Search.MinRelevance != 0.3 {
		t.Errorf("expected min_relevance 0.3, got %f", cfg.Memory.Search.MinRelevance)
	}
	if cfg.Memory.Engine != "fallback" {
		t.Errorf("expected engine fallback, got %s", cfg.Memory.Engine)
	}
	if !cfg.RAG.Enabled {
		t.Error("expected RAG enabled by default")
	}
	if cfg.RAG.ChunkSize != 512 {
		t.Errorf("expected chunk_size 512, got %d", cfg.RAG.ChunkSize)
	}
	if cfg.RAG.ChunkOverlap != 64 {
		t.Errorf("expected chunk_overlap 64, got %d", cfg.RAG.ChunkOverlap)
	}
	if cfg.RAG.MaxFileSize != 10485760 {
		t.Errorf("expected max_file_size 10485760, got %d", cfg.RAG.MaxFileSize)
	}
	if len(cfg.RAG.Extensions) == 0 {
		t.Error("expected default extensions")
	}
	if len(cfg.RAG.IgnoreDirs) == 0 {
		t.Error("expected default ignore_dirs")
	}
	// Inference defaults.
	if cfg.Inference.DefaultModel != "" {
		t.Errorf("expected empty default_model, got %s", cfg.Inference.DefaultModel)
	}
	if cfg.Inference.ContextSize != 4096 {
		t.Errorf("expected context_size 4096, got %d", cfg.Inference.ContextSize)
	}
	if !strings.Contains(cfg.Inference.SystemPrompt, "Kairos") {
		t.Errorf("expected default system_prompt to mention Kairos, got %q", cfg.Inference.SystemPrompt)
	}
	if !cfg.Inference.Ollama.Enabled {
		t.Error("expected ollama enabled by default")
	}
	if cfg.Inference.Ollama.URL != "http://localhost:11434" {
		t.Errorf("expected ollama URL http://localhost:11434, got %s", cfg.Inference.Ollama.URL)
	}
	if !cfg.Inference.Ollama.AutoDiscover {
		t.Error("expected ollama auto_discover true by default")
	}
	if cfg.Inference.LlamaCpp.Enabled {
		t.Error("expected llamacpp disabled by default")
	}
	if cfg.Inference.LlamaCpp.URL != "http://localhost:8080" {
		t.Errorf("expected llamacpp URL http://localhost:8080, got %s", cfg.Inference.LlamaCpp.URL)
	}
	// Dashboard defaults.
	if !cfg.Dashboard.Enabled {
		t.Error("expected dashboard enabled by default")
	}
	if cfg.Dashboard.DevMode {
		t.Error("expected dashboard dev_mode false by default")
	}
	if cfg.Dashboard.StaticDir != "" {
		t.Errorf("expected empty static_dir, got %s", cfg.Dashboard.StaticDir)
	}
}

func TestLoadFromFile(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.toml")
	content := []byte(`
[server]
host = "0.0.0.0"
port = 9999

[log]
level = "debug"
format = "console"
`)
	if err := os.WriteFile(cfgPath, content, 0644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(tmpDir)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg.Server.Host != "0.0.0.0" {
		t.Errorf("expected host 0.0.0.0, got %s", cfg.Server.Host)
	}
	if cfg.Server.Port != 9999 {
		t.Errorf("expected port 9999, got %d", cfg.Server.Port)
	}
}

func TestLoadMemoryFromFile(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.toml")
	content := []byte(`
[memory]
engine = "rust"

[memory.decay]
factor = 0.90
threshold = 0.05

[memory.search]
limit = 10
min_relevance = 0.5
`)
	if err := os.WriteFile(cfgPath, content, 0644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(tmpDir)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg.Memory.Engine != "rust" {
		t.Errorf("expected engine rust, got %s", cfg.Memory.Engine)
	}
	if cfg.Memory.Decay.Factor != 0.90 {
		t.Errorf("expected decay factor 0.90, got %f", cfg.Memory.Decay.Factor)
	}
	if cfg.Memory.Decay.Threshold != 0.05 {
		t.Errorf("expected decay threshold 0.05, got %f", cfg.Memory.Decay.Threshold)
	}
	if cfg.Memory.Search.Limit != 10 {
		t.Errorf("expected search limit 10, got %d", cfg.Memory.Search.Limit)
	}
	if cfg.Memory.Search.MinRelevance != 0.5 {
		t.Errorf("expected min_relevance 0.5, got %f", cfg.Memory.Search.MinRelevance)
	}
}

func TestLoadRAGFromFile(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.toml")
	content := []byte(`
[rag]
enabled = false
watch_paths = ["/data/docs"]
extensions = [".md", ".txt"]
chunk_size = 1024
chunk_overlap = 128
max_file_size = 5242880
`)
	if err := os.WriteFile(cfgPath, content, 0644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(tmpDir)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg.RAG.Enabled {
		t.Error("expected RAG disabled")
	}
	if cfg.RAG.ChunkSize != 1024 {
		t.Errorf("expected chunk_size 1024, got %d", cfg.RAG.ChunkSize)
	}
	if cfg.RAG.ChunkOverlap != 128 {
		t.Errorf("expected chunk_overlap 128, got %d", cfg.RAG.ChunkOverlap)
	}
	if cfg.RAG.MaxFileSize != 5242880 {
		t.Errorf("expected max_file_size 5242880, got %d", cfg.RAG.MaxFileSize)
	}
	if len(cfg.RAG.WatchPaths) != 1 || cfg.RAG.WatchPaths[0] != "/data/docs" {
		t.Errorf("expected watch_paths [/data/docs], got %v", cfg.RAG.WatchPaths)
	}
}

func TestLoadInferenceFromFile(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.toml")
	content := []byte(`
[inference]
default_model = "llama3:latest"
context_size = 8192
system_prompt = "You are a helpful assistant."

[inference.ollama]
enabled = false
url = "http://remote:11434"
auto_discover = false

[inference.llamacpp]
enabled = true
url = "http://localhost:9090"
`)
	if err := os.WriteFile(cfgPath, content, 0644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(tmpDir)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg.Inference.DefaultModel != "llama3:latest" {
		t.Errorf("expected default_model llama3:latest, got %s", cfg.Inference.DefaultModel)
	}
	if cfg.Inference.ContextSize != 8192 {
		t.Errorf("expected context_size 8192, got %d", cfg.Inference.ContextSize)
	}
	if cfg.Inference.SystemPrompt != "You are a helpful assistant." {
		t.Errorf("expected system_prompt, got %s", cfg.Inference.SystemPrompt)
	}
	if cfg.Inference.Ollama.Enabled {
		t.Error("expected ollama disabled")
	}
	if cfg.Inference.Ollama.URL != "http://remote:11434" {
		t.Errorf("expected ollama URL http://remote:11434, got %s", cfg.Inference.Ollama.URL)
	}
	if cfg.Inference.Ollama.AutoDiscover {
		t.Error("expected auto_discover false")
	}
	if !cfg.Inference.LlamaCpp.Enabled {
		t.Error("expected llamacpp enabled")
	}
	if cfg.Inference.LlamaCpp.URL != "http://localhost:9090" {
		t.Errorf("expected llamacpp URL http://localhost:9090, got %s", cfg.Inference.LlamaCpp.URL)
	}
}

func TestEnsureDataDir(t *testing.T) {
	tmpDir := t.TempDir()
	kairosDir := filepath.Join(tmpDir, ".kairos")
	_, err := Load(kairosDir)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if _, err := os.Stat(kairosDir); os.IsNotExist(err) {
		t.Error("expected data directory to be created")
	}
}

func TestParse(t *testing.T) {
	cfg, err := Parse([]byte("[server]\nport = 9090\n"), t.TempDir())
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	if cfg.Server.Port != 9090 {
		t.Fatalf("expected port 9090, got %d", cfg.Server.Port)
	}
}

func TestStarterTOMLUsesRequestedEngine(t *testing.T) {
	content := StarterTOML("rust")
	if !strings.Contains(content, `engine = "rust"`) {
		t.Fatalf("expected rust engine in starter config, got %q", content)
	}
}
