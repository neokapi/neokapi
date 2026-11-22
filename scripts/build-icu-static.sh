#!/usr/bin/env bash
#
# build-icu-static.sh: build a static, PIC-enabled ICU from source with a
# trimmed data set (Linux + macOS).
#
# Why from source
# ---------------
# We static-link ICU into the cgo binaries (see gen-icu-static-pc.sh for the
# rationale), and prebuilt ICU always ships the FULL ~31MB data archive:
#   * distro libicu-dev static archives are not guaranteed present and are
#     typically built WITHOUT -fPIC, which fails to link into Go's
#     position-independent (PIE) cgo output ("recompile with -fPIC");
#   * the distro ICU major version drifts (70/72/74), diverging from the
#     version we link on macOS/Windows;
#   * Homebrew's icu4c static archives (the previous macOS source) cannot be
#     data-filtered, and the full data archive dominates the released binary
#     (~31MB of the CLI's ~82MB), which also lengthens macOS Gatekeeper's
#     one-time first-exec scan of each new binary.
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
# tables, display names, time zones, units and RBNF are excluded, because
# nothing resolves them at runtime. To use more of ICU, add the category (and
# any localeFilter) to the filter file; the categories are documented at
# https://unicode-org.github.io/icu/userguide/icu_data/buildtool.html, and
# every platform picks it up on the next build. Set ICU_DATA_FILTER=full to
# build the complete data set instead (or point ICU_DATA_FILTER_FILE at an
# alternative filter). The filtered data build needs python3 on PATH.
#
# Checksums
# ---------
# Each downloaded archive is checked against the SHA-256 recorded below before
# it is unpacked, so a changed or truncated download fails here and never
# reaches a binary. The digests were taken from the official release assets
# and checked against GitHub's asset digests, the release's SHASUM512.txt and
# the ICU release key's signatures when the pin was raised. An ICU_VERSION
# with no recorded checksum is refused unless ICU_SHA256_SOURCES (and, for a
# filtered build, ICU_SHA256_DATA) give the digests explicitly.
#
# Smoke check
# -----------
# After the install, a small C program is linked against the static archives
# and run. It opens the root sentence and word break iterators, a Thai word
# break iterator (dictionary-based) and the FTS5 tokenizer's compound
# transliterator, so a data filter that drops something the code loads fails
# the build instead of a query at runtime.
#
# Usage:  build-icu-static.sh [<prefix>]
#   prefix  install prefix (default: /opt/icu-static). Printed on stdout so a
#           caller can feed it to gen-icu-static-pc.sh as ICU_PREFIX.
#
# Honors ICU_VERSION (default 79.1), ICU_BUILD_JOBS (default: all cores),
# ICU_DATA_FILTER (full → no filter), ICU_DATA_FILTER_FILE (filter override),
# ICU_SHA256_SOURCES and ICU_SHA256_DATA (digests for an unrecorded version).
set -euo pipefail

script_dir=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)

ICU_VERSION="${ICU_VERSION:-79.1}"
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

# ICU release tag/asset naming (78.x and 79.x): tag "release-79.1", source
# tarball "icu4c-79.1-sources.tgz" (unpacks to icu/source), data sources
# "icu4c-79.1-data.zip" (unpacks to data/). Older lines (<=77) used dashes and
# "-src.tgz"; this targets the current dotted/-sources scheme.
tag="release-${ICU_VERSION}"
asset="icu4c-${ICU_VERSION}-sources.tgz"
data_asset="icu4c-${ICU_VERSION}-data.zip"
base_url="https://github.com/unicode-org/icu/releases/download/${tag}"

# The recorded SHA-256 of each release asset.
recorded_sha256() {
  case "$1" in
    icu4c-79.1-sources.tgz) echo "06fa63ecb521048f8082f12ad988f6b678d90cbd88aafce24cbc94902b191d99" ;;
    icu4c-79.1-data.zip)    echo "b3a430009bb5fab77b1a4b98fab85cc8f887e26d4e4166121724298ae14d7224" ;;
    *)                      echo "" ;;
  esac
}

# expected_sha256 <asset> <override>: the digest to check <asset> against. An
# explicit override wins; with neither an override nor a recorded digest, it
# fails and names the asset.
expected_sha256() {
  local name="$1" override="$2" sum
  sum="${override:-$(recorded_sha256 "$name")}"
  if [ -z "$sum" ]; then
    echo "build-icu-static: no checksum recorded for $name (ICU $ICU_VERSION)" >&2
    echo "build-icu-static: add its SHA-256 to scripts/build-icu-static.sh, or set ICU_SHA256_SOURCES / ICU_SHA256_DATA" >&2
    return 1
  fi
  echo "$sum"
}

# fetch_verified <asset> <expected-sha256> <dest>: download one release asset
# and refuse it unless its SHA-256 matches.
fetch_verified() {
  local name="$1" want="$2" out="$3" got
  echo "build-icu-static: fetching ${base_url}/${name}" >&2
  curl -fsSL --retry 3 --retry-delay 5 "${base_url}/${name}" -o "$out"
  if command -v sha256sum >/dev/null 2>&1; then
    got="$(sha256sum "$out" | cut -d' ' -f1)"
  else
    got="$(shasum -a 256 "$out" | cut -d' ' -f1)"
  fi
  if [ "$got" != "$want" ]; then
    echo "build-icu-static: checksum mismatch for $name" >&2
    echo "build-icu-static:   expected $want" >&2
    echo "build-icu-static:   actual   $got" >&2
    return 1
  fi
}

# Resolve every digest before downloading anything, so an unrecorded version
# fails at once.
src_sha256="$(expected_sha256 "$asset" "${ICU_SHA256_SOURCES:-}")"
data_sha256=""
if [ -n "$filter_file" ]; then
  data_sha256="$(expected_sha256 "$data_asset" "${ICU_SHA256_DATA:-}")"
fi

workdir=$(mktemp -d)
trap 'rm -rf "$workdir"' EXIT

fetch_verified "$asset" "$src_sha256" "$workdir/icu-src.tgz"
tar -xzf "$workdir/icu-src.tgz" -C "$workdir"

# The tarball unpacks to icu/source.
cd "$workdir/icu/source"

# The -sources tarball ships a PREBUILT data/in/icudt*.dat and omits the data
# source text files, so the data build takes the "assuming prebuilt data in
# data/in" path and a data filter would be SILENTLY ignored. When filtering,
# fetch the separate data-sources zip and replace data/ so the Python
# databuilder actually runs the filter.
if [ -n "$filter_file" ] && [ ! -f data/locales/root.txt ]; then
  fetch_verified "$data_asset" "$data_sha256" "$workdir/icu-data.zip"
  rm -rf data
  unzip -q "$workdir/icu-data.zip" -d "$workdir/icu/source"
  [ -f data/locales/root.txt ] || { echo "build-icu-static: $data_asset did not provide data/locales/root.txt" >&2; exit 1; }
fi

# Static only, PIC for PIE linking, no samples/tests/tools data we don't ship.
# CXXFLAGS/CFLAGS -fPIC belt-and-suspenders alongside --with-pic.
# ICU_DATA_FILTER_FILE is read by configure's data-build setup (the Python
# databuilder). It is exported only when filtering, so a `full` build runs the
# databuilder with no filter.
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

# Smoke check: link a small program against the installed static archives and
# open what the code loads at runtime (see the header).
cat > "$workdir/icu-smoke.c" <<'SMOKE'
#include <stdio.h>
#include <unicode/ubrk.h>
#include <unicode/ustring.h>
#include <unicode/utrans.h>

static int failed(const char *what, UErrorCode status) {
  if (U_FAILURE(status)) {
    fprintf(stderr, "icu-smoke: %s: %s\n", what, u_errorName(status));
    return 1;
  }
  return 0;
}

/* count_breaks opens a break iterator over utf8 and counts its boundaries
   after the start, or returns -1 when the iterator does not open. */
static int count_breaks(UBreakIteratorType type, const char *locale,
                        const char *utf8, const char *what) {
  UErrorCode status = U_ZERO_ERROR;
  UChar text[128];
  int32_t len = 0;
  int n = 0;
  UBreakIterator *bi;

  u_strFromUTF8(text, 128, &len, utf8, -1, &status);
  bi = ubrk_open(type, locale, text, len, &status);
  if (failed(what, status)) return -1;
  while (ubrk_next(bi) != UBRK_DONE) n++;
  ubrk_close(bi);
  return n;
}

int main(void) {
  /* The FTS5 tokenizer's default rule (core/storage/fts5_icu.h). */
  static const char *rules =
      "NFKD; Arabic-Latin; Cyrillic-Latin; Hebrew-Latin; Greek-Latin; "
      "Latin-ASCII; Lower; NFKC; Traditional-Simplified; Katakana-Hiragana";
  /* Thai is written without spaces: only the dictionary splits it. */
  static const char *thai =
      "\xe0\xb8\xaa\xe0\xb8\xa7\xe0\xb8\xb1\xe0\xb8\xaa\xe0\xb8\x94\xe0\xb8\xb5"
      "\xe0\xb8\x84\xe0\xb8\xa3\xe0\xb8\xb1\xe0\xb8\x9a";
  UErrorCode status = U_ZERO_ERROR;
  UChar urules[256];
  int32_t len = 0;
  UTransliterator *tr;
  int fail = 0, n;

  n = count_breaks(UBRK_SENTENCE, "", "One. Two.", "root sentence break iterator");
  if (n != 2) { fprintf(stderr, "icu-smoke: expected 2 sentences, got %d\n", n); fail = 1; }

  n = count_breaks(UBRK_WORD, "", "One two", "root word break iterator");
  if (n < 3) { fprintf(stderr, "icu-smoke: expected 3 word boundaries, got %d\n", n); fail = 1; }

  n = count_breaks(UBRK_WORD, "th", thai, "Thai word break iterator");
  if (n < 2) { fprintf(stderr, "icu-smoke: the Thai dictionary did not split the text (%d)\n", n); fail = 1; }

  u_strFromUTF8(urules, 256, &len, rules, -1, &status);
  tr = utrans_openU(urules, len, UTRANS_FORWARD, NULL, 0, NULL, &status);
  if (failed("FTS5 tokenizer transliterator", status)) fail = 1;
  if (tr) utrans_close(tr);

  if (!fail) printf("icu-smoke: ok (ICU %s)\n", U_ICU_VERSION);
  return fail;
}
SMOKE
case "$(uname -s)" in
  Darwin) sys_libs=(-lc++ -lm -lpthread) ;;
  *)      sys_libs=(-lstdc++ -lm -lpthread -ldl) ;;
esac
"${CC:-cc}" -I"$prefix/include" "$workdir/icu-smoke.c" -o "$workdir/icu-smoke" \
  "$prefix/lib/libicui18n.a" "$prefix/lib/libicuuc.a" "$prefix/lib/libicudata.a" \
  "${sys_libs[@]}" >&2
"$workdir/icu-smoke" >&2

echo "$prefix"
