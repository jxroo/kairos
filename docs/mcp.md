# MCP Integration

## Overview

Kairos implements the [Model Context Protocol (MCP)](https://modelcontextprotocol.io/) to expose its capabilities -- memory search, RAG file search, conversation history, tool execution, and system status -- as tools that any MCP-compatible client can invoke. This lets AI assistants like Claude Desktop and Cursor tap directly into your local Kairos knowledge base during conversations.

Kairos acts as an **MCP server**, registering six tools that clients can discover and call. It also acts as an **MCP client**, connecting to external MCP servers you configure and proxying their tools through the Kairos tool registry.

## Transport Modes

Kairos supports two MCP transport modes:

| Transport | How it works | Used by |
|-----------|-------------|---------|
| **stdio** | `kairos mcp` runs as a subprocess, communicating over stdin/stdout | Claude Desktop, Cursor, any subprocess-based client |
| **SSE** | HTTP Server-Sent Events endpoint at `http://localhost:7777/mcp/sse` | HTTP-based MCP clients, browser integrations |

### Configuration

In your `~/.kairos/config.toml`:

```toml
[mcp]
enabled = true
transport = "both"   # "stdio", "sse", or "both"
```

- `enabled` -- master switch for MCP support (default: `true`)
- `transport` -- which transports to activate:
  - `"stdio"` -- only the `kairos mcp` subprocess mode
  - `"sse"` -- only the HTTP SSE endpoint
  - `"both"` -- both transports (default)

## Claude Desktop Setup

### 1. Locate the Claude Desktop configuration file

| OS | Path |
|----|------|
| macOS | `~/Library/Application Support/Claude/claude_desktop_config.json` |
| Linux | `~/.config/Claude/claude_desktop_config.json` |
| Windows | `%APPDATA%\Claude\claude_desktop_config.json` |

### 2. Add the Kairos MCP server

Open the configuration file and add (or merge into) the `mcpServers` object:

```json
{
  "mcpServers": {
    "kairos": {
      "command": "/path/to/kairos",
      "args": ["mcp"]
    }
  }
}
```

Replace `/path/to/kairos` with the absolute path to your `kairos` binary. You can find it by running:

```bash
which kairos
```

### 3. Verify the connection

1. Restart Claude Desktop.
2. Look for a hammer icon in the input area -- this indicates MCP tools are available.
3. Click the hammer icon to confirm `kairos_remember`, `kairos_recall`, and the other Kairos tools are listed.
4. Try asking Claude something like: "Search my memories for recent project notes" -- it should invoke the `kairos_recall` tool.

## Cursor Setup

### 1. Open Cursor MCP settings

In Cursor, open **Settings** and navigate to the **MCP** section (or edit `.cursor/mcp.json` in your project root or `~/.cursor/mcp.json` globally).

### 2. Add the Kairos MCP server

```json
{
  "mcpServers": {
    "kairos": {
      "command": "/path/to/kairos",
      "args": ["mcp"]
    }
  }
}
```

Replace `/path/to/kairos` with the absolute path to your `kairos` binary.

### 3. Verify

After reloading Cursor, the Kairos tools should appear in the MCP tools list. Cursor's agent mode can then use `kairos_recall`, `kairos_search_files`, and the other tools automatically.

## SSE Transport

For HTTP-based MCP clients, Kairos exposes an SSE endpoint when the daemon is running.

### Prerequisites

The Kairos daemon must be running:

```bash
kairos start
```

### Endpoint

```
http://localhost:7777/mcp/sse
```

The SSE transport is mounted at `/mcp` on the daemon's HTTP server (default port 7777). The base path handles both the SSE connection and JSON-RPC message exchange.

### Example: connecting with an HTTP MCP client

Point your MCP client's SSE transport at the endpoint:

```
Base URL: http://localhost:7777/mcp
SSE URL:  http://localhost:7777/mcp/sse
```

The client will receive tool definitions via the standard MCP discovery flow and can then call any of the registered Kairos tools.

## Generic MCP Client

Any MCP-compatible client can integrate with Kairos using either transport:

### stdio

Run `kairos mcp` as a subprocess. The binary communicates over stdin/stdout using the MCP JSON-RPC protocol.

```bash
# Example: spawning Kairos as a subprocess
kairos mcp
```

The process will block, reading JSON-RPC requests from stdin and writing responses to stdout. This is the standard subprocess model used by the MCP specification.

### SSE

Connect to the HTTP endpoint at `http://localhost:7777/mcp/sse` (requires the daemon to be running with SSE transport enabled).

## Available Tools

Kairos registers the following MCP tools:

### `kairos_remember`

Store a new memory with content, tags, and importance level.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `content` | string | yes | The memory content to store |
| `tags` | string[] | no | Tags for organizing the memory |
| `importance` | string | no | Importance level: `low`, `normal`, `high` |
| `context` | string | no | Additional context for the memory |

### `kairos_recall`

Search memories by semantic similarity.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `query` | string | yes | Search query |
| `limit` | number | no | Maximum results to return (default: 5) |
| `tags` | string[] | no | Required tags to filter by |
| `min_relevance` | number | no | Minimum relevance score (default: 0.3) |

### `kairos_search_files`

Search indexed files and documents using hybrid semantic + keyword search (RAG).

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `query` | string | yes | Search query |
| `limit` | number | no | Maximum results to return (default: 5) |

### `kairos_run_tool`

Execute a registered tool by name with given arguments. This includes built-in tools and any tools loaded from the tools directory or external MCP servers.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `tool_name` | string | yes | Name of the tool to execute |
| `arguments` | object | no | Tool arguments as key-value pairs |

### `kairos_conversations`

List or search past conversations.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `query` | string | no | Optional search query over titles and messages |
| `limit` | number | no | Maximum conversations to return (default: 20) |

### `kairos_status`

Get Kairos system status including memory count, index stats, and available models. Takes no parameters.

Returns a JSON object with:
- `version` -- Kairos version
- `memory_count` -- total number of stored memories
- `index` -- RAG indexing state, file counts, and progress percentage
- `models` -- list of available LLM model names

## External MCP Servers

Kairos can also connect to **other** MCP servers as a client, proxying their tools through the Kairos tool registry. This lets you aggregate tools from multiple MCP servers into a single interface.

Configure external servers in `~/.kairos/config.toml`:

```toml
[[mcp.external_servers]]
name = "my-tool-server"
command = "/path/to/server"
args = ["--flag"]
env = ["API_KEY=xxx"]
```

External tools are registered with the prefix `external:<server-name>:<tool-name>` and can be invoked via `kairos_run_tool` or the REST API.

## Troubleshooting

### "Connection refused" when using SSE

The Kairos daemon is not running. Start it first:

```bash
kairos start
```

Then verify it is listening:

```bash
curl -s http://localhost:7777/health
```

### "Command not found" in Claude Desktop or Cursor

The `kairos` binary is not in your PATH, or the path in the config is incorrect. Use the absolute path to the binary:

```json
{
  "mcpServers": {
    "kairos": {
      "command": "/usr/local/bin/kairos",
      "args": ["mcp"]
    }
  }
}
```

Find the correct path with:

```bash
which kairos
# or, if installed via go install:
ls "$(go env GOPATH)/bin/kairos"
```

### Transport mismatch

If the `kairos mcp` command exits with an error like `MCP transport "sse" does not include stdio`, your config restricts the transport. Either:

1. Change the transport setting to include the mode you need:

```toml
[mcp]
transport = "both"
```

2. Or use the transport that matches your config (e.g., use SSE if transport is set to `"sse"`).

### MCP server is disabled

If you see `MCP server is disabled in config`, enable it:

```toml
[mcp]
enabled = true
```

### Tools not appearing in Claude Desktop

1. Confirm `kairos mcp` runs without errors when executed manually in a terminal.
2. Check that the JSON in `claude_desktop_config.json` is valid (no trailing commas, correct quoting).
3. Restart Claude Desktop completely after editing the config.
4. Check the Kairos logs at `~/.kairos/logs/` for any startup errors.

### External MCP server connection failures

External server connection failures are non-fatal -- Kairos will start without those tools and log a warning. Check:

1. The `command` path exists and is executable.
2. Any required `env` variables are set correctly.
3. The external server implements the MCP protocol correctly.
