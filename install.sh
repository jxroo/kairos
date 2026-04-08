#!/bin/bash
set -euo pipefail

# Kairos Installer
# Downloads and installs the latest Kairos release binary.
# Usage: curl -sSL https://raw.githubusercontent.com/jxroo/kairos/main/install.sh | bash

REPO="jxroo/kairos"
PREFIX=""
BIN_DIR=""
LIB_DIR=""
TMPDIR_CLEANUP=""

cleanup() {
    if [ -n "$TMPDIR_CLEANUP" ] && [ -d "$TMPDIR_CLEANUP" ]; then
        rm -rf "$TMPDIR_CLEANUP"
    fi
}
trap cleanup EXIT

echo "=== Kairos Installer ==="

# Detect OS.
OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
case "$OS" in
    linux)  OS="linux" ;;
    darwin) OS="darwin" ;;
    *)
        echo "Error: unsupported OS '$OS'. Kairos supports linux and darwin."
        exit 1
        ;;
esac

# Detect architecture.
ARCH="$(uname -m)"
case "$ARCH" in
    x86_64)  ARCH="amd64" ;;
    amd64)   ARCH="amd64" ;;
    aarch64) ARCH="arm64" ;;
    arm64)   ARCH="arm64" ;;
    *)
        echo "Error: unsupported architecture '$ARCH'. Kairos supports amd64 and arm64."
        exit 1
        ;;
esac

echo "Detected: ${OS}/${ARCH}"

# Determine install prefix.
if [ -w "/usr/local/bin" ]; then
    PREFIX="/usr/local"
else
    PREFIX="$HOME/.local"
fi
BIN_DIR="$PREFIX/bin"
LIB_DIR="$PREFIX/lib"
mkdir -p "$BIN_DIR" "$LIB_DIR"

echo "Install prefix: $PREFIX"

# Create temp directory.
TMPDIR_CLEANUP="$(mktemp -d)"
cd "$TMPDIR_CLEANUP"

# Download archive and checksums.
ARCHIVE="kairos-${OS}-${ARCH}.tar.gz"
BASE_URL="https://github.com/${REPO}/releases/latest/download"

echo "Downloading ${ARCHIVE}..."
if ! curl -fsSL -o "$ARCHIVE" "${BASE_URL}/${ARCHIVE}"; then
    echo "Error: failed to download ${ARCHIVE}."
    echo "Check that a release exists at: https://github.com/${REPO}/releases"
    exit 1
fi

echo "Downloading checksums..."
if curl -fsSL -o checksums.txt "${BASE_URL}/checksums.txt" 2>/dev/null; then
    # Extract only our platform's line to avoid failing on missing entries.
    if grep " ${ARCHIVE}$" checksums.txt > archive.sum 2>/dev/null && [ -s archive.sum ]; then
        echo "Verifying checksum..."
        if command -v sha256sum &>/dev/null; then
            if ! sha256sum --check archive.sum; then
                echo "Error: checksum verification failed."
                exit 1
            fi
        elif command -v shasum &>/dev/null; then
            if ! shasum -a 256 --check archive.sum; then
                echo "Error: checksum verification failed."
                exit 1
            fi
        else
            echo "Warning: no checksum tool found, skipping verification."
        fi
    else
        echo "Warning: no checksum entry for ${ARCHIVE}, skipping verification."
    fi
else
    echo "Warning: checksums.txt not found, skipping verification."
fi

# Extract.
echo "Extracting..."
tar xzf "$ARCHIVE"

# Install binary and Rust runtime library.
if [ -f "bin/kairos" ]; then
    chmod +x bin/kairos
    mv bin/kairos "$BIN_DIR/kairos"
elif [ -f "kairos" ]; then
    chmod +x kairos
    mv kairos "$BIN_DIR/kairos"
else
    echo "Error: kairos binary not found in archive."
    exit 1
fi

if [ -f "lib/libvecstore.so" ]; then
    mv lib/libvecstore.so "$LIB_DIR/libvecstore.so"
elif [ -f "lib/libvecstore.dylib" ]; then
    mv lib/libvecstore.dylib "$LIB_DIR/libvecstore.dylib"
else
    echo "Error: Rust runtime library not found in archive."
    exit 1
fi

# Create config directory.
KAIROS_DIR="$HOME/.kairos"
mkdir -p "$KAIROS_DIR"

# Write default config if none exists.
if [ ! -f "$KAIROS_DIR/config.toml" ]; then
    cat > "$KAIROS_DIR/config.toml" << 'TOML'
# Kairos Configuration
# See: https://github.com/jxroo/kairos

[server]
host = "127.0.0.1"
port = 7777

[log]
level = "info"
format = "json"

[memory]
engine = "rust"

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
TOML
    echo "Created default config at $KAIROS_DIR/config.toml"
fi

echo ""
echo "Kairos installed successfully!"
echo ""
echo "  Binary: $BIN_DIR/kairos"
echo "  Library: $LIB_DIR"
echo "  Config: $KAIROS_DIR/config.toml"
echo ""
echo "Get started:"
echo "  kairos start"
echo ""

# Check if install dir is in PATH.
case ":$PATH:" in
    *":$BIN_DIR:"*) ;;
    *)
        echo "Note: $BIN_DIR is not in your PATH."
        echo "Add it with: export PATH=\"$BIN_DIR:\$PATH\""
        ;;
esac
