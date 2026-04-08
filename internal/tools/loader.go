package tools

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/pelletier/go-toml/v2"
	"go.uber.org/zap"
)

// tomlToolFile represents the TOML structure of a tool definition file.
type tomlToolFile struct {
	Name        string              `toml:"name"`
	Description string              `toml:"description"`
	InputSchema map[string]Param    `toml:"input_schema"`
	Permissions []Permission        `toml:"permissions"`
	Script      tomlScriptSection   `toml:"script"`
}

type tomlScriptSection struct {
	Inline string `toml:"inline"`
	File   string `toml:"file"`
}

// LoadToolFile parses a single .toml tool definition file.
// Returns the ToolDefinition, the JS script source, and any error.
func LoadToolFile(path string) (*ToolDefinition, string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, "", fmt.Errorf("reading tool file %s: %w", path, err)
	}

	var tf tomlToolFile
	if err := toml.Unmarshal(data, &tf); err != nil {
		return nil, "", fmt.Errorf("parsing tool file %s: %w", path, err)
	}

	if tf.Name == "" {
		return nil, "", fmt.Errorf("tool file %s: name is required", path)
	}

	var script string
	if tf.Script.Inline != "" {
		script = tf.Script.Inline
	} else if tf.Script.File != "" {
		// Resolve relative to the TOML file's directory.
		scriptPath := tf.Script.File
		if !filepath.IsAbs(scriptPath) {
			scriptPath = filepath.Join(filepath.Dir(path), scriptPath)
		}
		scriptData, err := os.ReadFile(scriptPath)
		if err != nil {
			return nil, "", fmt.Errorf("reading script file %s: %w", scriptPath, err)
		}
		script = string(scriptData)
	}

	def := &ToolDefinition{
		Name:        tf.Name,
		Description: tf.Description,
		InputSchema: tf.InputSchema,
		Permissions: tf.Permissions,
		ScriptPath:  path,
	}
	return def, script, nil
}

// LoadToolsFromDir scans dir for .toml files, parses each, wraps the script
// with sandbox, and registers the tool. Errors on individual files are logged
// but don't stop processing.
func LoadToolsFromDir(dir string, registry *Registry, sandbox *SandboxRunner, logger *zap.Logger) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // no tools dir is fine
		}
		return fmt.Errorf("reading tools directory: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".toml") {
			continue
		}

		path := filepath.Join(dir, entry.Name())
		def, script, err := LoadToolFile(path)
		if err != nil {
			logger.Warn("skipping tool file", zap.String("file", path), zap.Error(err))
			continue
		}

		handler := sandbox.WrapScript(def.Name, script)
		if err := registry.Register(*def, handler); err != nil {
			logger.Warn("failed to register tool", zap.String("tool", def.Name), zap.Error(err))
		} else {
			logger.Info("loaded custom tool", zap.String("tool", def.Name))
		}
	}
	return nil
}
