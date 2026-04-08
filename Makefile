.PHONY: build build-rust build-dashboard build-go package-release test test-all test-rust test-go test-python test-online lint lint-go lint-rust clean clean-dashboard bench run dashboard-dev install-tools

RUST_LIB_DIR := vecstore/target/release
GO_BINARY := kairos
DIST_DIR := dist
VERSION ?= dev
GO_LDFLAGS := -X github.com/jxroo/kairos/internal/version.Version=$(VERSION)

# golangci-lint: prefer $PATH, fall back to $(go env GOPATH)/bin so local
# `go install` drops are picked up without requiring shell PATH tweaks.
GOPATH_BIN := $(shell go env GOPATH)/bin
GOLANGCI_LINT := $(shell command -v golangci-lint 2>/dev/null || echo $(GOPATH_BIN)/golangci-lint)

# Allow rpath flags in cgo LDFLAGS (required for darwin @executable_path).
export CGO_LDFLAGS_ALLOW = -Wl,-rpath,.*
OS := $(shell uname -s | tr '[:upper:]' '[:lower:]')
ARCH_RAW := $(shell uname -m)

ifeq ($(ARCH_RAW),x86_64)
ARCH := amd64
else ifeq ($(ARCH_RAW),amd64)
ARCH := amd64
else ifeq ($(ARCH_RAW),aarch64)
ARCH := arm64
else ifeq ($(ARCH_RAW),arm64)
ARCH := arm64
else
ARCH := $(ARCH_RAW)
endif

# Build everything
build: build-rust build-dashboard build-go

build-rust:
	cd vecstore && cargo build --release

build-dashboard:
	cd dashboard && npm ci && npm run build

build-go: build-rust build-dashboard
	CGO_ENABLED=1 \
	CGO_LDFLAGS="-L$(CURDIR)/$(RUST_LIB_DIR)" \
	go build -ldflags "$(GO_LDFLAGS)" -o $(GO_BINARY) ./cmd/kairos/

package-release: build
	rm -rf $(DIST_DIR)/stage
	mkdir -p $(DIST_DIR)/stage/bin $(DIST_DIR)/stage/lib
	cp $(GO_BINARY) $(DIST_DIR)/stage/bin/kairos
	if [ -f $(RUST_LIB_DIR)/libvecstore.so ]; then cp $(RUST_LIB_DIR)/libvecstore.so $(DIST_DIR)/stage/lib/; fi
	if [ -f $(RUST_LIB_DIR)/libvecstore.dylib ]; then cp $(RUST_LIB_DIR)/libvecstore.dylib $(DIST_DIR)/stage/lib/; fi
	printf '%s\n' '# Kairos Configuration' '# See: https://github.com/jxroo/kairos' '' '[server]' 'host = "127.0.0.1"' 'port = 7777' '' '[log]' 'level = "info"' 'format = "json"' '' '[memory]' 'engine = "rust"' '' '[rag]' 'enabled = true' '# watch_paths = ["~/Documents", "~/Projects"]' '' '[inference.ollama]' 'enabled = true' 'url = "http://localhost:11434"' '' '[mcp]' 'enabled = true' 'transport = "both"' '' '[dashboard]' 'enabled = true' > $(DIST_DIR)/stage/config.toml
	cp README.md LICENSE $(DIST_DIR)/stage/
	tar -C $(DIST_DIR)/stage -czf $(DIST_DIR)/kairos-$(OS)-$(ARCH).tar.gz .

# Core runtime tests
test: test-rust test-go

# Full repository validation
test-all: test test-python

test-rust:
	cd vecstore && cargo test

test-go: build-rust build-dashboard
	CGO_ENABLED=1 \
	LD_LIBRARY_PATH=$(CURDIR)/$(RUST_LIB_DIR) \
	go test ./... -v

test-python:
	python -m pytest sdk-python/tests -q

test-online: build-rust
	cd vecstore && KAIROS_ONLINE_TESTS=1 cargo test
	CGO_ENABLED=1 \
	LD_LIBRARY_PATH=$(CURDIR)/$(RUST_LIB_DIR) \
	KAIROS_ONLINE_TESTS=1 \
	go test ./internal/vecbridge -v

# Benchmarks
bench: build-rust build-dashboard
	CGO_ENABLED=1 \
	LD_LIBRARY_PATH=$(CURDIR)/$(RUST_LIB_DIR) \
	go test -bench=. -benchmem -run=^$$ ./...

# Lint
lint: lint-go lint-rust

lint-go: build-rust
	@if [ ! -x "$(GOLANGCI_LINT)" ]; then \
		echo "golangci-lint not found at $(GOLANGCI_LINT)"; \
		echo "Install with: make install-tools"; \
		exit 1; \
	fi
	CGO_LDFLAGS="-L$(CURDIR)/$(RUST_LIB_DIR)" \
	LD_LIBRARY_PATH=$(CURDIR)/$(RUST_LIB_DIR) \
	$(GOLANGCI_LINT) run ./...

lint-rust:
	cd vecstore && cargo clippy -- -D warnings

# Install developer tools into $(go env GOPATH)/bin. Safe to re-run; does
# nothing if versions already match.
install-tools:
	go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.11.4
	@echo "Installed to $(GOPATH_BIN). Add it to PATH:"
	@echo "  export PATH=\"$(GOPATH_BIN):\$$PATH\""

# Clean
clean: clean-dashboard
	rm -f $(GO_BINARY)
	cd vecstore && cargo clean

clean-dashboard:
	rm -rf dashboard/dist

# Run the daemon (dev mode)
run: build
	LD_LIBRARY_PATH=$(CURDIR)/$(RUST_LIB_DIR) ./$(GO_BINARY) start

# Dashboard dev server (Vite with HMR)
dashboard-dev:
	cd dashboard && if [ ! -x node_modules/.bin/vite ]; then npm ci; fi && npm run dev
