# Changelog

All notable changes to Kairos will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/),
and this project adheres to [Semantic Versioning](https://semver.org/).

## [0.1.0] - 2026-04-08

Initial release of Kairos, a personal AI runtime daemon.

### Added

- **Core runtime** — Go daemon with graceful shutdown, CLI (`start`, `stop`,
  `status`, `version`), TOML configuration via viper
- **HTTP API** — chi-based server with 18 REST endpoints, OpenAI-compatible
  chat completions (`/v1/chat/completions`, `/v1/models`)
- **Persistent memory** — SQLite-backed CRUD with importance scoring, tag
  filtering, entity extraction (dates, URLs, emails, persons, projects),
  exponential decay with configurable pruning
- **Vector search** — Rust engine (fastembed AllMiniLM-L6-V2 + usearch HNSW)
  exposed via CGO bridge, with pure-Go hash-based fallback for development;
  keyword boost scoring for short queries
- **RAG pipeline** — filesystem watcher with debounced indexing, intelligent
  chunking (512-char, heading/paragraph boundaries), hybrid vector + Bleve
  full-text search via RRF; supports `.md`, `.txt`, `.go`, `.py`, `.rs`,
  `.js`, `.ts`, code files (16 languages)
- **Inference bridge** — Ollama and llama.cpp providers with streaming SSE,
  model auto-discovery, provider manager with model routing
- **Context assembly** — memory + RAG injection into prompts with token budget,
  search query augmentation for follow-up messages, metadata-enriched context
- **Agentic tool loop** — iterative tool calling with configurable iteration
  and timeout limits (10 min default)
- **Tool runtime** — sandboxed JavaScript execution (goja) with permission
  system, audit logging, built-in tools (filesystem, network, git, shell)
- **MCP server** — stdio and SSE transport for Claude Desktop, Cursor, and
  other MCP clients; external MCP client connectivity
- **Dashboard** — embedded React (Vite + Tailwind) SPA with system status,
  memory browser, RAG index, conversations, and configuration editor
- **Conversation persistence** — full conversation history with message storage
- **Python SDK** — `kairos-sdk` 0.1.0 with sync/async client for memory,
  search, RAG, chat, and evaluator APIs
- **Distribution** — multi-platform release workflow (linux/darwin x
  amd64/arm64), `install.sh` with checksum verification, Homebrew formula
- **Containerization** — multi-stage Dockerfile (Rust + Node + Go + Debian
  slim runtime)
- **CI** — GitHub Actions for Go tests, golangci-lint, Rust clippy + tests,
  Python SDK tests, and benchmarks
- **Documentation** — API reference, configuration guide, MCP integration
  guide, Python SDK guide, contributing guidelines, security policy

[0.1.0]: https://github.com/jxroo/kairos/releases/tag/v0.1.0
