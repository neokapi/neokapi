#!/usr/bin/env bash
#
# gen-pdfium-static-pc.sh — emit a pkg-config file that forces STATIC linking of
# PDFium into the kapi-pdfium plugin, mirroring gen-icu-static-pc.sh.
#
# Why static: a distributable plugin must not depend on a shared libpdfium on the
# user's machine (absent; and on macOS a differently-signed dylib is rejected by
# library validation — the same reason ICU is static-linked). Static linking
# leaves only the platform C++ runtime (Apple-signed libc++ / libstdc++), always
# present.
#
# Inputs (env):
#   PDFIUM_PREFIX     install prefix containing lib/ + include/ (REQUIRED).
#                     Get a static build from bblanchon/pdfium-binaries
#                     (the "static" asset, or build PDFIUM with
#                     is_component_build=false, use_custom_libcxx=true). The
#                     default per-platform asset is SHARED-only.
#   PDFIUM_LIBDIR     dir holding libpdfium.a; default: $PDFIUM_PREFIX/lib.
#   PDFIUM_INCLUDEDIR headers dir;            default: $PDFIUM_PREFIX/include.
#   PDFIUM_CXX_LIB    C++ runtime; default: c++ on Darwin, else stdc++.
#
# NOTE on ICU coexistence: kapi static-links ICU 78. Use a PDFium build that is
# ICU-less (or uses system ICU compatibly) to avoid duplicate-symbol/version
# clashes at link time. This is the load-bearing integration risk to validate
# per platform.
set -euo pipefail

: "${PDFIUM_PREFIX:?set PDFIUM_PREFIX to a STATIC pdfium install (lib/ + include/)}"
LIBDIR="${PDFIUM_LIBDIR:-$PDFIUM_PREFIX/lib}"
INCDIR="${PDFIUM_INCLUDEDIR:-$PDFIUM_PREFIX/include}"
ARCHIVE="$LIBDIR/libpdfium.a"
[ -f "$ARCHIVE" ] || { echo "gen-pdfium-static-pc.sh: missing static archive $ARCHIVE" >&2; exit 1; }

case "$(uname -s)" in
  Darwin) CXX="${PDFIUM_CXX_LIB:-c++}"; FRAMEWORKS="-framework CoreGraphics -framework CoreText -framework CoreFoundation -framework AppKit" ;;
  *)      CXX="${PDFIUM_CXX_LIB:-stdc++}"; FRAMEWORKS="-lpthread -ldl -lm" ;;
esac

PCDIR="$(mktemp -d)"
cat > "$PCDIR/pdfium.pc" <<PC
prefix=$PDFIUM_PREFIX
libdir=$LIBDIR
includedir=$INCDIR
Name: PDFium
Description: PDFium (static)
Version: 1
Cflags: -I\${includedir}
Libs: \${libdir}/libpdfium.a -l$CXX $FRAMEWORKS
PC
echo "$PCDIR"
