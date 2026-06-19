#!/usr/bin/env bash
# Build + package one platform's kapi-asr plugin tarball for distribution via the
# neokapi/registry (installable with `kapi plugins install asr`).
#
# kapi-asr is a pure-Go plugin that execs a bundled whisper.cpp `whisper-cli`
# (MIT). This script assembles a self-contained tarball; the per-platform
# whisper-cli + its shared libraries are produced by the release workflow (built
# from source on macOS/windows-arm64, downloaded prebuilt on linux/windows-amd64)
# and handed in via --whisper-dir.
#
# Tarball layout (kapi-asr_<version>_<os>_<arch>.tar.gz), flat at the root so the
# binary self-resolves whisper-cli + model from beside itself (cmd/kapi-asr:
# resolveBin/resolveModel):
#
#     kapi-asr[.exe]        the plugin binary
#     manifest.json         the plugin manifest
#     NOTICE                third-party (whisper.cpp/ggml/model MIT) notices
#     whisper-cli[.exe]     the bundled whisper.cpp CLI
#     <shared libs>         whisper-cli's runtime libraries (libwhisper, libggml…)
#     ggml-*.bin            the default model
#
# Usage (flags or env; flags win):
#   scripts/package-asr-plugin.sh \
#     --version 0.1.0 \
#     --whisper-dir /path/with/whisper-cli+libs \
#     --model /path/to/ggml-base.en.bin \
#     --out-dir /path/to/output
set -euo pipefail

VERSION="${VERSION:-}"; WHISPER_DIR="${WHISPER_DIR:-}"; MODEL="${MODEL:-}"; OUT_DIR="${OUT_DIR:-}"
# Optional: an already-built (and, on macOS, Developer-ID-signed + notarized)
# kapi-asr binary to bundle as-is instead of building one here. The macOS release
# leg pre-signs the binary before packaging so the bundled binary carries the
# notarized signature; see release-asr.yml.
PREBUILT_BIN="${PREBUILT_BIN:-}"
while [ $# -gt 0 ]; do
  case "$1" in
    --version) VERSION="$2"; shift 2 ;;
    --whisper-dir) WHISPER_DIR="$2"; shift 2 ;;
    --model) MODEL="$2"; shift 2 ;;
    --out-dir) OUT_DIR="$2"; shift 2 ;;
    --prebuilt-bin) PREBUILT_BIN="$2"; shift 2 ;;
    *) echo "unknown arg: $1" >&2; exit 2 ;;
  esac
done
: "${VERSION:?--version required}"; : "${WHISPER_DIR:?--whisper-dir required}"; : "${OUT_DIR:?--out-dir required}"

# On Windows the inputs arrive as Windows-style paths (set by pwsh/msys2 steps);
# normalize them for git-bash so cp/tar resolve them.
if command -v cygpath >/dev/null 2>&1; then
  WHISPER_DIR=$(cygpath -u "$WHISPER_DIR")
  OUT_DIR=$(cygpath -u "$OUT_DIR")
  [ -n "$MODEL" ] && MODEL=$(cygpath -u "$MODEL")
fi

repo="$(cd "$(dirname "$0")/.." && pwd)"
goos="$(cd "$repo/plugins/asr" && GOWORK=off go env GOOS)"
goarch="$(cd "$repo/plugins/asr" && GOWORK=off go env GOARCH)"
exe=""; [ "$goos" = windows ] && exe=".exe"

stage="$(mktemp -d)/kapi-asr"; mkdir -p "$stage"

if [ -n "$PREBUILT_BIN" ]; then
  echo "==> use prebuilt (pre-signed) kapi-asr: $PREBUILT_BIN"
  cp "$PREBUILT_BIN" "$stage/kapi-asr$exe"
else
  echo "==> build kapi-asr ($goos/$goarch)"
  ( cd "$repo/plugins/asr" && GOWORK=off CGO_ENABLED=0 go build -trimpath \
      -ldflags "-s -w" -o "$stage/kapi-asr$exe" ./cmd/kapi-asr )
fi

cp "$repo/plugins/asr/manifest.json" "$stage/manifest.json"
cp "$repo/plugins/asr/NOTICE" "$stage/NOTICE"

echo "==> bundle whisper-cli + libs from $WHISPER_DIR"
cp "$WHISPER_DIR/whisper-cli$exe" "$stage/whisper-cli$exe"
# Copy the runtime libraries beside the binary (the workflow has already fixed
# rpaths/signing so they load from @loader_path / $ORIGIN / the same dir).
shopt -s nullglob
for lib in "$WHISPER_DIR"/*.dylib "$WHISPER_DIR"/*.so* "$WHISPER_DIR"/*.dll; do
  cp -p "$lib" "$stage/"
done
shopt -u nullglob

if [ -n "$MODEL" ]; then
  cp "$MODEL" "$stage/$(basename "$MODEL")"
fi

mkdir -p "$OUT_DIR"
name="kapi-asr_${VERSION}_${goos}_${goarch}.tar.gz"
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
