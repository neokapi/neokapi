#!/usr/bin/env bash
# Build + package one platform's kapi-check plugin tarball for distribution via
# the neokapi/registry (installable with `kapi plugins install check`).
#
# The kapi-check plugin is built with `-tags onnx` (cgo): it links the
# daulet/tokenizers static library at BUILD time and loads the onnxruntime
# shared library at RUNTIME. To make an installed plugin work with no
# environment configuration, the onnxruntime shared library is bundled into the
# tarball at `lib/<unversioned-name>` beside the binary — exactly where
# plugins/check/internal/embed/ort_onnx.go:resolveORTLib() looks for it when
# KAPI_CHECK_ORT_LIB is unset.
#
# Models are NOT bundled: the int8 sentence-embedding model downloads on the
# explicit `kapi-check pull` step (into the XDG model cache), matching the
# kapi-sat plugin and common practice (vale sync / spacy download / ollama pull).
# This keeps the per-platform tarball small (binary + runtime lib) while the
# heavier model is fetched once, cached, and offline thereafter.
#
# Tarball layout (kapi-check_<version>_<os>_<arch>.tar.gz):
#
#     kapi-check[.exe]          the plugin binary, at the tarball root
#     manifest.json             the plugin manifest, at the tarball root
#     lib/libonnxruntime.dylib  onnxruntime shared lib (darwin)
#       | libonnxruntime.so     (linux)
#       | onnxruntime.dll       (windows)
#
# Usage (flags or env; flags win):
#
#     scripts/package-check-plugin.sh \
#       --version 0.1.0 \
#       --ort-dir /path/to/extracted/onnxruntime \
#       --tokenizers-lib /path/to/dir/with/libtokenizers.a \
#       --out-dir /path/to/output
#
# Equivalent env vars: VERSION, ORT_DIR, TOKENIZERS_LIB, OUT_DIR.
#
# - ORT_DIR is an EXTRACTED onnxruntime release directory; the script finds the
#   (possibly versioned) shared library under "$ORT_DIR" (commonly
#   "$ORT_DIR/lib") and copies it to "lib/<unversioned-name>" in the staged
#   tarball.
# - TOKENIZERS_LIB is the directory containing libtokenizers.a (unix) or the
#   import/static lib (windows). It is passed to the cgo linker via CGO_LDFLAGS.
#
# Builds for the HOST platform only (go env GOOS/GOARCH); the release matrix
# runs this once per native runner. Echoes the tarball path and sha256, and
# appends them to $GITHUB_OUTPUT (as `tarball` and `sha256`) when set.
set -euo pipefail

# ── parse flags (override env) ────────────────────────────────────────────────
VERSION="${VERSION:-}"
ORT_DIR="${ORT_DIR:-}"
TOKENIZERS_LIB="${TOKENIZERS_LIB:-}"
OUT_DIR="${OUT_DIR:-}"

while [ $# -gt 0 ]; do
  case "$1" in
    --version)        VERSION="$2"; shift 2 ;;
    --ort-dir)        ORT_DIR="$2"; shift 2 ;;
    --tokenizers-lib) TOKENIZERS_LIB="$2"; shift 2 ;;
    --out-dir)        OUT_DIR="$2"; shift 2 ;;
    --version=*)        VERSION="${1#*=}"; shift ;;
    --ort-dir=*)        ORT_DIR="${1#*=}"; shift ;;
    --tokenizers-lib=*) TOKENIZERS_LIB="${1#*=}"; shift ;;
    --out-dir=*)        OUT_DIR="${1#*=}"; shift ;;
    -h|--help)
      sed -n '2,51p' "$0"
      exit 0
      ;;
    *)
      echo "package-check-plugin: unknown argument: $1" >&2
      exit 2
      ;;
  esac
done

: "${VERSION:?--version (or \$VERSION) is required}"
: "${ORT_DIR:?--ort-dir (or \$ORT_DIR) is required: extracted onnxruntime dir}"
: "${TOKENIZERS_LIB:?--tokenizers-lib (or \$TOKENIZERS_LIB) is required: dir with libtokenizers.a}"
: "${OUT_DIR:?--out-dir (or \$OUT_DIR) is required}"

# ── locate this repo (the script lives in <repo>/scripts/) ────────────────────
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
PLUGIN_DIR="$REPO_ROOT/plugins/check"

if [ ! -f "$PLUGIN_DIR/go.mod" ]; then
  echo "package-check-plugin: cannot find plugins/check under $REPO_ROOT" >&2
  exit 1
fi

# ── host platform ─────────────────────────────────────────────────────────────
GOOS="$(go env GOOS)"
GOARCH="$(go env GOARCH)"

BIN_NAME="kapi-check"
if [ "$GOOS" = "windows" ]; then
  BIN_NAME="kapi-check.exe"
fi

# Unversioned onnxruntime shared-library name the binary expects beside it
# (must match plugins/check/internal/embed/ort_onnx.go:ortLibName()).
case "$GOOS" in
  darwin)  ORT_LIB_NAME="libonnxruntime.dylib" ;;
  windows) ORT_LIB_NAME="onnxruntime.dll" ;;
  *)       ORT_LIB_NAME="libonnxruntime.so" ;;
esac

# ── clean staging dir ─────────────────────────────────────────────────────────
mkdir -p "$OUT_DIR"
OUT_DIR="$(cd "$OUT_DIR" && pwd)"
STAGE="$OUT_DIR/stage-${GOOS}-${GOARCH}"
rm -rf "$STAGE"
mkdir -p "$STAGE/lib"

# ── stage the plugin binary (cgo, -tags onnx) ─────────────────────────────────
# If CHECK_PREBUILT_BIN points at an already-built binary, reuse it (the release
# workflow builds + Developer-ID-signs + notarizes the macOS binary BEFORE
# packaging, then sets CHECK_PREBUILT_BIN so the signature is preserved in the
# tarball). Otherwise build here.
#
# GOWORK=off so the plugin's own go.mod (with its replace directive) governs the
# build, matching `make build-check-plugin-onnx`.
if [ -n "${CHECK_PREBUILT_BIN:-}" ]; then
  if [ ! -f "$CHECK_PREBUILT_BIN" ]; then
    echo "package-check-plugin: CHECK_PREBUILT_BIN set but not a file: $CHECK_PREBUILT_BIN" >&2
    exit 1
  fi
  echo "package-check-plugin: using pre-built (signed) binary $CHECK_PREBUILT_BIN"
  cp "$CHECK_PREBUILT_BIN" "$STAGE/${BIN_NAME}"
  chmod +x "$STAGE/${BIN_NAME}"
else
  VERSION_PKG="github.com/neokapi/neokapi/core/version"
  LDFLAGS="-s -w -X ${VERSION_PKG}.Version=${VERSION}"
  # cgo link flags. On Windows, daulet/tokenizers' static lib bundles Rust's
  # std, which calls NT native APIs exported by ntdll; daulet's own cgo LDFLAGS
  # link ws2_32/userenv but not ntdll, so mingw's single-pass linker reports
  # "undefined reference to Nt*/Rtl*". Append -lntdll via the env CGO_LDFLAGS,
  # which Go places AFTER the package's -ltokenizers in the link line — the
  # order the single-pass GNU linker requires.
  CGO_LDFLAGS_VAL="-L${TOKENIZERS_LIB}"
  if [ "$GOOS" = "windows" ]; then
    CGO_LDFLAGS_VAL="${CGO_LDFLAGS_VAL} -lntdll"
  fi
  echo "package-check-plugin: building ${BIN_NAME} ${VERSION} for ${GOOS}/${GOARCH} (-tags onnx)"
  (
    cd "$PLUGIN_DIR"
    GOWORK=off CGO_ENABLED=1 \
      CGO_LDFLAGS="${CGO_LDFLAGS_VAL}" \
      go build -tags onnx -trimpath \
        -ldflags "${LDFLAGS}" \
        -o "$STAGE/${BIN_NAME}" ./cmd/kapi-check
  )
fi

# ── stage manifest ────────────────────────────────────────────────────────────
cp "$PLUGIN_DIR/manifest.json" "$STAGE/manifest.json"

# ── stage onnxruntime shared library at lib/<unversioned-name> ────────────────
# Find the platform shared library inside the extracted onnxruntime dir. Prefer
# $ORT_DIR/lib; fall back to a recursive search. onnxruntime ships versioned
# names (libonnxruntime.1.25.0.dylib, libonnxruntime.so.1.25.0) alongside an
# unversioned alias (libonnxruntime.dylib / libonnxruntime.so); we normalise to
# the unversioned name the loader path points at. On Windows the DLL is
# unversioned already (onnxruntime.dll).
#
# Debug bundles (*.dSYM on macOS) contain a same-named Mach-O DWARF file; they
# must be excluded so we never pick the debug stub over the real library.
find_ort_lib() {
  local search="$1"
  case "$GOOS" in
    darwin)
      local unversioned
      unversioned="$(find "$search" -type f -name 'libonnxruntime.dylib' -not -path '*.dSYM/*' 2>/dev/null | head -n1)"
      if [ -n "$unversioned" ]; then
        echo "$unversioned"
      else
        find "$search" -type f -name 'libonnxruntime*.dylib' -not -path '*.dSYM/*' 2>/dev/null | sort | head -n1
      fi
      ;;
    windows)
      find "$search" -type f -iname 'onnxruntime.dll' 2>/dev/null | head -n1
      ;;
    *)
      local unversioned
      unversioned="$(find "$search" -type f -name 'libonnxruntime.so' 2>/dev/null | head -n1)"
      if [ -n "$unversioned" ]; then
        echo "$unversioned"
      else
        find "$search" -type f -name 'libonnxruntime.so.*' 2>/dev/null | sort | tail -n1
      fi
      ;;
  esac
}

ORT_SRC="$(find_ort_lib "$ORT_DIR/lib")"
if [ -z "$ORT_SRC" ]; then
  ORT_SRC="$(find_ort_lib "$ORT_DIR")"
fi
if [ -z "$ORT_SRC" ] || [ ! -f "$ORT_SRC" ]; then
  echo "package-check-plugin: could not find onnxruntime shared library under $ORT_DIR" >&2
  echo "package-check-plugin: (looked for the ${GOOS} ${ORT_LIB_NAME} family)" >&2
  exit 1
fi

echo "package-check-plugin: bundling onnxruntime: $ORT_SRC -> lib/${ORT_LIB_NAME}"
# Dereference symlinks (-L) so the tarball carries the real shared object.
cp -L "$ORT_SRC" "$STAGE/lib/${ORT_LIB_NAME}"

# macOS: the plugin calls ort.SetSharedLibraryPath() with this exact absolute
# path, so the dylib's own LC_ID_DYLIB install_name does not need rewriting for
# the loader to find it. We still normalise the id to the bundled name so any
# `otool -L` / re-sign tooling sees a self-consistent library.
if [ "$GOOS" = "darwin" ] && command -v install_name_tool >/dev/null 2>&1; then
  install_name_tool -id "@loader_path/${ORT_LIB_NAME}" "$STAGE/lib/${ORT_LIB_NAME}" 2>/dev/null || \
    echo "package-check-plugin: install_name_tool id rewrite skipped (non-fatal)"
fi

# ── tar it up ─────────────────────────────────────────────────────────────────
TARBALL="kapi-check_${VERSION}_${GOOS}_${GOARCH}.tar.gz"
TARBALL_PATH="$OUT_DIR/$TARBALL"

tar -czf "$TARBALL_PATH" -C "$STAGE" "$BIN_NAME" manifest.json lib

# ── sha256 (portable across linux/macos) ──────────────────────────────────────
if command -v sha256sum >/dev/null 2>&1; then
  SHA256="$(sha256sum "$TARBALL_PATH" | awk '{print $1}')"
else
  SHA256="$(shasum -a 256 "$TARBALL_PATH" | awk '{print $1}')"
fi
echo "$SHA256  $TARBALL" > "$TARBALL_PATH.sha256"

echo "package-check-plugin: wrote $TARBALL_PATH"
echo "package-check-plugin: sha256 $SHA256"

# Machine-readable outputs.
echo "$TARBALL_PATH"
echo "$SHA256"
if [ -n "${GITHUB_OUTPUT:-}" ]; then
  {
    echo "tarball=$TARBALL_PATH"
    echo "tarball_name=$TARBALL"
    echo "sha256=$SHA256"
  } >> "$GITHUB_OUTPUT"
fi
