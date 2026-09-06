#!/bin/sh
set -eu

# Server template variables (injected dynamically when served via /_maek/install.sh)
TEMPLATE_SERVER="__SERVER_URL__"
TEMPLATE_VERSION="__VERSION__"

BASE_URL="${SERVER_URL:-}"
if [ -z "$BASE_URL" ]; then
  case "$TEMPLATE_SERVER" in
    http://*|https://*) BASE_URL="$TEMPLATE_SERVER" ;;
  esac
fi

# Detect OS
OS_RAW="$(uname -s)"
case "$OS_RAW" in
  Darwin) OS="Darwin" ;;
  Linux)  OS="Linux" ;;
  *)
    echo "Error: Unsupported operating system '$OS_RAW'." >&2
    exit 1
    ;;
esac

# Detect Architecture
ARCH_RAW="$(uname -m)"
case "$ARCH_RAW" in
  x86_64|amd64) ARCH="x86_64" ;;
  arm64|aarch64) ARCH="arm64" ;;
  *)
    echo "Error: Unsupported architecture '$ARCH_RAW'." >&2
    exit 1
    ;;
esac

# Detect Version
VER="${VERSION:-}"
if [ -z "$VER" ]; then
  case "$TEMPLATE_VERSION" in
    v*|[0-9]*) VER="${TEMPLATE_VERSION#v}" ;;
  esac
fi
if [ -z "$VER" ]; then
  VER="$(curl -sSL "https://api.github.com/repos/gosuda/maek/releases/latest" 2>/dev/null | grep '"tag_name":' | sed -E 's/.*"v?([^"]+)".*/\1/' || echo "")"
  [ -z "$VER" ] && VER="0.1.0"
fi
VER="${VER#v}"

TARBALL="maek_${OS}_${ARCH}.tar.gz"
TMPDIR="$(mktemp -d 2>/dev/null || mktemp -d -t 'maek-install')"
cleanup() { rm -rf "$TMPDIR"; }
trap cleanup EXIT INT TERM

echo "==> Downloading maek for ${OS}/${ARCH}..."

DOWNLOAD_SUCCESS=0
# 1. Try downloading from maek server if available
if [ -n "$BASE_URL" ]; then
  SERVER_DOWNLOAD_URL="${BASE_URL}/_maek/download?os=${OS}&arch=${ARCH}"
  if curl -fsSL "$SERVER_DOWNLOAD_URL" -o "$TMPDIR/$TARBALL" 2>/dev/null; then
    DOWNLOAD_SUCCESS=1
  fi
fi

# 2. Fallback to GitHub Releases if server download failed or not configured
if [ "$DOWNLOAD_SUCCESS" -ne 1 ]; then
  GITHUB_RELEASE_TARBALL="maek_${VER}_${OS}_${ARCH}.tar.gz"
  GITHUB_URL="https://github.com/gosuda/maek/releases/download/v${VER}/${GITHUB_RELEASE_TARBALL}"
  echo "==> Fetching from GitHub Releases (v${VER})..."
  if ! curl -fsSL "$GITHUB_URL" -o "$TMPDIR/$TARBALL"; then
    echo "Error: Failed to download maek package from $GITHUB_URL" >&2
    exit 1
  fi
fi

# Extract binary
tar -xzf "$TMPDIR/$TARBALL" -C "$TMPDIR" maek 2>/dev/null || tar -xzf "$TMPDIR/$TARBALL" -C "$TMPDIR"

# Choose install directory
if [ -n "${INSTALL_DIR:-}" ]; then
  TARGET_DIR="$INSTALL_DIR"
elif [ -w "/usr/local/bin" ]; then
  TARGET_DIR="/usr/local/bin"
elif [ -d "$HOME/.local/bin" ] || mkdir -p "$HOME/.local/bin" 2>/dev/null; then
  TARGET_DIR="$HOME/.local/bin"
else
  TARGET_DIR="/usr/local/bin"
fi

mkdir -p "$TARGET_DIR" 2>/dev/null || true

echo "==> Installing to $TARGET_DIR/maek..."
if [ -w "$TARGET_DIR" ]; then
  install -m 755 "$TMPDIR/maek" "$TARGET_DIR/maek" 2>/dev/null || { cp "$TMPDIR/maek" "$TARGET_DIR/maek" && chmod 755 "$TARGET_DIR/maek"; }
else
  echo "==> Elevated permissions required to install to $TARGET_DIR"
  sudo install -m 755 "$TMPDIR/maek" "$TARGET_DIR/maek" 2>/dev/null || { sudo cp "$TMPDIR/maek" "$TARGET_DIR/maek" && sudo chmod 755 "$TARGET_DIR/maek"; }
fi

echo ""
echo "✓ maek installed successfully!"
echo "Location: $TARGET_DIR/maek"
"$TARGET_DIR/maek" version 2>/dev/null || true
echo ""

# Check if TARGET_DIR is in PATH
case ":$PATH:" in
  *":$TARGET_DIR:"*) ;;
  *)
    echo "Notice: $TARGET_DIR is not in your PATH."
    echo "To run 'maek' from anywhere, add this to your ~/.zshrc or ~/.bashrc:"
    echo "  export PATH=\"$TARGET_DIR:\$PATH\""
    echo ""
    ;;
esac
