#!/bin/sh
# hi — one-line installer (Linux & macOS)
# Usage: curl -fsSL https://raw.githubusercontent.com/mars-base/hi/main/install.sh | sh
#
# Supports fresh install and upgrade to latest release.

set -e

REPO="mars-base/hi"
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
    echo "Failed to fetch latest release. Check your internet connection."
    exit 1
fi

VERSION="${TAG#v}"

# Determine install directory.
INSTALL_DIR="${PREFIX:-/usr/local/bin}"
if [ ! -w "$INSTALL_DIR" ]; then
    INSTALL_DIR="${HOME}/.local/bin"
fi

# Check current version — skip if up to date.
if command -v "$BIN" >/dev/null 2>&1; then
    CURRENT=$("$BIN" --version 2>/dev/null | sed 's/^hi //')
    if [ "$CURRENT" = "$VERSION" ]; then
        echo "hi $TAG is already installed and up to date."
        exit 0
    fi
    if [ -n "$CURRENT" ]; then
        echo "Upgrading hi: $CURRENT -> $VERSION"
    fi
fi

ARCHIVE="hi-${VERSION}-${OS}-${ARCH}.tar.gz"
URL="https://github.com/$REPO/releases/download/$TAG/$ARCHIVE"

echo "Downloading hi $TAG for $OS/$ARCH..."
curl -fsSL "$URL" -o "$ARCHIVE"
tar xzf "$ARCHIVE"
rm -f "$ARCHIVE"

# Install.
mkdir -p "$INSTALL_DIR"
install -m 755 "$BIN" "$INSTALL_DIR/$BIN"

echo ""
echo "hi $TAG installed to $INSTALL_DIR/$BIN"
echo "Run: hi launch"
