#!/usr/bin/env bash
#
# Provide PDFium to pkg-config for type-checking the kapi-pdfium plugin, and
# print the directory to put on PKG_CONFIG_PATH.
#
# go-pdfium's cgo files name `pkg-config: pdfium`, so a tool that type-checks
# plugins/pdfium (scripts/fieldguard) fails on that package without it. The
# library comes from scripts/fetch-pdfium.sh, pinned and checked against its
# SHA-256, and is fetched once into <cache-dir>/<release>. The pdfium.pc
# written beside it points at the archive's include/ and lib/.
#
# Usage:
#   scripts/pdfium-pkgconfig.sh <cache-dir>
set -euo pipefail

if [ "$#" -ne 1 ]; then
  echo "usage: $0 <cache-dir>" >&2
  exit 2
fi
root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
release="$(sed -n 's/^readonly DEFAULT_RELEASE="\(.*\)"$/\1/p' "$root/scripts/fetch-pdfium.sh")"
if [ -z "$release" ]; then
  echo "pdfium-pkgconfig: cannot read the pinned release from scripts/fetch-pdfium.sh" >&2
  exit 1
fi
goos="$(go env GOOS)"
goarch="$(go env GOARCH)"
dir="$1/${release//\//-}-$goos-$goarch"

if [ ! -f "$dir/pdfium/include/fpdfview.h" ]; then
  tmp="$dir.partial.$$"
  rm -rf "$tmp"
  # fetch-pdfium.sh prints its progress; this script's stdout is the path.
  PDFIUM_RELEASE="$release" "$root/scripts/fetch-pdfium.sh" "$goos" "$goarch" "$tmp" >&2
  mkdir -p "$dir"
  rm -rf "$dir/pdfium"
  mv "$tmp" "$dir/pdfium"
fi

cat > "$dir/pdfium.pc" <<PC
prefix=$dir/pdfium
includedir=\${prefix}/include
libdir=\${prefix}/lib
Name: PDFium
Description: PDFium (pinned by scripts/fetch-pdfium.sh)
Version: ${release#chromium/}
Cflags: -I\${includedir}
Libs: -L\${libdir} -lpdfium
PC
echo "$dir"
