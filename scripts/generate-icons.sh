#!/usr/bin/env bash
#
# Generate all Bowrain app icons from a single source PNG.
#
# Usage: ./scripts/generate-icons.sh <input.png>
#
# Requires: ImageMagick (magick), iconutil (macOS), sips (macOS)
#
# macOS icons get the standard squircle mask with padding so they sit
# correctly alongside other app icons in the Dock and App Switcher.
#
# Generates:
#   Desktop app:
#     bowrain/apps/bowrain/build/appicon.png          (1024x1024, macOS squircle)
#     bowrain/apps/bowrain/build/darwin/icons.icns     (macOS ICNS, squircle)
#     bowrain/apps/bowrain/build/windows/icon.ico      (16-256px multi-size)
#     bowrain/apps/bowrain/frontend/public/favicon.ico (16,32,48px)
#     bowrain/apps/bowrain/frontend/public/apple-touch-icon.png (180x180)
#   Web app:
#     bowrain/apps/web/public/favicon.ico              (16,32,48px)
#     bowrain/apps/web/public/apple-touch-icon.png     (180x180)
#     bowrain/apps/web/public/icon-192.png             (192x192)
#     bowrain/apps/web/public/icon-512.png             (512x512)
#   Kapi web:
#     kapi/apps/kapi-web/public/favicon.ico         (16,32,48px)
#     kapi/apps/kapi-web/public/apple-touch-icon.png (180x180)

set -euo pipefail

if [[ $# -ne 1 ]]; then
  echo "Usage: $0 <input.png>" >&2
  exit 1
fi

INPUT="$1"

if [[ ! -f "$INPUT" ]]; then
  echo "Error: file not found: $INPUT" >&2
  exit 1
fi

# Resolve repo root (script lives in scripts/).
REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"

# Verify input is at least 1024x1024.
WIDTH=$(sips --getProperty pixelWidth "$INPUT" | awk '/pixelWidth/{print $2}')
HEIGHT=$(sips --getProperty pixelHeight "$INPUT" | awk '/pixelHeight/{print $2}')
if [[ "$WIDTH" -lt 1024 || "$HEIGHT" -lt 1024 ]]; then
  echo "Error: input image must be at least 1024x1024 (got ${WIDTH}x${HEIGHT})" >&2
  exit 1
fi

TMPDIR="$(mktemp -d)"
trap 'rm -rf "$TMPDIR"' EXIT

echo "Generating icons from: $INPUT"

# --- Helper: resize to PNG (full-bleed, no mask) ---
resize() {
  local size="$1" output="$2"
  magick "$INPUT" -resize "${size}x${size}" -strip "$output"
}

# --- Helper: generate favicon.ico (16, 32, 48) ---
make_favicon() {
  local output="$1"
  local ico16="$TMPDIR/ico-16.png"
  local ico32="$TMPDIR/ico-32.png"
  local ico48="$TMPDIR/ico-48.png"
  resize 16 "$ico16"
  resize 32 "$ico32"
  resize 48 "$ico48"
  magick "$ico16" "$ico32" "$ico48" "$output"
}

# --- Helper: create macOS squircle-masked icon at a given canvas size ---
#
# Apple's macOS icon grid places the artwork at ~80% of the canvas with a
# continuous-corner (squircle) mask. At 1024x1024 the content area is ~824x824
# centered with ~100px padding on each side, corner radius ~185px.
#
# For smaller sizes the same proportions apply (80% content, 22.5% radius).
macos_icon() {
  local canvas="$1" output="$2"

  # Content area = 80% of canvas, padding = 10% each side.
  local body=$(( canvas * 80 / 100 ))
  local offset=$(( (canvas - body) / 2 ))
  # Corner radius ~22.5% of body size.
  local radius=$(( body * 225 / 1000 ))

  # Resize artwork to fill the body area.
  local body_png="$TMPDIR/macos-body-${canvas}.png"
  magick "$INPUT" -resize "${body}x${body}" -strip "$body_png"

  # Create the rounded-rect mask.
  local mask_png="$TMPDIR/macos-mask-${canvas}.png"
  magick -size "${body}x${body}" xc:none \
    -fill white -draw "roundrectangle 0,0 $((body - 1)),$((body - 1)) ${radius},${radius}" \
    "$mask_png"

  # Apply mask to artwork.
  local masked_png="$TMPDIR/macos-masked-${canvas}.png"
  magick "$body_png" "$mask_png" -compose DstIn -composite "$masked_png"

  # Center on transparent canvas.
  magick -size "${canvas}x${canvas}" xc:none \
    "$masked_png" -geometry "+${offset}+${offset}" \
    -composite "$output"
}

# --- 1. Desktop app: appicon.png (1024x1024, macOS squircle) ---
echo "  appicon.png (1024x1024, macOS squircle)"
macos_icon 1024 "$REPO_ROOT/bowrain/apps/bowrain/build/appicon.png"

# --- 2. Desktop app: macOS ICNS ---
echo "  darwin/icons.icns"
ICONSET="$TMPDIR/icons.iconset"
mkdir -p "$ICONSET"
# Standard macOS iconset sizes (including @2x variants).
# Each size gets the squircle mask at proper proportions.
macos_icon 16   "$ICONSET/icon_16x16.png"
macos_icon 32   "$ICONSET/icon_16x16@2x.png"
macos_icon 32   "$ICONSET/icon_32x32.png"
macos_icon 64   "$ICONSET/icon_32x32@2x.png"
macos_icon 128  "$ICONSET/icon_128x128.png"
macos_icon 256  "$ICONSET/icon_128x128@2x.png"
macos_icon 256  "$ICONSET/icon_256x256.png"
macos_icon 512  "$ICONSET/icon_256x256@2x.png"
macos_icon 512  "$ICONSET/icon_512x512.png"
macos_icon 1024 "$ICONSET/icon_512x512@2x.png"
iconutil --convert icns --output "$REPO_ROOT/bowrain/apps/bowrain/build/darwin/icons.icns" "$ICONSET"

# --- 3. Desktop app: Windows ICO (16, 32, 48, 64, 128, 256) ---
echo "  windows/icon.ico"
WIN_SIZES=(16 32 48 64 128 256)
WIN_PNGS=()
for s in "${WIN_SIZES[@]}"; do
  p="$TMPDIR/win-${s}.png"
  resize "$s" "$p"
  WIN_PNGS+=("$p")
done
magick "${WIN_PNGS[@]}" "$REPO_ROOT/bowrain/apps/bowrain/build/windows/icon.ico"

# --- 4. Desktop frontend: favicon.ico + apple-touch-icon ---
echo "  desktop frontend icons"
make_favicon "$REPO_ROOT/bowrain/apps/bowrain/frontend/public/favicon.ico"
resize 180 "$REPO_ROOT/bowrain/apps/bowrain/frontend/public/apple-touch-icon.png"

# --- 5. Web app: favicon.ico + apple-touch-icon + PWA icons ---
echo "  web app icons"
make_favicon "$REPO_ROOT/bowrain/apps/web/public/favicon.ico"
resize 180 "$REPO_ROOT/bowrain/apps/web/public/apple-touch-icon.png"
resize 192 "$REPO_ROOT/bowrain/apps/web/public/icon-192.png"
resize 512 "$REPO_ROOT/bowrain/apps/web/public/icon-512.png"

# --- 6. Kapi web: favicon.ico + apple-touch-icon ---
echo "  kapi-web icons"
make_favicon "$REPO_ROOT/kapi/apps/kapi-web/public/favicon.ico"
resize 180 "$REPO_ROOT/kapi/apps/kapi-web/public/apple-touch-icon.png"

echo "Done. Generated all icons."
