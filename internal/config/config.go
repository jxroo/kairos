package config

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/viper"
)

type Config struct {
	Server    ServerConfig    `mapstructure:"server"`
	Log       LogConfig       `mapstructure:"log"`
	Data      DataConfig      `mapstructure:"data"`
	Memory    MemoryConfig    `mapstructure:"memory"`
	RAG       RAGConfig       `mapstructure:"rag"`
	Inference InferenceConfig `mapstructure:"inference"`
	Tools     ToolsConfig     `mapstructure:"tools"`
	MCP       MCPConfig       `mapstructure:"mcp"`
	Dashboard DashboardConfig `mapstructure:"dashboard"`
}

type DashboardConfig struct {
	Enabled   bool   `mapstructure:"enabled"`
	DevMode   bool   `mapstructure:"dev_mode"`
	StaticDir string `mapstructure:"static_dir"`
}

type ToolsConfig struct {
	Dir            string           `mapstructure:"dir"`
	EnableBuiltins bool             `mapstructure:"enable_builtins"`
	DefaultTimeout int              `mapstructure:"default_timeout"` // seconds
	Permissions    []PermissionRule `mapstructure:"permissions"`
}

type PermissionRule struct {
	Tool     string   `mapstructure:"tool"`     // tool name or "*"
	Resource string   `mapstructure:"resource"` // "filesystem","network","shell"
	Allow    bool     `mapstructure:"allow"`
	Paths    []string `mapstructure:"paths,omitempty"`
}

type MCPConfig struct {
	Enabled         bool                `mapstructure:"enabled"`
	Transport       string              `mapstructure:"transport"` // "stdio","sse","both"
	ExternalServers []ExternalMCPServer `mapstructure:"external_servers"`
}

type ExternalMCPServer struct {
	Name    string   `mapstructure:"name"`
	Command string   `mapstructure:"command"`
	Args    []string `mapstructure:"args"`
	Env     []string `mapstructure:"env,omitempty"`
}

type InferenceConfig struct {
	DefaultModel    string         `mapstructure:"default_model"`
	Ollama          OllamaConfig   `mapstructure:"ollama"`
	LlamaCpp        LlamaCppConfig `mapstructure:"llamacpp"`
	ContextSize     int            `mapstructure:"context_size"`
	SystemPrompt    string         `mapstructure:"system_prompt"`
	ToolLoop        ToolLoopConfig `mapstructure:"tool_loop"`
	ResponseReserve int            `mapstructure:"response_reserve"` // tokens reserved for generation; 0 = default 512
}

// ToolLoopConfig controls the agentic tool-use loop in chat completions.
type ToolLoopConfig struct {
	Enabled       bool          `mapstructure:"enabled"`
	MaxIterations int           `mapstructure:"max_iterations"`
	MaxToolCalls  int           `mapstructure:"max_tool_calls"`
	Timeout       time.Duration `mapstructure:"timeout"`
}

type OllamaConfig struct {
	Enabled      bool   `mapstructure:"enabled"`
	URL          string `mapstructure:"url"`
	AutoDiscover bool   `mapstructure:"auto_discover"`
}

type LlamaCppConfig struct {
	Enabled bool   `mapstructure:"enabled"`
	URL     string `mapstructure:"url"`
}

type MemoryConfig struct {
	Decay  DecayConfig  `mapstructure:"decay"`
	Search SearchConfig `mapstructure:"search"`
	Engine string       `mapstructure:"engine"` // "rust" or "fallback"
}

type DecayConfig struct {
	Factor    float64 `mapstructure:"factor"`
	Threshold float64 `mapstructure:"threshold"`
}

type SearchConfig struct {
	Limit        int     `mapstructure:"limit"`
	MinRelevance float64 `mapstructure:"min_relevance"`
}

type ServerConfig struct {
	Host string `mapstructure:"host"`
	Port int    `mapstructure:"port"`
}

type LogConfig struct {
	Level  string `mapstructure:"level"`
	Format string `mapstructure:"format"`
}

type DataConfig struct {
	Dir string `mapstructure:"dir"`
}

type RAGConfig struct {
	Enabled      bool     `mapstructure:"enabled"`
	WatchPaths   []string `mapstructure:"watch_paths"`
	Extensions   []string `mapstructure:"extensions"`
	IgnoreDirs   []string `mapstructure:"ignore_dirs"`
	ChunkSize    int      `mapstructure:"chunk_size"`
	ChunkOverlap int      `mapstructure:"chunk_overlap"`
	MaxFileSize  int64    `mapstructure:"max_file_size"`
}

func Load(dataDir string) (*Config, error) {
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, fmt.Errorf("creating data directory: %w", err)
	}

	v := viper.New()
	v.SetConfigName("config")
	v.SetConfigType("toml")
	v.AddConfigPath(dataDir)
	applyDefaults(v, dataDir)

	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("reading config: %w", err)
		}
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("unmarshaling config: %w", err)
	}
	if cfg.Data.Dir == "" {
		cfg.Data.Dir = dataDir
	}
	return &cfg, nil
}

// Parse validates TOML content against Kairos config defaults without
// rewriting the caller's original document.
func Parse(content []byte, dataDir string) (*Config, error) {
	v := viper.New()
	v.SetConfigType("toml")
	applyDefaults(v, dataDir)

	if err := v.ReadConfig(bytes.NewReader(content)); err != nil {
		return nil, fmt.Errorf("reading config: %w", err)
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("unmarshaling config: %w", err)
	}
	if cfg.Data.Dir == "" {
		cfg.Data.Dir = dataDir
	}
	return &cfg, nil
}

// StarterTOML returns the starter configuration shown in installers and the
// dashboard editor when no config file exists yet.
func StarterTOML(memoryEngine string) string {
	engine := strings.TrimSpace(memoryEngine)
	if engine == "" {
		engine = "rust"
	}

	return fmt.Sprintf(`# Kairos Configuration
# See: https://github.com/jxroo/kairos

[server]
host = "127.0.0.1"
port = 7777

[log]
level = "info"
format = "json"

[memory]
engine = "%s"

[rag]
enabled = true
# watch_paths = ["~/Documents", "~/Projects"]

[inference.ollama]
enabled = true
url = "http://localhost:11434"

[mcp]
enabled = true
transport = "both"

[dashboard]
enabled = true
`, engine)
}

func DefaultDataDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("getting home directory: %w", err)
	}
	return filepath.Join(home, ".kairos"), nil
}

func applyDefaults(v *viper.Viper, dataDir string) {
	v.SetDefault("server.host", "127.0.0.1")
	v.SetDefault("server.port", 7777)
	v.SetDefault("log.level", "info")
	v.SetDefault("log.format", "json")
	v.SetDefault("data.dir", dataDir)
	v.SetDefault("memory.decay.factor", 0.95)
	v.SetDefault("memory.decay.threshold", 0.01)
	v.SetDefault("memory.search.limit", 5)
	v.SetDefault("memory.search.min_relevance", 0.3)
	v.SetDefault("memory.engine", "fallback")
	v.SetDefault("rag.enabled", true)
	v.SetDefault("rag.watch_paths", []string{"~/Documents", "~/Projects"})
	v.SetDefault("rag.extensions", []string{".md", ".txt", ".go", ".py", ".rs", ".js", ".ts", ".pdf"})
	v.SetDefault("rag.ignore_dirs", []string{".git", "node_modules", "vendor", "target", ".venv", "__pycache__"})
	v.SetDefault("rag.chunk_size", 512)
	v.SetDefault("rag.chunk_overlap", 64)
	v.SetDefault("rag.max_file_size", 10485760)
	v.SetDefault("inference.default_model", "")
	v.SetDefault("inference.context_size", 4096)
	v.SetDefault("inference.system_prompt", strings.TrimSpace(`
You are a helpful assistant powered by Kairos, a personal AI with long-term memory and access to the user's documents.

Your context may include these sections:
- "Relevant Memories": facts previously saved about this user. Reference them naturally without saying you are reading from memory.
- "Relevant Documents": excerpts from the user's local files. Cite the source filename when helpful.

You have tools:
- kairos_remember: save important new facts, preferences, or deadlines for future conversations.
- kairos_recall: search memories when the provided context is insufficient.
- kairos_search_files: search documents when the provided context is insufficient.

Use the provided context first. Only call tools when it does not contain what you need.
`))
	v.SetDefault("inference.ollama.enabled", true)
	v.SetDefault("inference.ollama.url", "http://localhost:11434")
	v.SetDefault("inference.ollama.auto_discover", true)
	v.SetDefault("inference.llamacpp.enabled", false)
	v.SetDefault("inference.llamacpp.url", "http://localhost:8080")
	v.SetDefault("inference.response_reserve", 512)
	v.SetDefault("inference.tool_loop.enabled", true)
	v.SetDefault("inference.tool_loop.max_iterations", 5)
	v.SetDefault("inference.tool_loop.max_tool_calls", 10)
	v.SetDefault("inference.tool_loop.timeout", "10m")
	v.SetDefault("tools.enable_builtins", true)
	v.SetDefault("tools.default_timeout", 30)
	v.SetDefault("mcp.enabled", true)
	v.SetDefault("mcp.transport", "both")
	v.SetDefault("dashboard.enabled", true)
	v.SetDefault("dashboard.dev_mode", false)
	v.SetDefault("dashboard.static_dir", "")
}
