#!/usr/bin/env bash
# Stage a kapi-av plugin bundle for one platform: kapi-av binary + manifest +
# NOTICE + bundled ffmpeg/ffprobe (+ their dylibs). The release matrix runs this
# per target with an LGPL ffmpeg build (--disable-gpl --disable-nonfree) and
# Developer-ID signing; locally it copies the system ffmpeg/ffprobe to prove the
# discovery + in-process resolution wiring.
#
# Usage: stage-plugin.sh <out-dir> [ffmpeg-path] [ffprobe-path]
set -euo pipefail
here="$(cd "$(dirname "$0")/.." && pwd)"   # plugins/av
out="${1:?usage: stage-plugin.sh <out-dir> [ffmpeg] [ffprobe]}"
ffmpeg_bin="${2:-$(command -v ffmpeg || true)}"
ffprobe_bin="${3:-$(command -v ffprobe || true)}"
mkdir -p "$out"

echo "==> build kapi-av"
( cd "$here" && GOWORK=off go build -o "$out/kapi-av" ./cmd/kapi-av )
cp "$here/manifest.json" "$out/manifest.json"
cp "$here/NOTICE" "$out/NOTICE"

for tool in ffmpeg ffprobe; do
  src="$([ "$tool" = ffmpeg ] && echo "$ffmpeg_bin" || echo "$ffprobe_bin")"
  if [ -z "$src" ] || [ ! -x "$src" ]; then
    echo "error: $tool not found (pass a path, or install ffmpeg)" >&2; exit 1
  fi
  cp "$src" "$out/$tool"; chmod +w "$out/$tool"
done

# Bundle dynamic libs + fix rpaths so the bundle is self-contained.
case "$(uname -s)" in
  Darwin)
    for tool in ffmpeg ffprobe; do
      for lib in $(otool -L "$out/$tool" 2>/dev/null | awk 'NR>1{print $1}' | grep -E '/(opt|Cellar)/' || true); do
        base="$(basename "$lib")"
        [ -e "$out/$base" ] || cp -p "$lib" "$out/" 2>/dev/null || true
        install_name_tool -change "$lib" "@loader_path/$base" "$out/$tool" 2>/dev/null || true
      done
    done
    chmod +w "$out"/*.dylib 2>/dev/null || true
    if command -v codesign >/dev/null; then
      for f in "$out"/*.dylib "$out/ffmpeg" "$out/ffprobe"; do
        [ -e "$f" ] && codesign --force --sign - "$f" 2>/dev/null || true
      done
    fi
    ;;
  Linux)
    command -v patchelf >/dev/null && { patchelf --set-rpath '$ORIGIN' "$out/ffmpeg" 2>/dev/null || true; patchelf --set-rpath '$ORIGIN' "$out/ffprobe" 2>/dev/null || true; }
    ;;
esac
echo "==> staged kapi-av plugin at $out"; ls "$out"
