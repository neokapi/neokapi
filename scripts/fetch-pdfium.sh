#!/usr/bin/env bash
#
# Fetch the pinned libpdfium shared library for one platform.
#
# The library comes from a bblanchon/pdfium-binaries release: one archive per
# platform. The archive is checked against the SHA-256 recorded below before
# anything is extracted, so a changed or truncated download fails here and
# never reaches a plugin tarball.
#
# The result is the extracted archive, the layout scripts/package-pdfium-plugin.sh
# reads as --pdfium-dir:
#
#   <dest>/include/   the PDFium C API headers
#   <dest>/lib/       libpdfium.dylib or libpdfium.so
#   <dest>/bin/       pdfium.dll (windows)
#   <dest>/LICENSE    PDFium's licence, with the bundled notices under licenses/
#
# Usage:
#   scripts/fetch-pdfium.sh <goos> <goarch> <dest>
#
# PDFIUM_RELEASE selects the release tag (default: the pin below). A release
# with no recorded checksum is refused: add the SHA-256 of every platform
# archive here in the same change that raises the pin. The release must be the
# one the klippa-app/go-pdfium version in plugins/pdfium/go.mod builds against.
set -euo pipefail

readonly DEFAULT_RELEASE="chromium/8165"
release="${PDFIUM_RELEASE:-$DEFAULT_RELEASE}"

if [ "$#" -ne 3 ]; then
  echo "usage: $0 <goos> <goarch> <dest>" >&2
  exit 2
fi
goos="$1"
goarch="$2"
dest="$3"

case "$goos/$goarch" in
  darwin/arm64)  asset="pdfium-mac-arm64.tgz" ;;
  linux/amd64)   asset="pdfium-linux-x64.tgz" ;;
  linux/arm64)   asset="pdfium-linux-arm64.tgz" ;;
  windows/amd64) asset="pdfium-win-x64.tgz" ;;
  *)
    echo "fetch-pdfium: no libpdfium build recorded for $goos/$goarch" >&2
    exit 1
    ;;
esac

# The checksums are the digests GitHub reports for the release assets, checked
# against a download of each archive when the pin was raised.
case "$release/$asset" in
  chromium/8165/pdfium-mac-arm64.tgz)   sha256="112e74d717f3e0310273b70f3c8780e8a61ff11b34834124465c8305c841d350" ;;
  chromium/8165/pdfium-linux-x64.tgz)   sha256="ba7865e16c0271daab75256790be06c26272190afe6e2a70eae0bb39bb99089c" ;;
  chromium/8165/pdfium-linux-arm64.tgz) sha256="338f5dcc907a20331812ae8ab01b16ff18966f4a247d790c8ad8fb49029d768b" ;;
  chromium/8165/pdfium-win-x64.tgz)     sha256="ca7338ae7779b8b1b82f4580305c4d762d214f8df1d54b47647fe3cc75989ded" ;;
  *)
    echo "fetch-pdfium: no checksum recorded for $asset in pdfium-binaries $release" >&2
    echo "fetch-pdfium: add its SHA-256 to scripts/fetch-pdfium.sh" >&2
    exit 1
    ;;
esac

work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT

url="https://github.com/bblanchon/pdfium-binaries/releases/download/${release}/${asset}"
echo "fetch-pdfium: downloading $url"
curl -sSfL --retry 3 --retry-delay 5 -o "$work/pdfium.tgz" "$url"

if command -v sha256sum >/dev/null 2>&1; then
  actual="$(sha256sum "$work/pdfium.tgz" | cut -d' ' -f1)"
else
  actual="$(shasum -a 256 "$work/pdfium.tgz" | cut -d' ' -f1)"
fi
if [ "$actual" != "$sha256" ]; then
  echo "fetch-pdfium: checksum mismatch for $asset in pdfium-binaries $release" >&2
  echo "fetch-pdfium:   expected $sha256" >&2
  echo "fetch-pdfium:   actual   $actual" >&2
  exit 1
fi

mkdir -p "$dest"
tar xzf "$work/pdfium.tgz" -C "$dest"

case "$goos" in
  darwin)  lib="lib/libpdfium.dylib" ;;
  windows) lib="bin/pdfium.dll" ;;
  *)       lib="lib/libpdfium.so" ;;
esac
if [ ! -s "$dest/$lib" ]; then
  echo "fetch-pdfium: $lib is missing from $asset in pdfium-binaries $release" >&2
  exit 1
fi
echo "fetch-pdfium: $dest/$lib"
