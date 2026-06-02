#!/usr/bin/env bash
#
# build-icu-static.sh — build a static, PIC-enabled ICU from source (Linux).
#
# Why from source on Linux
# ------------------------
# We static-link ICU into the cgo binaries (see gen-icu-static-pc.sh for the
# rationale). On macOS, Homebrew's icu4c ships the static archives we need; on
# Linux there is no equally reliable source:
#   * distro libicu-dev static archives are not guaranteed present and are
#     typically built WITHOUT -fPIC, which fails to link into Go's
#     position-independent (PIE) cgo output ("recompile with -fPIC");
#   * the distro ICU major version drifts (70/72/74), diverging from the 78.x
#     we link on macOS/Windows.
# Building ICU 78.x from source with --enable-static --with-pic gives a
# deterministic, PIC-correct, version-matched static ICU on every Linux arch.
#
# Usage:  build-icu-static.sh [<prefix>]
#   prefix  install prefix (default: /opt/icu-static). Printed on stdout so a
#           caller can feed it to gen-icu-static-pc.sh as ICU_PREFIX.
#
# Honors ICU_VERSION (default 78.3) and ICU_BUILD_JOBS (default: nproc).
set -euo pipefail

ICU_VERSION="${ICU_VERSION:-78.3}"
prefix="${1:-/opt/icu-static}"
jobs="${ICU_BUILD_JOBS:-$(nproc 2>/dev/null || echo 4)}"

# ICU release tag/asset naming: v78.3 -> tag release-78-3, asset icu4c-78_3-src.
tag="release-${ICU_VERSION//./-}"
asset="icu4c-${ICU_VERSION//./_}-src.tgz"
url="https://github.com/unicode-org/icu/releases/download/${tag}/${asset}"

workdir=$(mktemp -d)
trap 'rm -rf "$workdir"' EXIT

echo "build-icu-static: fetching $url" >&2
curl -fsSL "$url" -o "$workdir/icu-src.tgz"
tar -xzf "$workdir/icu-src.tgz" -C "$workdir"

# The tarball unpacks to icu/source.
cd "$workdir/icu/source"

# Static only, PIC for PIE linking, no samples/tests/tools data we don't ship.
# CXXFLAGS/CFLAGS -fPIC belt-and-suspenders alongside --with-pic.
CFLAGS="${CFLAGS:-} -fPIC" CXXFLAGS="${CXXFLAGS:-} -fPIC" \
  ./configure \
    --prefix="$prefix" \
    --enable-static \
    --disable-shared \
    --with-pic \
    --disable-samples \
    --disable-tests \
    --disable-extras >&2

make -j"$jobs" >&2
# sudo only if the prefix isn't writable by the current user.
if [ -w "$(dirname "$prefix")" ] || [ -w "$prefix" ] 2>/dev/null; then
  make install >&2
else
  sudo make install >&2
fi

echo "$prefix"
