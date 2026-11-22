#!/usr/bin/env bash
#
# Fetch the pinned onnxruntime shared library for one platform.
#
# The library comes from the Microsoft.ML.OnnxRuntime NuGet package: one
# archive that holds the native library of every platform kapi-sat and
# kapi-vision are built for. The archive is checked against the SHA-256 pinned
# below before anything is extracted, so a changed or truncated download fails
# here and never reaches a plugin tarball.
#
# The result has the layout the package scripts and the test jobs read:
#
#   <dest>/lib/                   the platform's native library files
#   <dest>/include/               the C API headers
#   <dest>/LICENSE                onnxruntime's licence
#   <dest>/ThirdPartyNotices.txt  the notices of what it bundles
#
# Usage:
#   scripts/fetch-onnxruntime.sh <goos> <goarch> <dest>
#
# ONNXRUNTIME_VERSION selects the version (default: the pin below). A version
# with no recorded checksum is refused: add its SHA-256 here in the same change
# that raises the pin. The version must match the ORT_API_VERSION of the
# yalue/onnxruntime_go release in plugins/sat/go.mod, plugins/vision/go.mod and
# plugins/check/go.mod.
set -euo pipefail

readonly DEFAULT_VERSION="1.31.1"
version="${ONNXRUNTIME_VERSION:-$DEFAULT_VERSION}"

case "$version" in
  1.31.1) sha256="6871ccbc97f3d5bdb4095a1dcec33aa78d56f6ed8de5980ef5fa1d9f2e7c679a" ;;
  *)
    echo "fetch-onnxruntime: no checksum recorded for onnxruntime $version" >&2
    echo "fetch-onnxruntime: add its SHA-256 to scripts/fetch-onnxruntime.sh" >&2
    exit 1
    ;;
esac

if [ "$#" -ne 3 ]; then
  echo "usage: $0 <goos> <goarch> <dest>" >&2
  exit 2
fi
goos="$1"
goarch="$2"
dest="$3"

case "$goos/$goarch" in
  linux/amd64)   rid="linux-x64" ;;
  linux/arm64)   rid="linux-arm64" ;;
  darwin/arm64)  rid="osx-arm64" ;;
  windows/amd64) rid="win-x64" ;;
  windows/arm64) rid="win-arm64" ;;
  *)
    echo "fetch-onnxruntime: no onnxruntime build for $goos/$goarch" >&2
    exit 1
    ;;
esac

work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT

url="https://api.nuget.org/v3-flatcontainer/microsoft.ml.onnxruntime/${version}/microsoft.ml.onnxruntime.${version}.nupkg"
echo "fetch-onnxruntime: downloading onnxruntime $version ($rid)"
curl -sSfL --retry 3 --retry-delay 5 -o "$work/onnxruntime.nupkg" "$url"

if command -v sha256sum >/dev/null 2>&1; then
  actual="$(sha256sum "$work/onnxruntime.nupkg" | cut -d' ' -f1)"
else
  actual="$(shasum -a 256 "$work/onnxruntime.nupkg" | cut -d' ' -f1)"
fi
if [ "$actual" != "$sha256" ]; then
  echo "fetch-onnxruntime: checksum mismatch for onnxruntime $version" >&2
  echo "fetch-onnxruntime:   expected $sha256" >&2
  echo "fetch-onnxruntime:   actual   $actual" >&2
  exit 1
fi

unzip -q "$work/onnxruntime.nupkg" \
  "runtimes/${rid}/native/*" "build/native/include/*" LICENSE ThirdPartyNotices.txt \
  -d "$work/pkg"

mkdir -p "$dest/lib" "$dest/include"
cp "$work/pkg/runtimes/${rid}/native/"* "$dest/lib/"
cp "$work/pkg/build/native/include/"* "$dest/include/"
cp "$work/pkg/LICENSE" "$work/pkg/ThirdPartyNotices.txt" "$dest/"

case "$goos" in
  darwin)  lib="libonnxruntime.dylib" ;;
  windows) lib="onnxruntime.dll" ;;
  *)       lib="libonnxruntime.so" ;;
esac
if [ ! -s "$dest/lib/$lib" ]; then
  echo "fetch-onnxruntime: $lib is missing from the onnxruntime $version package for $rid" >&2
  exit 1
fi
echo "fetch-onnxruntime: $dest/lib/$lib"
