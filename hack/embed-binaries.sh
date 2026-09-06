#!/usr/bin/env bash
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$REPO_ROOT"

OUT_DIR="internal/server/static/bin"
mkdir -p "$OUT_DIR"
# Remove any existing binary archives
rm -f "${OUT_DIR}/maek_"*

VERSION="${VERSION:-$(git describe --tags --abbrev=0 2>/dev/null || echo "v0.1.0")}"
VERSION="${VERSION#v}"
COMMIT="${COMMIT:-$(git rev-parse HEAD 2>/dev/null || echo "none")}"
BUILD_DATE="${BUILD_DATE:-$(date -u +"%Y-%m-%dT%H:%M:%SZ")}"

LDFLAGS="-s -w -X github.com/gosuda/maek/internal/version.Version=${VERSION} -X github.com/gosuda/maek/internal/version.Commit=${COMMIT} -X github.com/gosuda/maek/internal/version.Date=${BUILD_DATE}"

echo "==> Building embedded client binaries (v${VERSION})..."

TMP_BUILD="$(mktemp -d 2>/dev/null || mktemp -d -t 'maek-embed')"
PKG_DIR="${TMP_BUILD}/pkg"
mkdir -p "$PKG_DIR"
trap 'rm -rf "$TMP_BUILD"' EXIT

# Compile platforms in parallel
build_target() {
    local goos="$1"
    local goarch="$2"
    local os_label="$3"
    local arch_label="$4"
    local ext="${5:-}"

    local bin_name="maek${ext}"
    local target_tmp="${TMP_BUILD}/${os_label}_${arch_label}"
    mkdir -p "$target_tmp"

    CGO_ENABLED=0 GOOS="$goos" GOARCH="$goarch" go build -ldflags="$LDFLAGS" -o "${target_tmp}/${bin_name}" ./cmd/maek

    if [ "$goos" = "windows" ]; then
        (cd "$target_tmp" && zip -q -9 "${PKG_DIR}/maek_${os_label}_${arch_label}.zip" "${bin_name}")
    else
        tar -czf "${PKG_DIR}/maek_${os_label}_${arch_label}.tar.gz" -C "$target_tmp" "${bin_name}"
    fi
}

build_target darwin arm64 Darwin arm64 &
build_target darwin amd64 Darwin x86_64 &
build_target linux  arm64 Linux  arm64 &
build_target linux  amd64 Linux  x86_64 &
build_target windows amd64 Windows x86_64 .exe &

wait

# Move all packaged archives into the embed directory
mv "${PKG_DIR}/maek_"* "${OUT_DIR}/"

echo "==> Embedded binaries prepared successfully in ${OUT_DIR}:"
ls -lh "$OUT_DIR"
