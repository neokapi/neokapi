#!/usr/bin/env bash
# Build + package one platform's kapi-llm plugin tarball for distribution via the
# neokapi/registry (installable with `kapi plugins install llm`).
#
# kapi-llm is built with `-tags onnx` (cgo): it loads the onnxruntime SHARED
# library at RUNTIME and links the daulet/tokenizers STATIC library at build
# time. To make an installed plugin work with no configuration, the tarball
# bundles the onnxruntime shared library at lib/<unversioned-name> beside the
# binary (where plugins/llm/internal/llm/ort_onnx.go:resolveORTLib() looks when
# KAPI_LLM_ORT_LIB is unset).
#
# The Gemma 4 model files (~GBs) are NOT bundled — they are downloaded on demand
# into an XDG cache on first use (see plugins/llm/internal/model), so the tarball
# stays small.
#
# Tarball layout (kapi-llm_<version>_<os>_<arch>.tar.gz):
#
#     kapi-llm[.exe]             the plugin binary, at the tarball root
#     manifest.json              the plugin manifest, at the tarball root
#     lib/libonnxruntime.dylib   onnxruntime shared lib (darwin)
#       | libonnxruntime.so      (linux)
#       | onnxruntime.dll        (windows)
#
# Usage (flags or env; flags win):
#
#     scripts/package-llm-plugin.sh \
#       --version 0.1.0 \
#       --ort-dir /path/to/extracted/onnxruntime \
#       --tokenizers-lib /path/to/dir/with/libtokenizers.a \
#       --out-dir /path/to/output
#
# Equivalent env vars: VERSION, ORT_DIR, TOKENIZERS_LIB, OUT_DIR.
#
# Builds for the HOST platform only (go env GOOS/GOARCH); the release matrix runs
# this once per native runner. Echoes the tarball path + sha256, and appends them
# to $GITHUB_OUTPUT (as `tarball`, `tarball_name`, `sha256`).
set -euo pipefail

VERSION="${VERSION:-}"
ORT_DIR="${ORT_DIR:-}"
TOKENIZERS_LIB="${TOKENIZERS_LIB:-}"
OUT_DIR="${OUT_DIR:-}"

while [ $# -gt 0 ]; do
  case "$1" in
    --version)         VERSION="$2"; shift 2 ;;
    --ort-dir)         ORT_DIR="$2"; shift 2 ;;
    --tokenizers-lib)  TOKENIZERS_LIB="$2"; shift 2 ;;
    --out-dir)         OUT_DIR="$2"; shift 2 ;;
    --version=*)        VERSION="${1#*=}"; shift ;;
    --ort-dir=*)        ORT_DIR="${1#*=}"; shift ;;
    --tokenizers-lib=*) TOKENIZERS_LIB="${1#*=}"; shift ;;
    --out-dir=*)        OUT_DIR="${1#*=}"; shift ;;
    -h|--help) sed -n '2,40p' "$0"; exit 0 ;;
    *) echo "package-llm-plugin: unknown argument: $1" >&2; exit 2 ;;
  esac
done

: "${VERSION:?--version (or \$VERSION) is required}"
: "${ORT_DIR:?--ort-dir (or \$ORT_DIR) is required: extracted onnxruntime dir}"
: "${TOKENIZERS_LIB:?--tokenizers-lib (or \$TOKENIZERS_LIB) is required: dir with libtokenizers.a}"
: "${OUT_DIR:?--out-dir (or \$OUT_DIR) is required}"

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
PLUGIN_DIR="$REPO_ROOT/plugins/llm"
[ -f "$PLUGIN_DIR/go.mod" ] || { echo "package-llm-plugin: cannot find plugins/llm under $REPO_ROOT" >&2; exit 1; }

GOOS="$(go env GOOS)"
GOARCH="$(go env GOARCH)"

BIN_NAME="kapi-llm"
[ "$GOOS" = "windows" ] && BIN_NAME="kapi-llm.exe"

case "$GOOS" in
  darwin)  ORT_LIB_NAME="libonnxruntime.dylib" ;;
  windows) ORT_LIB_NAME="onnxruntime.dll" ;;
  *)       ORT_LIB_NAME="libonnxruntime.so" ;;
esac

mkdir -p "$OUT_DIR"; OUT_DIR="$(cd "$OUT_DIR" && pwd)"
STAGE="$OUT_DIR/stage-${GOOS}-${GOARCH}"
rm -rf "$STAGE"; mkdir -p "$STAGE/lib"

# ── build the plugin binary (cgo, -tags onnx) ─────────────────────────────────
# LLM_PREBUILT_BIN lets the release workflow inject an already-signed binary.
if [ -n "${LLM_PREBUILT_BIN:-}" ]; then
  [ -f "$LLM_PREBUILT_BIN" ] || { echo "package-llm-plugin: LLM_PREBUILT_BIN not a file: $LLM_PREBUILT_BIN" >&2; exit 1; }
  echo "package-llm-plugin: using pre-built (signed) binary $LLM_PREBUILT_BIN"
  cp "$LLM_PREBUILT_BIN" "$STAGE/${BIN_NAME}"; chmod +x "$STAGE/${BIN_NAME}"
else
  VERSION_PKG="github.com/neokapi/neokapi/core/version"
  CGO_LDFLAGS_VAL="-L${TOKENIZERS_LIB}"
  # On windows-gnu the rust tokenizers-ffi static lib pulls in std symbols
  # (RtlNtStatusToDosError, Nt*/Rtl*) exported by ntdll; daulet's own cgo LDFLAGS
  # link ws2_32/userenv but not ntdll, so mingw's single-pass linker reports
  # "undefined reference". Append -lntdll explicitly (mirrors package-sat-plugin.sh).
  [ "$GOOS" = "windows" ] && CGO_LDFLAGS_VAL="${CGO_LDFLAGS_VAL} -static -lntdll"
  echo "package-llm-plugin: building ${BIN_NAME} ${VERSION} for ${GOOS}/${GOARCH} (-tags onnx)"
  (
    cd "$PLUGIN_DIR"
    GOWORK=off CGO_ENABLED=1 CGO_LDFLAGS="${CGO_LDFLAGS_VAL}" \
      go build -tags onnx -trimpath \
        -ldflags "-s -w -X ${VERSION_PKG}.Version=${VERSION}" \
        -o "$STAGE/${BIN_NAME}" ./cmd/kapi-llm
  )
fi

cp "$PLUGIN_DIR/manifest.json" "$STAGE/manifest.json"
# Stamp the release version into the staged manifest so the published plugin
# version matches its tag (manifest.json's source value is a dev default). Only
# the top-level "version" (2-space indent) is rewritten, not nested model versions.
sed 's/^  "version": *"[^"]*"/  "version": "'"$VERSION"'"/' "$STAGE/manifest.json" > "$STAGE/manifest.json.tmp" \
  && mv "$STAGE/manifest.json.tmp" "$STAGE/manifest.json"

# ── bundle onnxruntime shared library at lib/<unversioned-name> ───────────────
find_ort_lib() {
  local search="$1"
  case "$GOOS" in
    darwin)
      local u; u="$(find "$search" -type f -name 'libonnxruntime.dylib' -not -path '*.dSYM/*' 2>/dev/null | head -n1)"
      [ -n "$u" ] && echo "$u" || find "$search" -type f -name 'libonnxruntime*.dylib' -not -path '*.dSYM/*' 2>/dev/null | sort | head -n1 ;;
    windows) find "$search" -type f -iname 'onnxruntime.dll' 2>/dev/null | head -n1 ;;
    *)
      local u; u="$(find "$search" -type f -name 'libonnxruntime.so' 2>/dev/null | head -n1)"
      [ -n "$u" ] && echo "$u" || find "$search" -type f -name 'libonnxruntime.so.*' 2>/dev/null | sort | tail -n1 ;;
  esac
}
ORT_SRC="$(find_ort_lib "$ORT_DIR/lib")"; [ -z "$ORT_SRC" ] && ORT_SRC="$(find_ort_lib "$ORT_DIR")"
[ -n "$ORT_SRC" ] && [ -f "$ORT_SRC" ] || { echo "package-llm-plugin: could not find onnxruntime shared library under $ORT_DIR" >&2; exit 1; }
echo "package-llm-plugin: bundling onnxruntime: $ORT_SRC -> lib/${ORT_LIB_NAME}"
cp -L "$ORT_SRC" "$STAGE/lib/${ORT_LIB_NAME}"
if [ "$GOOS" = "darwin" ] && command -v install_name_tool >/dev/null 2>&1; then
  # Only rewrite when the id isn't already correct: install_name_tool invalidates
  # any code signature, and the release workflow pre-sets this id before
  # Developer-ID signing the dylib. Skipping the no-op preserves that signature.
  cur="$(otool -D "$STAGE/lib/${ORT_LIB_NAME}" 2>/dev/null | tail -n1 | tr -d '[:space:]')"
  if [ "$cur" != "@loader_path/${ORT_LIB_NAME}" ]; then
    install_name_tool -id "@loader_path/${ORT_LIB_NAME}" "$STAGE/lib/${ORT_LIB_NAME}" 2>/dev/null || true
  fi
fi

# ── tar + sha256 ──────────────────────────────────────────────────────────────
TARBALL="kapi-llm_${VERSION}_${GOOS}_${GOARCH}.tar.gz"
TARBALL_PATH="$OUT_DIR/$TARBALL"
tar -czf "$TARBALL_PATH" -C "$STAGE" "$BIN_NAME" manifest.json lib

if command -v sha256sum >/dev/null 2>&1; then
  SHA256="$(sha256sum "$TARBALL_PATH" | awk '{print $1}')"
else
  SHA256="$(shasum -a 256 "$TARBALL_PATH" | awk '{print $1}')"
fi
echo "$SHA256  $TARBALL" > "$TARBALL_PATH.sha256"
echo "package-llm-plugin: wrote $TARBALL_PATH"
echo "package-llm-plugin: sha256 $SHA256"
if [ -n "${GITHUB_OUTPUT:-}" ]; then
  { echo "tarball=$TARBALL_PATH"; echo "tarball_name=$TARBALL"; echo "sha256=$SHA256"; } >> "$GITHUB_OUTPUT"
fi
