#!/usr/bin/env bash
# Build + package one platform's kapi-av plugin tarball for distribution via the
# neokapi/registry (installable with `kapi plugins install av`).
#
# kapi-av is a pure-Go plugin that bundles an LGPL ffmpeg/ffprobe (configured
# --disable-gpl --disable-nonfree, separate executables, never linked into kapi)
# so the host's in-process video demux (core/av) can find them without a system
# install. This script assembles a self-contained tarball; the per-platform
# ffmpeg + ffprobe (+ any shared libs) are produced by the release workflow
# (BtbN static LGPL prebuilt on linux/windows, built from source on macOS) and
# handed in via --ffmpeg-dir.
#
# Tarball layout (kapi-av_<version>_<os>_<arch>.tar.gz), flat at the root so the
# binary self-resolves ffmpeg/ffprobe from beside itself (cmd/kapi-av: bundled):
#
#     kapi-av[.exe]        the plugin binary
#     manifest.json        the plugin manifest
#     NOTICE               FFmpeg LGPL notice
#     ffmpeg[.exe]         the bundled LGPL ffmpeg
#     ffprobe[.exe]        the bundled LGPL ffprobe
#     <shared libs>        any runtime libraries (static builds bundle none)
#
# Usage (flags or env; flags win):
#   scripts/package-av-plugin.sh \
#     --version 0.1.0 \
#     --ffmpeg-dir /path/with/ffmpeg+ffprobe \
#     --out-dir /path/to/output
set -euo pipefail

VERSION="${VERSION:-}"; FFMPEG_DIR="${FFMPEG_DIR:-}"; OUT_DIR="${OUT_DIR:-}"
# Optional: an already-built (and, on macOS, Developer-ID-signed + notarized)
# kapi-av binary to bundle as-is instead of building one here. See release-av.yml.
PREBUILT_BIN="${PREBUILT_BIN:-}"
while [ $# -gt 0 ]; do
  case "$1" in
    --version) VERSION="$2"; shift 2 ;;
    --ffmpeg-dir) FFMPEG_DIR="$2"; shift 2 ;;
    --out-dir) OUT_DIR="$2"; shift 2 ;;
    --prebuilt-bin) PREBUILT_BIN="$2"; shift 2 ;;
    *) echo "unknown arg: $1" >&2; exit 2 ;;
  esac
done
: "${VERSION:?--version required}"; : "${FFMPEG_DIR:?--ffmpeg-dir required}"; : "${OUT_DIR:?--out-dir required}"

# On Windows the inputs arrive as Windows-style paths (set by pwsh steps);
# normalize them for git-bash so cp/tar resolve them.
if command -v cygpath >/dev/null 2>&1; then
  FFMPEG_DIR=$(cygpath -u "$FFMPEG_DIR")
  OUT_DIR=$(cygpath -u "$OUT_DIR")
fi

repo="$(cd "$(dirname "$0")/.." && pwd)"
goos="$(cd "$repo/plugins/av" && GOWORK=off go env GOOS)"
goarch="$(cd "$repo/plugins/av" && GOWORK=off go env GOARCH)"
exe=""; [ "$goos" = windows ] && exe=".exe"

stage="$(mktemp -d)/kapi-av"; mkdir -p "$stage"

if [ -n "$PREBUILT_BIN" ]; then
  echo "==> use prebuilt (pre-signed) kapi-av: $PREBUILT_BIN"
  cp "$PREBUILT_BIN" "$stage/kapi-av$exe"
else
  echo "==> build kapi-av ($goos/$goarch)"
  ( cd "$repo/plugins/av" && GOWORK=off CGO_ENABLED=0 go build -trimpath \
      -ldflags "-s -w" -o "$stage/kapi-av$exe" ./cmd/kapi-av )
fi

cp "$repo/plugins/av/manifest.json" "$stage/manifest.json"
# Stamp the release version into the staged manifest so the published plugin
# version matches its tag (manifest.json's source value is a dev default). Only
# the top-level "version" (2-space indent) is rewritten, not nested model versions.
sed 's/^  "version": *"[^"]*"/  "version": "'"$VERSION"'"/' "$stage/manifest.json" > "$stage/manifest.json.tmp" \
  && mv "$stage/manifest.json.tmp" "$stage/manifest.json"
cp "$repo/plugins/av/NOTICE" "$stage/NOTICE"

echo "==> bundle ffmpeg + ffprobe from $FFMPEG_DIR"
for tool in ffmpeg ffprobe; do
  src="$FFMPEG_DIR/$tool$exe"
  [ -f "$src" ] || { echo "::error::$tool$exe not found in $FFMPEG_DIR"; ls -la "$FFMPEG_DIR"; exit 1; }
  cp "$src" "$stage/$tool$exe"; chmod +w "$stage/$tool$exe" 2>/dev/null || true
done
# Copy any runtime libraries beside the binaries (static builds bundle none; the
# workflow has already fixed rpaths/signing so they load from @loader_path /
# $ORIGIN / the same dir).
shopt -s nullglob
for lib in "$FFMPEG_DIR"/*.dylib "$FFMPEG_DIR"/*.so* "$FFMPEG_DIR"/*.dll; do
  cp -p "$lib" "$stage/"
done
shopt -u nullglob

mkdir -p "$OUT_DIR"
name="kapi-av_${VERSION}_${goos}_${goarch}.tar.gz"
tarball="$OUT_DIR/$name"
echo "==> tar $name"
tar -C "$(dirname "$stage")" -czf "$tarball" "$(basename "$stage")"

if command -v sha256sum >/dev/null; then sha=$(sha256sum "$tarball" | awk '{print $1}'); else sha=$(shasum -a 256 "$tarball" | awk '{print $1}'); fi
echo "==> $name  sha256=$sha"

if [ -n "${GITHUB_OUTPUT:-}" ]; then
  {
    echo "tarball=$tarball"
    echo "tarball_name=$name"
    echo "sha256=$sha"
  } >> "$GITHUB_OUTPUT"
fi
