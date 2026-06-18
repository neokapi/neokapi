#!/usr/bin/env bash
# Build + package one platform's kapi-pdfium plugin tarball for distribution via
# the neokapi/registry (installable with `kapi plugins install pdfium`) and
# bundling with the kapi CLI / desktop.
#
# kapi-pdfium is a cgo binary: it links PDFium (go-pdfium) at build time. To make
# an installed plugin work with no environment configuration, the PDFium SHARED
# library is bundled into the tarball at `lib/<name>` beside the binary and found
# via an rpath baked into the binary (@loader_path/lib on macOS, $ORIGIN/lib on
# Linux; same-dir search on Windows). This mirrors how package-sat-plugin.sh
# bundles onnxruntime — no static archive, no ICU-coexistence concern.
#
# Tarball layout (kapi-pdfium_<version>_<os>_<arch>.tar.gz):
#     kapi-pdfium[.exe]         the plugin binary, at the tarball root
#     manifest.json             the plugin manifest, at the tarball root
#     lib/libpdfium.dylib       PDFium shared lib (darwin) | libpdfium.so (linux)
#     pdfium.dll                (windows: beside the .exe at root)
#
# Usage (flags or env; flags win):
#     scripts/package-pdfium-plugin.sh \
#       --version 0.1.0 \
#       --pdfium-dir /path/to/extracted/bblanchon/pdfium \
#       --out-dir /path/to/output
#
# PDFIUM_DIR is an EXTRACTED bblanchon pdfium-binaries release dir, containing
# include/ (headers) and lib/ (shared lib; bin/ on Windows). Get it from
# https://github.com/bblanchon/pdfium-binaries/releases (the default per-platform
# asset is the shared build this script wants).
#
# Builds for the HOST platform only (go env GOOS/GOARCH); the release matrix runs
# this once per native runner. Echoes the tarball path + sha256, and appends them
# to $GITHUB_OUTPUT (as `tarball` and `sha256`) when set.
set -euo pipefail

VERSION="${VERSION:-}"
PDFIUM_DIR="${PDFIUM_DIR:-}"
OUT_DIR="${OUT_DIR:-}"

while [ $# -gt 0 ]; do
  case "$1" in
    --version)     VERSION="$2"; shift 2 ;;
    --pdfium-dir)  PDFIUM_DIR="$2"; shift 2 ;;
    --out-dir)     OUT_DIR="$2"; shift 2 ;;
    --version=*)    VERSION="${1#*=}"; shift ;;
    --pdfium-dir=*) PDFIUM_DIR="${1#*=}"; shift ;;
    --out-dir=*)    OUT_DIR="${1#*=}"; shift ;;
    -h|--help) sed -n '2,33p' "$0"; exit 0 ;;
    *) echo "package-pdfium-plugin: unknown argument: $1" >&2; exit 2 ;;
  esac
done

: "${VERSION:?--version (or \$VERSION) is required}"
: "${PDFIUM_DIR:?--pdfium-dir (or \$PDFIUM_DIR) is required: extracted bblanchon pdfium dir}"
: "${OUT_DIR:?--out-dir (or \$OUT_DIR) is required}"

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
PLUGIN_DIR="$REPO_ROOT/plugins/pdfium"
[ -f "$PLUGIN_DIR/go.mod" ] || { echo "package-pdfium-plugin: cannot find plugins/pdfium under $REPO_ROOT" >&2; exit 1; }

GOOS="$(go env GOOS)"
GOARCH="$(go env GOARCH)"
PDFIUM_DIR="$(cd "$PDFIUM_DIR" && pwd)"

BIN_NAME="kapi-pdfium"
[ "$GOOS" = "windows" ] && BIN_NAME="kapi-pdfium.exe"

# Platform shared-library name + where it goes in the tarball.
case "$GOOS" in
  darwin)  LIB_NAME="libpdfium.dylib"; LIB_DEST="lib" ;;
  windows) LIB_NAME="pdfium.dll";      LIB_DEST="." ;;   # Windows searches the exe dir
  *)       LIB_NAME="libpdfium.so";    LIB_DEST="lib" ;;
esac

# Locate the shared lib in the bblanchon dir (lib/ on unix, bin/ on windows).
PDFIUM_LIB=""
for d in "$PDFIUM_DIR/lib" "$PDFIUM_DIR/bin" "$PDFIUM_DIR"; do
  for n in libpdfium.dylib libpdfium.so pdfium.dll; do
    [ -f "$d/$n" ] && PDFIUM_LIB="$d/$n" && break 2
  done
done
[ -n "$PDFIUM_LIB" ] || { echo "package-pdfium-plugin: no shared libpdfium under $PDFIUM_DIR" >&2; exit 1; }
[ -d "$PDFIUM_DIR/include" ] || { echo "package-pdfium-plugin: missing $PDFIUM_DIR/include" >&2; exit 1; }

mkdir -p "$OUT_DIR"; OUT_DIR="$(cd "$OUT_DIR" && pwd)"
STAGE="$OUT_DIR/stage-${GOOS}-${GOARCH}"
rm -rf "$STAGE"; mkdir -p "$STAGE/lib"

# Dynamic pkg-config file so go-pdfium's cgo build finds PDFium headers + lib.
# On Windows, mingw's cgo toolchain can't consume bblanchon's MSVC import lib
# (pdfium.dll.lib); instead link the DLL DIRECTLY with GNU ld's `-l:<file>`
# form (-l:pdfium.dll), which needs no .dll.a import library. Elsewhere use the
# normal -lpdfium against the shared object.
PC_INC="$PDFIUM_DIR/include"
PC_LIBDIR="$(dirname "$PDFIUM_LIB")"
if [ "$GOOS" = "windows" ] && command -v cygpath >/dev/null 2>&1; then
  # mingw gcc/ld want mixed-style paths (D:/a/…), not git-bash POSIX (/d/a/…).
  PC_INC="$(cygpath -m "$PC_INC")"
  PC_LIBDIR="$(cygpath -m "$PC_LIBDIR")"
fi
if [ "$GOOS" = "windows" ]; then
  LIBS_LINE="-L$PC_LIBDIR -l:pdfium.dll"
else
  LIBS_LINE="-L$PC_LIBDIR -lpdfium"
fi
PCDIR="$OUT_DIR/pc-${GOOS}-${GOARCH}"; mkdir -p "$PCDIR"
cat > "$PCDIR/pdfium.pc" <<PC
Name: PDFium
Description: PDFium (shared)
Version: 1
Cflags: -I$PC_INC
Libs: ${LIBS_LINE}
PC

# rpath so the bundled lib is found beside the binary with no env config.
case "$GOOS" in
  darwin) RPATH_FLAG="-Wl,-rpath,@loader_path/lib" ;;
  linux)  RPATH_FLAG="-Wl,-rpath,\$ORIGIN/lib" ;;
  *)      RPATH_FLAG="" ;;
esac

VERSION_PKG="github.com/neokapi/neokapi/core/version"
# -tags pdfium_experimental wires go-pdfium's experimental marked-content APIs so
# the tagged-PDF structure path can bridge struct elements to text. The bundled
# bblanchon libpdfium exports those symbols.
echo "package-pdfium-plugin: building ${BIN_NAME} ${VERSION} for ${GOOS}/${GOARCH}"
(
  cd "$PLUGIN_DIR"
  GOWORK=off CGO_ENABLED=1 PKG_CONFIG_PATH="$PCDIR" \
    CGO_LDFLAGS="${RPATH_FLAG}" \
    go build -trimpath -tags pdfium_experimental -ldflags "-s -w -X ${VERSION_PKG}.Version=${VERSION}" \
      -o "$STAGE/${BIN_NAME}" ./cmd/kapi-pdfium
)

cp "$PLUGIN_DIR/manifest.json" "$STAGE/manifest.json"

# Bundle the shared lib + make the binary find it via rpath.
cp -L "$PDFIUM_LIB" "$STAGE/${LIB_DEST}/${LIB_NAME}"
if [ "$GOOS" = "darwin" ]; then
  install_name_tool -id "@rpath/${LIB_NAME}" "$STAGE/lib/${LIB_NAME}"
  # Repoint the binary's PDFium reference (bblanchon ships id "./libpdfium.dylib")
  # at the rpath-resolved bundled copy.
  oldref="$(otool -L "$STAGE/${BIN_NAME}" | awk '/libpdfium\.dylib/{print $1; exit}')"
  [ -n "$oldref" ] && install_name_tool -change "$oldref" "@rpath/${LIB_NAME}" "$STAGE/${BIN_NAME}"
fi
[ "$GOOS" != "windows" ] && [ "$LIB_DEST" != "lib" ] && rmdir "$STAGE/lib" 2>/dev/null || true

# Tar at the tarball root (no stage-dir component).
TARBALL="kapi-pdfium_${VERSION}_${GOOS}_${GOARCH}.tar.gz"
TARBALL_PATH="$OUT_DIR/$TARBALL"
tar -czf "$TARBALL_PATH" -C "$STAGE" .

# sha256: Linux + Windows git-bash have sha256sum; macOS has shasum -a 256.
if command -v sha256sum >/dev/null 2>&1; then
  SHA="$(sha256sum "$TARBALL_PATH" | awk '{print $1}')"
else
  SHA="$(shasum -a 256 "$TARBALL_PATH" | awk '{print $1}')"
fi
echo "package-pdfium-plugin: $TARBALL_PATH"
echo "package-pdfium-plugin: sha256 $SHA"
if [ -n "${GITHUB_OUTPUT:-}" ]; then
  { echo "tarball=$TARBALL_PATH"; echo "sha256=$SHA"; } >> "$GITHUB_OUTPUT"
fi
