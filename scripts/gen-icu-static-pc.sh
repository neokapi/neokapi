#!/usr/bin/env bash
#
# gen-icu-static-pc.sh — emit pkg-config files that force STATIC linking of ICU.
#
# Why this exists
# ---------------
# The Wails desktop apps (kapi-desktop, bowrain) require cgo, and the framework
# links ICU through `#cgo pkg-config: icu-uc icu-i18n` (the FTS5 ICU tokenizer
# in core/storage and the UAX-29 segmenter in core/segment/uax29). On macOS,
# Homebrew's stock icu-uc.pc / icu-i18n.pc emit `-licui18n -licuuc`, which the
# linker resolves to the Homebrew *.dylib at /opt/homebrew/.../libicui18n.78.dylib.
# A distributed .app then fails at launch in dyld because:
#   1. the user usually has no Homebrew icu4c at all, and
#   2. even when present, the dylib's code signature carries a different Team ID
#      than the notarized app, so dyld refuses to map it (SIGABRT before main).
#
# Static linking removes the runtime dependency entirely: the only dylib left is
# the Apple-signed system /usr/lib/libc++.1.dylib, which every Mac has.
#
# What it does
# ------------
# Writes override icu-uc.pc and icu-i18n.pc into a fresh temp directory whose
# `Libs:` lines reference the ICU static archives (*.a) by absolute path, in
# dependency order (i18n -> uc -> data) plus -lc++ for ICU's C++ runtime, and
# prints that directory on stdout. Point PKG_CONFIG_PATH at it before building:
#
#   PCDIR=$(scripts/gen-icu-static-pc.sh)
#   PKG_CONFIG_PATH="$PCDIR" CGO_ENABLED=1 go build ...
#
# The ICU prefix is taken from $ICU_PREFIX if set, otherwise discovered via
# Homebrew (icu4c@78, then icu4c). Fails loudly if the static archives are
# missing.
set -euo pipefail

icu_prefix="${ICU_PREFIX:-}"
if [[ -z "$icu_prefix" ]]; then
  for formula in icu4c@78 icu4c; do
    if icu_prefix=$(brew --prefix "$formula" 2>/dev/null) && [[ -n "$icu_prefix" ]]; then
      break
    fi
  done
fi

if [[ -z "$icu_prefix" || ! -d "$icu_prefix/lib" ]]; then
  echo "gen-icu-static-pc.sh: could not locate an ICU prefix (set ICU_PREFIX or 'brew install icu4c')" >&2
  exit 1
fi

libdir="$icu_prefix/lib"
for a in libicui18n.a libicuuc.a libicudata.a; do
  if [[ ! -f "$libdir/$a" ]]; then
    echo "gen-icu-static-pc.sh: missing static archive $libdir/$a (need an icu4c build with static libraries)" >&2
    exit 1
  fi
done

pcdir=$(mktemp -d)

# Both .pc files carry the full archive set in link order so the result is
# correct no matter which package(s) cgo passes to pkg-config (it always
# requests both "icu-uc icu-i18n"). Duplicate archives on the link line are
# harmless — the linker dedupes them with a warning.
libs="$libdir/libicui18n.a $libdir/libicuuc.a $libdir/libicudata.a -lc++"

for name in icu-uc icu-i18n; do
  cat > "$pcdir/$name.pc" <<EOF
prefix=$icu_prefix
libdir=\${prefix}/lib
includedir=\${prefix}/include

Name: $name
Description: ICU ($name) — static-archive override for self-contained app bundles
Version: $(basename "$icu_prefix" | sed 's/^icu4c@//')
Cflags: -I\${includedir}
Libs: $libs
EOF
done

echo "$pcdir"
