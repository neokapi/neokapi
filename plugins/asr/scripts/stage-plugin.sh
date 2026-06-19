#!/usr/bin/env bash
# Stage a kapi-asr plugin bundle for one platform: the plugin binary + manifest +
# NOTICE + the bundled whisper-cli + a default ggml model — the exact tarball
# layout the host's plugin discovery expects (<dir>/manifest.json, <dir>/kapi-asr,
# <dir>/whisper-cli, <dir>/ggml-*.bin). The release matrix runs this per target
# with a per-platform whisper-cli; locally it copies whisper-cli from PATH.
#
# Usage: stage-plugin.sh <out-dir> [whisper-cli-path] [model-path]
set -euo pipefail

here="$(cd "$(dirname "$0")/.." && pwd)"   # plugins/asr
out="${1:?usage: stage-plugin.sh <out-dir> [whisper-cli] [model]}"
whisper_bin="${2:-$(command -v whisper-cli || true)}"
model="${3:-}"
model_url="https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-base.en.bin"

mkdir -p "$out"

echo "==> build kapi-asr"
( cd "$here" && GOWORK=off go build -o "$out/kapi-asr" ./cmd/kapi-asr )

echo "==> manifest + NOTICE"
cp "$here/manifest.json" "$out/manifest.json"
cp "$here/NOTICE" "$out/NOTICE"

echo "==> bundle whisper-cli"
if [ -z "$whisper_bin" ] || [ ! -x "$whisper_bin" ]; then
  echo "error: whisper-cli not found (pass a path, or install whisper-cpp)" >&2
  exit 1
fi
cp "$whisper_bin" "$out/whisper-cli"
chmod +w "$out/whisper-cli"
# Bundle whisper-cli's dynamic libraries into the plugin dir so the bundle is
# self-contained (no dependency on a system whisper install). Per-platform:
case "$(uname -s)" in
  Darwin)
    # whisper-cli resolves @rpath/lib*.dylib; copy the whisper/ggml dylibs beside
    # it and add @loader_path to the rpath so it finds them in its own dir.
    libdir="$(dirname "$whisper_bin")/../lib"
    for lib in "$libdir"/libwhisper*.dylib "$libdir"/libggml*.dylib; do
      [ -e "$lib" ] && cp -p "$lib" "$out/"
    done
    chmod +w "$out"/*.dylib 2>/dev/null || true
    install_name_tool -add_rpath @loader_path "$out/whisper-cli" 2>/dev/null || true
    # install_name_tool invalidates the code signature; on Apple Silicon an
    # unsigned/broken binary is SIGKILLed at launch, so ad-hoc re-sign. (The
    # release matrix re-signs with the Developer ID instead.)
    if command -v codesign >/dev/null; then
      for f in "$out"/*.dylib "$out/whisper-cli"; do
        codesign --force --sign - "$f" 2>/dev/null || true
      done
    fi
    ;;
  Linux)
    # On Linux, bundle the .so files; the release wrapper sets an $ORIGIN rpath.
    libdir="$(dirname "$whisper_bin")/../lib"
    for lib in "$libdir"/libwhisper*.so* "$libdir"/libggml*.so*; do
      [ -e "$lib" ] && cp -p "$lib" "$out/"
    done
    command -v patchelf >/dev/null && patchelf --set-rpath '$ORIGIN' "$out/whisper-cli" || true
    ;;
esac

echo "==> bundle model"
if [ -n "$model" ]; then
  cp "$model" "$out/ggml-base.en.bin"
elif [ ! -f "$out/ggml-base.en.bin" ]; then
  echo "    downloading ggml-base.en.bin"
  curl -fsSL -o "$out/ggml-base.en.bin" "$model_url"
fi

echo "==> staged kapi-asr plugin at $out"
ls -la "$out"
