#!/usr/bin/env bash
#
# gen-icu-static-pc.sh — emit pkg-config files that force STATIC linking of ICU.
#
# Why this exists
# ---------------
# The release binaries (kapi/kapi-bowrain CLIs and the Wails desktop apps) are
# all cgo, and the framework links ICU through `#cgo pkg-config: icu-uc icu-i18n`
# (the FTS5 ICU tokenizer in core/storage and the UAX-29 segmenter in
# core/segment/uax29). Stock icu-uc.pc / icu-i18n.pc emit `-licui18n -licuuc`,
# which the linker resolves to a shared library — fatal for a distributable:
#   * macOS: a notarized .app/binary aborts in dyld at launch because Homebrew's
#     icu dylib is absent on users' machines and, when present, has a different
#     code-signing Team ID (library validation rejects it);
#   * Linux: a single prebuilt binary can't satisfy the distro libicu major-skew
#     (system libicu 70/72/74 vs the build's 78 — versioned symbols won't bind).
#
# Static linking removes the runtime dependency entirely; the only ICU-related
# dylib left is the platform C++ runtime (Apple-signed libc++ on macOS,
# libstdc++ elsewhere), which is always present.
#
# What it does
# ------------
# Writes override icu-uc.pc and icu-i18n.pc into a fresh temp directory whose
# `Libs:` lines reference the ICU static archives (*.a) by absolute path, in
# dependency order (i18n -> uc -> data) plus the C++ runtime, and prints that
# directory on stdout. Point PKG_CONFIG_PATH at it before building:
#
#   PCDIR=$(scripts/gen-icu-static-pc.sh)
#   PKG_CONFIG_PATH="$PCDIR" CGO_ENABLED=1 go build -tags fts5 ...
#
# Inputs (env, all optional — sensible per-platform defaults)
#   ICU_PREFIX       ICU install prefix; default: brew --prefix icu4c@78|icu4c.
#   ICU_LIBDIR       dir holding libicu*.a; default: $ICU_PREFIX/lib. Override
#                    for split layouts (Debian multiarch: /usr/lib/<triple>).
#   ICU_INCLUDEDIR   ICU headers dir;     default: $ICU_PREFIX/include.
#   ICU_CXX_LIB      C++ runtime to link; default: c++ on Darwin, else stdc++.
#
# Fails loudly if the static archives are missing.
set -euo pipefail

uname_s=$(uname -s)

icu_prefix="${ICU_PREFIX:-}"
if [[ -z "$icu_prefix" ]]; then
  for formula in icu4c@78 icu4c; do
    if icu_prefix=$(brew --prefix "$formula" 2>/dev/null) && [[ -n "$icu_prefix" ]]; then
      break
    fi
  done
fi

libdir="${ICU_LIBDIR:-${icu_prefix:+$icu_prefix/lib}}"
includedir="${ICU_INCLUDEDIR:-${icu_prefix:+$icu_prefix/include}}"

if [[ -z "$libdir" || ! -d "$libdir" ]]; then
  echo "gen-icu-static-pc.sh: could not locate an ICU lib dir (set ICU_PREFIX or ICU_LIBDIR)" >&2
  exit 1
fi

for a in libicui18n.a libicuuc.a libicudata.a; do
  if [[ ! -f "$libdir/$a" ]]; then
    echo "gen-icu-static-pc.sh: missing static archive $libdir/$a (need an icu4c build with static libraries)" >&2
    exit 1
  fi
done

cxx_lib="${ICU_CXX_LIB:-}"
if [[ -z "$cxx_lib" ]]; then
  if [[ "$uname_s" == "Darwin" ]]; then
    cxx_lib="c++"
  else
    cxx_lib="stdc++"
  fi
fi

pcdir=$(mktemp -d)

# Both .pc files carry the full archive set in link order so the result is
# correct no matter which package(s) cgo passes to pkg-config (it always
# requests both "icu-uc icu-i18n"). Duplicate archives on the link line are
# harmless — the linker dedupes them with a warning.
libs="$libdir/libicui18n.a $libdir/libicuuc.a $libdir/libicudata.a -l$cxx_lib"

for name in icu-uc icu-i18n; do
  cat > "$pcdir/$name.pc" <<EOF
libdir=$libdir
includedir=$includedir

Name: $name
Description: ICU ($name) — static-archive override for self-contained binaries
Version: 78
Cflags: -I\${includedir}
Libs: $libs
EOF
done

echo "$pcdir"
