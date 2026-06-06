#!/bin/sh
# hi — one-line installer
# Usage: curl -fsSL https://raw.githubusercontent.com/wdw8276/hi/main/install.sh | sh

set -e

REPO="wdw8276/hi"
BIN="hi"

# Detect OS and architecture.
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)
case "$ARCH" in
    x86_64|amd64) ARCH="amd64" ;;
    aarch64|arm64) ARCH="arm64" ;;
    *) echo "Unsupported architecture: $ARCH"; exit 1 ;;
esac
case "$OS" in
    linux|darwin) ;;
    mingw*|msys*|cygwin*) OS="windows"; ARCH="amd64" ;;
    *) echo "Unsupported OS: $OS"; exit 1 ;;
esac

# Get latest release tag.
TAG=$(curl -sL "https://api.github.com/repos/$REPO/releases/latest" | grep '"tag_name":' | sed -E 's/.*"([^"]+)".*/\1/')
if [ -z "$TAG" ]; then
    TAG="v1.0.0"  # fallback
fi

# Handle "v" prefix stripping for the tag.
VERSION="${TAG#v}"

if [ "$OS" = "windows" ]; then
    ARCHIVE="hi-${VERSION}-windows-amd64.zip"
    EXT=".exe"
else
    ARCHIVE="hi-${VERSION}-${OS}-${ARCH}.tar.gz"
    EXT=""
fi

URL="https://github.com/$REPO/releases/download/$TAG/$ARCHIVE"

echo "Downloading hi $TAG for $OS/$ARCH..."
curl -fsSL "$URL" -o "$ARCHIVE"

if [ "$OS" = "windows" ]; then
    unzip -o "$ARCHIVE" "$BIN$EXT"
else
    tar xzf "$ARCHIVE"
fi

rm -f "$ARCHIVE"

# Install to /usr/local/bin (Linux/macOS) or current directory (Windows).
if [ "$OS" != "windows" ]; then
    INSTALL_DIR="${PREFIX:-/usr/local/bin}"
    if [ ! -w "$INSTALL_DIR" ]; then
        echo "Need sudo to install to $INSTALL_DIR"
        sudo install -m 755 "$BIN" "$INSTALL_DIR/$BIN"
    else
        install -m 755 "$BIN" "$INSTALL_DIR/$BIN"
    fi
    echo ""
    echo "hi $TAG installed to $INSTALL_DIR/hi"
    echo "Run: hi launch"
else
    echo ""
    echo "hi $TAG downloaded."
    echo "Run: .\\$BIN$EXT launch"
fi
