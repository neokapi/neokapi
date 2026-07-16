#!/usr/bin/env bash
#
# Regenerate every bowrain-branded asset from the canonical vector mark.
#
# The bowrain mark (quarter-circle rainbow) lives at:
#   bowrain/assets/brand/mark.svg
#
# This script is fully deterministic — drop in an updated mark.svg and re-run;
# every derived asset is rebuilt the same way. (Bowrain is a separate brand
# from neokapi/kapi — for kapi assets see scripts/generate-neokapi-logo.sh.)
#
# Requires: rsvg-convert (librsvg), ImageMagick (magick), iconutil (macOS).
#
# Generated:
#   masters:
#     bowrain/assets/brand/mark-1024.png                (transparent, full-bleed)
#   landing (bowrain/web/landing/public/):
#     favicon.svg  favicon.ico (16/32/48)  og.png (1200x630)
#   web app (bowrain/apps/web/public/):
#     favicon.ico  apple-touch-icon.png (180, tile)  icon-192.png  icon-512.png
#   desktop app (bowrain/apps/bowrain/):
#     build/appicon.png (1024, macOS squircle)  build/darwin/icons.icns
#     build/windows/icon.ico  frontend/public/favicon.ico
#     frontend/public/apple-touch-icon.png (180, tile)
#   docs site (bowrain/web/docs/static/img/):
#     logo.png (256)  favicon.png (32)  favicon.ico  hero-logo.png (512, tile)
#     apple-touch-icon.png (180, tile)
#   keycloak theme (bowrain/apps/keycloak-theme/):
#     public/logo.png (512)  src/login/assets/logo.png (512)

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
MARK="$REPO_ROOT/bowrain/assets/brand/mark.svg"
[[ -f "$MARK" ]] || { echo "Error: $MARK not found" >&2; exit 1; }

# Solid tile background for surfaces that need one (app icons, touch icons).
# Rainlight light background (oklch(0.985 0.003 240)).
TILE_BG="#f8fafc"

TMPDIR="$(mktemp -d)"
trap 'rm -rf "$TMPDIR"' EXIT

echo "Generating bowrain brand assets from: $MARK"

# --- Masters ---------------------------------------------------------------
MASTER="$REPO_ROOT/bowrain/assets/brand/mark-1024.png"
rsvg-convert -w 1024 -h 1024 "$MARK" -o "$MASTER"

# Tile master: mark at 72% centered on a solid tile (for app/touch icons).
TILE="$TMPDIR/tile-1024.png"
rsvg-convert -w 738 -h 738 "$MARK" -o "$TMPDIR/mark-738.png"
magick -size 1024x1024 "xc:$TILE_BG" "$TMPDIR/mark-738.png" -gravity center -composite "$TILE"

resize() { # <source> <size> <output>
  magick "$1" -resize "$2x$2" -strip "$3"
}

make_favicon() { # <source> <output.ico>
  local src="$1" out="$2"
  for s in 16 32 48; do resize "$src" "$s" "$TMPDIR/ico-$s.png"; done
  magick "$TMPDIR/ico-16.png" "$TMPDIR/ico-32.png" "$TMPDIR/ico-48.png" "$out"
}

# macOS squircle icon (80% content, 22.5% corner radius), from the solid tile.
macos_icon() { # <canvas> <output>
  local canvas="$1" output="$2"
  local body=$(( canvas * 80 / 100 ))
  local offset=$(( (canvas - body) / 2 ))
  local radius=$(( body * 225 / 1000 ))
  magick "$TILE" -resize "${body}x${body}" -strip "$TMPDIR/sq-body.png"
  magick -size "${body}x${body}" xc:none \
    -fill white -draw "roundrectangle 0,0 $((body - 1)),$((body - 1)) ${radius},${radius}" \
    "$TMPDIR/sq-mask.png"
  magick "$TMPDIR/sq-body.png" "$TMPDIR/sq-mask.png" -compose DstIn -composite "$TMPDIR/sq-masked.png"
  magick -size "${canvas}x${canvas}" xc:none \
    "$TMPDIR/sq-masked.png" -geometry "+${offset}+${offset}" -composite "$output"
}

# --- Landing ---------------------------------------------------------------
echo "  landing: favicon.svg favicon.ico og.png"
LANDING="$REPO_ROOT/bowrain/web/landing/public"
cp "$MARK" "$LANDING/favicon.svg"
make_favicon "$MASTER" "$LANDING/favicon.ico"
# OG card: tile background, mark left, wordmark + tagline. Text is rendered
# by librsvg (fontconfig system fonts), not ImageMagick, which ships fontless.
# The mark is inlined (librsvg refuses external file refs): strip mark.svg's
# outer <svg> wrapper and place its content in a scaled group (360/64 = 5.625).
MARK_INNER="$(sed -e '/<svg/d' -e '/<\/svg>/d' -e '/<!--/,/-->/d' "$MARK")"
cat > "$TMPDIR/og.svg" <<EOF
<svg xmlns="http://www.w3.org/2000/svg" width="1200" height="630" viewBox="0 0 1200 630">
  <rect width="1200" height="630" fill="$TILE_BG"/>
  <g transform="translate(100,135) scale(5.625)">
    $MARK_INNER
  </g>
  <text x="530" y="330" font-family="Helvetica Neue, Helvetica, Arial, sans-serif" font-size="110" font-weight="700" fill="#242b3a">bowrain</text>
  <text x="534" y="400" font-family="Helvetica Neue, Helvetica, Arial, sans-serif" font-size="38" fill="#5b6474">On-brand, in every language.</text>
</svg>
EOF
rsvg-convert -w 1200 -h 630 "$TMPDIR/og.svg" -o "$LANDING/og.png"

# --- Web app ---------------------------------------------------------------
echo "  web app icons"
WEB="$REPO_ROOT/bowrain/apps/web/public"
make_favicon "$MASTER" "$WEB/favicon.ico"
resize "$TILE" 180 "$WEB/apple-touch-icon.png"
resize "$TILE" 192 "$WEB/icon-192.png"
resize "$TILE" 512 "$WEB/icon-512.png"

# --- Desktop app -----------------------------------------------------------
echo "  desktop app icons"
DESKTOP="$REPO_ROOT/bowrain/apps/bowrain"
macos_icon 1024 "$DESKTOP/build/appicon.png"
ICONSET="$TMPDIR/icons.iconset"
mkdir -p "$ICONSET"
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
if command -v iconutil >/dev/null; then
  iconutil --convert icns --output "$DESKTOP/build/darwin/icons.icns" "$ICONSET"
else
  echo "  (skipping icons.icns — iconutil not available)"
fi
WIN_PNGS=()
for s in 16 32 48 64 128 256; do
  resize "$TILE" "$s" "$TMPDIR/win-$s.png"
  WIN_PNGS+=("$TMPDIR/win-$s.png")
done
magick "${WIN_PNGS[@]}" "$DESKTOP/build/windows/icon.ico"
make_favicon "$MASTER" "$DESKTOP/frontend/public/favicon.ico"
resize "$TILE" 180 "$DESKTOP/frontend/public/apple-touch-icon.png"

# --- Docs site ---------------------------------------------------------------
echo "  docs site images"
DOCS="$REPO_ROOT/bowrain/web/docs/static/img"
resize "$MASTER" 256 "$DOCS/logo.png"
resize "$MASTER" 32 "$DOCS/favicon.png"
make_favicon "$MASTER" "$DOCS/favicon.ico"
resize "$TILE" 512 "$DOCS/hero-logo.png"
resize "$TILE" 180 "$DOCS/apple-touch-icon.png"

# --- Keycloak theme ----------------------------------------------------------
echo "  keycloak theme logos"
resize "$MASTER" 512 "$REPO_ROOT/bowrain/apps/keycloak-theme/public/logo.png"
resize "$MASTER" 512 "$REPO_ROOT/bowrain/apps/keycloak-theme/src/login/assets/logo.png"

echo "Done."
