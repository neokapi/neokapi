#!/usr/bin/env bash
#
# build-icu-static.sh — build a static, PIC-enabled ICU from source with a
# trimmed data set (Linux + macOS).
#
# Why from source
# ---------------
# We static-link ICU into the cgo binaries (see gen-icu-static-pc.sh for the
# rationale), and prebuilt ICU always ships the FULL ~31MB data archive:
#   * distro libicu-dev static archives are not guaranteed present and are
#     typically built WITHOUT -fPIC, which fails to link into Go's
#     position-independent (PIE) cgo output ("recompile with -fPIC");
#   * the distro ICU major version drifts (70/72/74), diverging from the 78.x
#     we link on macOS/Windows;
#   * Homebrew's icu4c static archives (the previous macOS source) cannot be
#     data-filtered, and the full data archive dominates the released binary
#     (~31MB of the CLI's ~82MB) — which also directly lengthens macOS
#     Gatekeeper's one-time first-exec scan of each new binary.
# Building from source with --enable-static --with-pic and a data filter gives
# a deterministic, PIC-correct, version-matched, minimal ICU everywhere.
#
# Data filter
# -----------
# scripts/icu-data-filter.json (additive allowlist) keeps only what the code
# links and loads: break iteration (the UAX-29 segmenter in core/segment/uax29
# and the FTS5 ICU tokenizer in core/storage use ubrk_*) with its rules,
# locale bundles, and CJK/Thai/Khmer/Lao/Burmese dictionaries; normalization
# and transliteration (the FTS5 tokenizer's fold pipeline is a compound
# transliterator: script-to-Latin + Lower + NFKC + Traditional-Simplified);
# converter aliases; and emoji properties. Collation, charset conversion
# tables, display names, time zones, units, RBNF
# are all excluded — nothing resolves them at runtime today. To use more of
# ICU later, add the category (and any localeFilter) to the filter file — the
# categories are documented at
# https://unicode-org.github.io/icu/userguide/icu_data/buildtool.html — and
# every platform picks it up on the next build. Set ICU_DATA_FILTER=full to
# build the complete data set instead (or point ICU_DATA_FILTER_FILE at an
# alternative filter). The filtered data build needs python3 on PATH.
#
# Usage:  build-icu-static.sh [<prefix>]
#   prefix  install prefix (default: /opt/icu-static). Printed on stdout so a
#           caller can feed it to gen-icu-static-pc.sh as ICU_PREFIX.
#
# Honors ICU_VERSION (default 78.3), ICU_BUILD_JOBS (default: all cores),
# ICU_DATA_FILTER (full → no filter), ICU_DATA_FILTER_FILE (filter override).
set -euo pipefail

script_dir=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)

ICU_VERSION="${ICU_VERSION:-78.3}"
prefix="${1:-/opt/icu-static}"
jobs="${ICU_BUILD_JOBS:-$(nproc 2>/dev/null || sysctl -n hw.ncpu 2>/dev/null || echo 4)}"

# Resolve the data filter: explicit file > repo default; ICU_DATA_FILTER=full
# disables filtering entirely.
filter_file="${ICU_DATA_FILTER_FILE:-$script_dir/icu-data-filter.json}"
if [ "${ICU_DATA_FILTER:-}" = "full" ]; then
  filter_file=""
fi
if [ -n "$filter_file" ]; then
  [ -f "$filter_file" ] || { echo "build-icu-static: filter file not found: $filter_file" >&2; exit 1; }
  command -v python3 >/dev/null || { echo "build-icu-static: python3 is required for a filtered ICU data build (set ICU_DATA_FILTER=full to skip filtering)" >&2; exit 1; }
  echo "build-icu-static: data filter $filter_file" >&2
else
  echo "build-icu-static: full (unfiltered) ICU data" >&2
fi

# ICU release tag/asset naming (78.x): tag "release-78.3", source tarball
# "icu4c-78.3-sources.tgz" (unpacks to icu/source). Older lines (<=77) used
# dashes + "-src.tgz"; this targets the current dotted/-sources scheme.
tag="release-${ICU_VERSION}"
asset="icu4c-${ICU_VERSION}-sources.tgz"
url="https://github.com/unicode-org/icu/releases/download/${tag}/${asset}"

workdir=$(mktemp -d)
trap 'rm -rf "$workdir"' EXIT

echo "build-icu-static: fetching $url" >&2
curl -fsSL "$url" -o "$workdir/icu-src.tgz"
tar -xzf "$workdir/icu-src.tgz" -C "$workdir"

# The tarball unpacks to icu/source.
cd "$workdir/icu/source"

# The -sources tarball ships a PREBUILT data/in/icudt*.dat and omits the data
# source text files, so the data build takes the "assuming prebuilt data in
# data/in" path and a data filter would be SILENTLY ignored. When filtering,
# fetch the separate data-sources zip and replace data/ so the Python
# databuilder actually runs the filter.
if [ -n "$filter_file" ] && [ ! -f data/locales/root.txt ]; then
  data_asset="icu4c-${ICU_VERSION}-data.zip"
  echo "build-icu-static: fetching data sources $data_asset" >&2
  curl -fsSL "https://github.com/unicode-org/icu/releases/download/${tag}/${data_asset}" -o "$workdir/icu-data.zip"
  rm -rf data
  unzip -q "$workdir/icu-data.zip" -d "$workdir/icu/source"
  [ -f data/locales/root.txt ] || { echo "build-icu-static: $data_asset did not provide data/locales/root.txt" >&2; exit 1; }
fi

# Static only, PIC for PIE linking, no samples/tests/tools data we don't ship.
# CXXFLAGS/CFLAGS -fPIC belt-and-suspenders alongside --with-pic.
# ICU_DATA_FILTER_FILE is read by configure's data-build setup (the Python
# databuilder); exporting it only when filtering keeps `full` builds byte-
# identical to the previous behavior.
if [ -n "$filter_file" ]; then
  export ICU_DATA_FILTER_FILE="$filter_file"
else
  unset ICU_DATA_FILTER_FILE
fi
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
