#!/usr/bin/env bash
#
# Regenerate every neokapi-branded logo asset from the two-background source pair.
#
# The neokapi brand mark (the tapir hugging a globe) lives at:
#   web/assets/neokapi-logo-2-black.png   (logo over solid black)
#   web/assets/neokapi-logo-2-white.png   (logo over solid white)
#
# This script is fully deterministic -- no AI, no manual editing. Re-run it after
# dropping in an updated source pair and every derived asset is rebuilt the same
# way.
#
#   Phase A  combine the black/white renders into one transparent master and
#            strip the generator's corner watermark   (scripts/lib/logo_matte.py)
#   Phase B  fan the master out to every place neokapi uses a logo/icon/favicon
#
# Bowrain is a SEPARATE brand (its own mark) -- this script never touches
# bowrain/. For bowrain icons see scripts/generate-icons.sh.
#
# Generated (all transparent unless noted):
#   master:
#     web/assets/neokapi-logo.png                       (1024, transparent)
#   docs site (Docusaurus, web/static/img/):
#     favicon.png (32)  favicon.ico (16/32/48)  logo.png (256, navbar)
#     hero-logo.png (512, og:image)  apple-touch-icon.png (180)
#   Kapi desktop (apps/kapi-desktop/):
#     build/appicon.png (1024)  build/darwin/icons.icns  build/windows/icon.ico
#     frontend/public/neokapi-logo.png (512, in-app logo)
#   demo-video harness (used as mascot.png on the intro/outro cards):
#     harness/assets/mascot.png (512)  harness/public/mascot.png (512)
#
# macOS app icons (appicon.png + icons.icns) get the standard squircle mask with
# padding so they sit correctly in the Dock. Pass --no-squircle for full-bleed.
#
# Requires: ImageMagick (magick), Python 3 + Pillow + numpy, iconutil & sips (macOS).

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
ASSETS="$REPO_ROOT/web/assets"

BLACK="$ASSETS/neokapi-logo-2-black.png"
WHITE="$ASSETS/neokapi-logo-2-white.png"
MASTER="$ASSETS/neokapi-logo.png"
SQUIRCLE=1
CORNER="0.85"

usage() {
  sed -n '2,40p' "$0" | sed 's/^#\s\?//'
  exit "${1:-0}"
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --black)        BLACK="$2"; shift 2 ;;
    --white)        WHITE="$2"; shift 2 ;;
    --master)       MASTER="$2"; shift 2 ;;
    --corner)       CORNER="$2"; shift 2 ;;
    --no-squircle)  SQUIRCLE=0; shift ;;
    -h|--help)      usage 0 ;;
    *) echo "unknown argument: $1" >&2; usage 1 ;;
  esac
done

for f in "$BLACK" "$WHITE"; do
  [[ -f "$f" ]] || { echo "Error: source not found: $f" >&2; exit 1; }
done
command -v magick   >/dev/null || { echo "Error: ImageMagick (magick) is required" >&2; exit 1; }
command -v iconutil >/dev/null || { echo "Error: iconutil (macOS) is required for .icns" >&2; exit 1; }

TMPDIR="$(mktemp -d)"
trap 'rm -rf "$TMPDIR"' EXIT

# ----------------------------------------------------------------------------
# Phase A -- combine black+white into one transparent, watermark-free master.
# ----------------------------------------------------------------------------
echo "==> Phase A: matte extraction"
python3 "$REPO_ROOT/scripts/lib/logo_matte.py" --corner "$CORNER" "$BLACK" "$WHITE" "$MASTER"

# ----------------------------------------------------------------------------
# Phase B -- fan the master out to every neokapi asset.
# ----------------------------------------------------------------------------
echo "==> Phase B: deriving assets"

# Resize the (transparent) master to a square PNG, preserving alpha.
resize() {
  local size="$1" output="$2"
  mkdir -p "$(dirname "$output")"
  magick "$MASTER" -resize "${size}x${size}" -background none -strip "$output"
  echo "  $output (${size})"
}

# Multi-resolution favicon.ico (16/32/48).
make_favicon() {
  local output="$1"
  mkdir -p "$(dirname "$output")"
  resize 16 "$TMPDIR/fav-16.png"
  resize 32 "$TMPDIR/fav-32.png"
  resize 48 "$TMPDIR/fav-48.png"
  magick "$TMPDIR/fav-16.png" "$TMPDIR/fav-32.png" "$TMPDIR/fav-48.png" "$output"
  echo "  $output (16/32/48)"
}

# A single app-icon tile at the given canvas size. With the squircle mask the
# artwork sits at 80% on a continuous-corner rounded square (macOS convention);
# without it the artwork is full-bleed.
app_tile() {
  local canvas="$1" output="$2"
  if [[ "$SQUIRCLE" -eq 0 ]]; then
    magick "$MASTER" -resize "${canvas}x${canvas}" -background none -strip "$output"
    return
  fi
  local body=$(( canvas * 80 / 100 ))
  local offset=$(( (canvas - body) / 2 ))
  local radius=$(( body * 225 / 1000 ))   # ~22.5% continuous-corner radius
  magick "$MASTER" -resize "${body}x${body}" -background none -strip "$TMPDIR/tile-body.png"
  magick -size "${body}x${body}" xc:none \
    -fill white -draw "roundrectangle 0,0 $((body - 1)),$((body - 1)) ${radius},${radius}" \
    "$TMPDIR/tile-mask.png"
  magick "$TMPDIR/tile-body.png" "$TMPDIR/tile-mask.png" -compose DstIn -composite "$TMPDIR/tile-masked.png"
  magick -size "${canvas}x${canvas}" xc:none \
    "$TMPDIR/tile-masked.png" -geometry "+${offset}+${offset}" -composite "$output"
}

# --- Docs site (Docusaurus) ---------------------------------------------------
echo "docs site (web/static/img):"
DOCS_IMG="$REPO_ROOT/web/static/img"
resize 32  "$DOCS_IMG/favicon.png"
make_favicon "$DOCS_IMG/favicon.ico"
resize 256 "$DOCS_IMG/logo.png"
resize 512 "$DOCS_IMG/hero-logo.png"
resize 180 "$DOCS_IMG/apple-touch-icon.png"

# --- Kapi desktop app ---------------------------------------------------------
echo "kapi-desktop (apps/kapi-desktop):"
KD="$REPO_ROOT/apps/kapi-desktop"
mkdir -p "$KD/build/darwin" "$KD/build/windows"

echo "  appicon.png (1024$([[ $SQUIRCLE -eq 1 ]] && echo ', squircle'))"
app_tile 1024 "$KD/build/appicon.png"

echo "  build/darwin/icons.icns"
ICONSET="$TMPDIR/neokapi.iconset"
mkdir -p "$ICONSET"
app_tile 16   "$ICONSET/icon_16x16.png"
app_tile 32   "$ICONSET/icon_16x16@2x.png"
app_tile 32   "$ICONSET/icon_32x32.png"
app_tile 64   "$ICONSET/icon_32x32@2x.png"
app_tile 128  "$ICONSET/icon_128x128.png"
app_tile 256  "$ICONSET/icon_128x128@2x.png"
app_tile 256  "$ICONSET/icon_256x256.png"
app_tile 512  "$ICONSET/icon_256x256@2x.png"
app_tile 512  "$ICONSET/icon_512x512.png"
app_tile 1024 "$ICONSET/icon_512x512@2x.png"
iconutil --convert icns --output "$KD/build/darwin/icons.icns" "$ICONSET"

echo "  build/windows/icon.ico (16/32/48/64/128/256)"
WIN_PNGS=()
for s in 16 32 48 64 128 256; do
  p="$TMPDIR/win-${s}.png"
  magick "$MASTER" -resize "${s}x${s}" -background none -strip "$p"
  WIN_PNGS+=("$p")
done
magick "${WIN_PNGS[@]}" "$KD/build/windows/icon.ico"

resize 512 "$KD/frontend/public/neokapi-logo.png"

# --- Demo-video harness (mascot on the intro/outro cards) ---------------------
echo "demo harness (harness mascot):"
resize 512 "$REPO_ROOT/harness/assets/mascot.png"
resize 512 "$REPO_ROOT/harness/public/mascot.png"

echo "==> Done. Master: $MASTER"
echo "    Re-render demo videos (make harness-videos) to pick up the new mascot."
