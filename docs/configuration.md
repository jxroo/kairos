# Configuration Reference

Kairos reads its configuration from `~/.kairos/config.toml`. All settings have sensible defaults, so a zero-length (or missing) config file produces a working setup on `127.0.0.1:7777` with the fallback memory engine and no inference providers.

This document covers every configuration section, key, type, default value, and description.

---

## Table of Contents

- [server](#server)
- [log](#log)
- [memory](#memory)
- [memory.decay](#memorydecay)
- [memory.search](#memorysearch)
- [rag](#rag)
- [inference](#inference)
- [inference.ollama](#inferenceollama)
- [inference.llamacpp](#inferencellamacpp)
- [tools](#tools)
- [mcp](#mcp)
- [dashboard](#dashboard)
- [Example Configs](#example-configs)

---

## `[server]`

Controls the HTTP daemon bind address and port.

| Key    | Type   | Default       | Description                                      |
|--------|--------|---------------|--------------------------------------------------|
| `host` | string | `"127.0.0.1"` | IP address to bind the HTTP server to.           |
| `port` | int    | `7777`        | TCP port for the HTTP server.                    |

```toml
[server]
host = "127.0.0.1"
port = 7777
```

---

## `[log]`

Controls logging output level and format.

| Key      | Type   | Default  | Description                                                     |
|----------|--------|----------|-----------------------------------------------------------------|
| `level`  | string | `"info"` | Minimum log level. One of `"debug"`, `"info"`, `"warn"`, `"error"`. |
| `format` | string | `"json"` | Log output format. One of `"json"` or `"console"`.              |

```toml
[log]
level = "info"
format = "json"
```

---

## `[memory]`

Top-level memory engine configuration.

| Key      | Type   | Default      | Description                                                                                       |
|----------|--------|--------------|---------------------------------------------------------------------------------------------------|
| `engine` | string | `"fallback"` | Which embedding/search engine to use. `"rust"` for the production Rust vecstore (fastembed + usearch), `"fallback"` for the pure-Go hash-based embedder with brute-force cosine search. |

```toml
[memory]
engine = "fallback"
```

---

## `[memory.decay]`

Controls how stored memories lose relevance over time via exponential decay. A memory's effective relevance is multiplied by `factor ^ days_since_last_access`. Memories that fall below `threshold` become candidates for pruning.

| Key         | Type  | Default | Description                                                        |
|-------------|-------|---------|--------------------------------------------------------------------|
| `factor`    | float | `0.95`  | Exponential decay factor applied per day since last access.        |
| `threshold` | float | `0.01`  | Memories with relevance below this value are eligible for pruning. |

```toml
[memory.decay]
factor = 0.95
threshold = 0.01
```

---

## `[memory.search]`

Defaults for memory search queries.

| Key             | Type  | Default | Description                                        |
|-----------------|-------|---------|----------------------------------------------------|
| `limit`         | int   | `5`     | Default maximum number of search results returned. |
| `min_relevance` | float | `0.3`   | Minimum relevance score for a result to be included in the response. |

```toml
[memory.search]
limit = 5
min_relevance = 0.3
```

---

## `[rag]`

Controls the RAG (Retrieval-Augmented Generation) pipeline: file parsing, chunking, indexing, and hybrid search. When `enabled` is `true`, Kairos watches the configured paths for file changes and keeps a searchable index of their content.

| Key             | Type     | Default                                                                  | Description                                                               |
|-----------------|----------|--------------------------------------------------------------------------|---------------------------------------------------------------------------|
| `enabled`       | bool     | `true`                                                                   | Enable or disable the RAG pipeline entirely.                              |
| `watch_paths`   | []string | `["~/Documents", "~/Projects"]`                                         | Directories to watch recursively for indexable files.                     |
| `extensions`    | []string | `[".md", ".txt", ".go", ".py", ".rs", ".js", ".ts", ".pdf"]`            | File extensions to include when indexing.                                 |
| `ignore_dirs`   | []string | `[".git", "node_modules", "vendor", "target", ".venv", "__pycache__"]`  | Directory names to skip during recursive traversal.                       |
| `chunk_size`    | int      | `512`                                                                    | Target size of each text chunk in characters.                             |
| `chunk_overlap` | int      | `64`                                                                     | Number of overlapping characters between consecutive chunks.              |

```toml
[rag]
enabled = true
watch_paths = ["~/Documents", "~/Projects"]
extensions = [".md", ".txt", ".go", ".py", ".rs", ".js", ".ts", ".pdf"]
ignore_dirs = [".git", "node_modules", "vendor", "target", ".venv", "__pycache__"]
chunk_size = 512
chunk_overlap = 64
```

---

## `[inference]`

Top-level inference bridge settings. Kairos assembles context from memory and RAG results, then forwards the prompt to a configured LLM provider.

| Key            | Type | Default | Description                                                                              |
|----------------|------|---------|------------------------------------------------------------------------------------------|
| `context_size` | int  | `4096`  | Maximum token budget for the assembled context (memory + RAG) injected into the prompt.  |

```toml
[inference]
context_size = 4096
```

---

## `[inference.ollama]`

Configuration for the Ollama LLM provider (native HTTP API).

| Key       | Type   | Default                        | Description                          |
|-----------|--------|--------------------------------|--------------------------------------|
| `enabled` | bool   | `true`                         | Enable Ollama provider discovery.    |
| `url`     | string | `"http://localhost:11434"`     | Base URL of the Ollama HTTP API.     |

```toml
[inference.ollama]
enabled = true
url = "http://localhost:11434"
```

---

## `[inference.llamacpp]`

Configuration for the llama.cpp LLM provider (OpenAI-compatible API).

| Key       | Type   | Default                      | Description                                |
|-----------|--------|------------------------------|--------------------------------------------|
| `enabled` | bool   | `false`                      | Enable llama.cpp provider discovery.       |
| `url`     | string | `"http://localhost:8080"`    | Base URL of the llama.cpp HTTP server.     |

```toml
[inference.llamacpp]
enabled = false
url = "http://localhost:8080"
```

---

## `[tools]`

Controls the tool runtime that lets the LLM invoke local functions (file operations, shell commands, etc.).

| Key                | Type | Default | Description                                                  |
|--------------------|------|---------|--------------------------------------------------------------|
| `enable_builtins`  | bool | `true`  | Register the built-in tool set on startup.                   |
| `default_timeout`  | int  | `30`    | Default execution timeout for tool invocations, in seconds.  |

```toml
[tools]
enable_builtins = true
default_timeout = 30
```

---

## `[mcp]`

Model Context Protocol server settings. MCP allows external clients and editors to communicate with Kairos as a context provider.

| Key         | Type   | Default  | Description                                                                  |
|-------------|--------|----------|------------------------------------------------------------------------------|
| `enabled`   | bool   | `true`   | Enable the MCP server.                                                       |
| `transport` | string | `"both"` | Transport mode. One of `"stdio"`, `"sse"`, or `"both"`.                     |

```toml
[mcp]
enabled = true
transport = "both"
```

---

## `[dashboard]`

Web dashboard settings. The dashboard serves a single-page application for browsing memories, documents, conversations, and system status.

| Key        | Type   | Default | Description                                                                                   |
|------------|--------|---------|-----------------------------------------------------------------------------------------------|
| `enabled`  | bool   | `true`  | Serve the dashboard UI at `/dashboard/`.                                                      |
| `dev_mode` | bool   | `false` | When `true`, serves assets from `static_dir` on disk instead of the embedded filesystem.      |
| `static_dir` | string | `""`  | Path to a directory with dashboard assets. Only used when `dev_mode` is `true`.               |

```toml
[dashboard]
enabled = true
dev_mode = false
```

---

## Example Configs

### 1. Minimal

Start Kairos with all defaults. No config file is required; an empty file also works.

```toml
# ~/.kairos/config.toml
# Empty — all defaults apply.
# Kairos will listen on 127.0.0.1:7777 with the fallback memory engine.
```

### 2. Full-Featured (Production)

All sections configured for production use with the Rust vector engine, Ollama inference, expanded RAG coverage, and a generous context window.

```toml
# ~/.kairos/config.toml — production setup

[server]
host = "127.0.0.1"
port = 7777

[log]
level = "info"
format = "json"

[memory]
engine = "rust"

[memory.decay]
factor = 0.95
threshold = 0.01

[memory.search]
limit = 10
min_relevance = 0.25

[rag]
enabled = true
watch_paths = ["~/Documents", "~/Projects", "~/Notes"]
extensions = [".md", ".txt", ".go", ".py", ".rs", ".js", ".ts", ".jsx", ".tsx", ".pdf"]
ignore_dirs = [".git", "node_modules", "vendor", "target", ".venv", "__pycache__", "dist", "build"]
chunk_size = 512
chunk_overlap = 64

[inference]
context_size = 8192

[inference.ollama]
enabled = true
url = "http://localhost:11434"

[inference.llamacpp]
enabled = false
url = "http://localhost:8080"

[tools]
enable_builtins = true
default_timeout = 30

[mcp]
enabled = true
transport = "both"

[dashboard]
enabled = true
dev_mode = false
```

### 3. Development

Console-friendly logging, fallback engine (no Rust build needed), dashboard in dev mode serving assets from disk, and debug-level output.

```toml
# ~/.kairos/config.toml — development setup

[server]
host = "127.0.0.1"
port = 7777

[log]
level = "debug"
format = "console"

[memory]
engine = "fallback"

[memory.decay]
factor = 0.95
threshold = 0.01

[memory.search]
limit = 5
min_relevance = 0.1

[rag]
enabled = true
watch_paths = ["~/Projects/kairos/testdata"]
extensions = [".md", ".txt", ".go"]
ignore_dirs = [".git", "vendor"]
chunk_size = 256
chunk_overlap = 32

[inference]
context_size = 4096

[inference.ollama]
enabled = true
url = "http://localhost:11434"

[inference.llamacpp]
enabled = false
url = "http://localhost:8080"

[tools]
enable_builtins = true
default_timeout = 60

[mcp]
enabled = true
transport = "stdio"

[dashboard]
enabled = true
dev_mode = true
static_dir = "./dashboard/dist"
```
