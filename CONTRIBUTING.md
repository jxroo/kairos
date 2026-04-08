# Contributing to Kairos

Have questions? Join us on [Discord](https://discord.gg/kairos).

## Development Setup

### Prerequisites

- Go 1.24+
- Rust stable toolchain
- Node.js 20+ (for the embedded dashboard build)
- Python 3.10+ (only if you want to run `sdk-python` tests)
- Make
- SQLite3 development headers (linux: `libsqlite3-dev`)

### Clone and Build

```bash
git clone https://github.com/jxroo/kairos.git
cd kairos
make build
```

### Developer Tools

`make lint` requires `golangci-lint` v2. Install it into `$GOPATH/bin` with:

```bash
make install-tools
```

The Makefile picks up `golangci-lint` from `$GOPATH/bin` automatically, so
you do **not** need to add it to your shell `PATH` for `make lint` to work.
If you want to invoke `golangci-lint` directly outside of Make, add
`$(go env GOPATH)/bin` to your `PATH`.

### Run Tests

```bash
# Core runtime (Go + Rust + dashboard build)
make test

# Python SDK
python -m pip install -e ./sdk-python[dev]
make test-python

# Full repo validation
make test-all
```

Notes:

- `make test` validates the core runtime. `make test-python` validates the Python SDK.
- The Rust library loader path is wired automatically by the Makefile for Go tests.
- Real embedder smoke tests are opt-in because they need model downloads on a cold machine:

```bash
make test-online
```

### Run in Development

```bash
make run
```

This builds everything and starts the daemon on `localhost:7777`.

## Project Structure

- `cmd/kairos/` -- CLI entry point (cobra)
- `internal/config/` -- Configuration (viper, TOML)
- `internal/daemon/` -- Process lifecycle
- `internal/server/` -- HTTP API (chi router)
- `internal/memory/` -- Memory engine (SQLite + vector search)
- `internal/rag/` -- RAG pipeline (indexing, chunking, hybrid search)
- `internal/inference/` -- LLM bridge (Ollama, llama.cpp)
- `internal/mcp/` -- MCP server & client
- `internal/tools/` -- Tool runtime (goja sandbox)
- `internal/vecbridge/` -- CGO bridge to Rust
- `vecstore/` -- Rust vector engine
- `dashboard/` -- embedded React dashboard
- `sdk-python/` -- Python SDK

High-level dependency flow:

- `cmd/kairos` depends on config, logging, daemon, and server
- `internal/server` coordinates memory, RAG, inference, tools, and dashboard assets
- `internal/vecbridge` is the Go-to-Rust bridge for `vecstore`

## Code Conventions

### Error Handling

- Always wrap errors with context: `fmt.Errorf("loading config: %w", err)`
- Never panic in library code. Return errors.

### Logging

- Use `*zap.Logger` via constructor injection.
- Never create global loggers or use the `log` package.

### Naming

- Standard Go conventions: `MixedCaps`, not `snake_case`.
- Interfaces: `-er` suffix where appropriate.

### Testing

- Table-driven tests preferred.
- Use `t.TempDir()` for filesystem tests.
- Never touch real `~/.kairos/` in tests.
- Run single package: `go test ./internal/config/ -v`

## Task Format

When creating GitHub Issues for development tasks (especially for AI-assisted development), use this format:

```markdown
## Task: [Name]
**Package:** internal/[pkg]
**Files:** file.go, file_test.go
**Depends on:** #N

### Context
[Why this task exists]

### Requirements
1. [Specific requirement]

### Interface
[Go code showing types/functions]

### Tests to Pass
1. [Test case]

### Constraints
- No panic, return errors
- No global state
```

## Pull Request Process

### Branch Naming

- `feat/description` for features
- `fix/description` for bug fixes
- `docs/description` for documentation

### Commit Messages

Use conventional commits:

- `feat: add memory search endpoint`
- `fix: handle nil pointer in context assembler`
- `test: add benchmark for vector search`
- `docs: update API reference`

### Requirements

1. Core tests pass (`make test`)
2. SDK tests pass when touching `sdk-python/` (`make test-python`)
3. No lint errors (`make lint`)
4. New code has test coverage
5. No breaking changes to the API without discussion

## Good First Issues

Look for issues labeled `good-first-issue`. Areas that are good starting points:

- Adding new file parsers to `internal/rag/`
- Adding built-in tools to `internal/tools/`
- Dashboard components (once the React app is set up)
- Documentation improvements

## License

By contributing, you agree that your contributions will be licensed under the MIT License.
