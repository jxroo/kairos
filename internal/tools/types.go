package tools

import "context"

// ToolDefinition describes a registered tool's metadata and schema.
type ToolDefinition struct {
	Name        string           `toml:"name" json:"name"`
	Description string           `toml:"description" json:"description"`
	InputSchema map[string]Param `toml:"input_schema" json:"input_schema"`
	Permissions []Permission     `toml:"permissions" json:"permissions"`
	Builtin     bool             `json:"builtin"`
	ScriptPath  string           `toml:"script" json:"-"`
}

// Param describes a single parameter in a tool's input schema.
type Param struct {
	Type        string   `toml:"type" json:"type"` // "string","number","boolean","object","array"
	Description string   `toml:"description" json:"description"`
	Required    bool     `toml:"required" json:"required"`
	Default     any      `toml:"default,omitempty" json:"default,omitempty"`
	Enum        []string `toml:"enum,omitempty" json:"enum,omitempty"`
}

// Permission declares what resources a tool may access.
type Permission struct {
	Resource string   `toml:"resource" json:"resource"` // "filesystem","network","shell"
	Allow    bool     `toml:"allow" json:"allow"`
	Paths    []string `toml:"paths,omitempty" json:"paths,omitempty"`
}

// ToolResult is the outcome of executing a tool.
type ToolResult struct {
	Content string `json:"content"`
	IsError bool   `json:"is_error"`
}

// ToolHandler is a function that executes a tool with the given arguments.
type ToolHandler func(ctx context.Context, args map[string]any) (*ToolResult, error)
