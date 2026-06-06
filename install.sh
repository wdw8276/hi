#!/bin/sh
# hi — one-line installer (Linux & macOS)
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
    *) echo "Unsupported OS: $OS — installer supports Linux and macOS only"; exit 1 ;;
esac

# Get latest release tag.
TAG=$(curl -sL "https://api.github.com/repos/$REPO/releases/latest" | grep '"tag_name":' | sed -E 's/.*"([^"]+)".*/\1/')
if [ -z "$TAG" ]; then
    TAG="v1.0.0"  # fallback
fi

VERSION="${TAG#v}"
ARCHIVE="hi-${VERSION}-${OS}-${ARCH}.tar.gz"
URL="https://github.com/$REPO/releases/download/$TAG/$ARCHIVE"

echo "Downloading hi $TAG for $OS/$ARCH..."
curl -fsSL "$URL" -o "$ARCHIVE"
tar xzf "$ARCHIVE"
rm -f "$ARCHIVE"

INSTALL_DIR="${PREFIX:-/usr/local/bin}"
if [ ! -w "$INSTALL_DIR" ]; then
    echo "Need sudo to install to $INSTALL_DIR"
    sudo install -m 755 "$BIN" "$INSTALL_DIR/$BIN"
else
    install -m 755 "$BIN" "$INSTALL_DIR/$BIN"
fi

echo ""
echo "hi $TAG installed to $INSTALL_DIR/$BIN"
echo "Run: hi launch"
