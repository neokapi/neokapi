#!/usr/bin/env bash
# Build + package one platform's kapi-vision plugin tarball for distribution via
# the neokapi/registry (installable with `kapi plugins install vision`).
#
# kapi-vision is built with `-tags onnx` (cgo): it loads the onnxruntime SHARED
# library at RUNTIME. To make an installed plugin work with no configuration,
# the tarball is SELF-CONTAINED:
#   - the onnxruntime shared library is bundled at lib/<unversioned-name> beside
#     the binary (where plugins/vision/internal/ocr/ort_onnx.go:resolveORTLib()
#     looks when KAPI_VISION_ORT_LIB is unset);
#   - the PP-OCRv5 model assets (det/rec/dict, ~21 MB) are bundled at models/
#     beside the binary (where internal/models.Dir() looks when
#     KAPI_VISION_MODELS_DIR is unset — see the bundled-dir resolution).
#
# Tarball layout (kapi-vision_<version>_<os>_<arch>.tar.gz):
#
#     kapi-vision[.exe]          the plugin binary, at the tarball root
#     manifest.json              the plugin manifest, at the tarball root
#     lib/libonnxruntime.dylib   onnxruntime shared lib (darwin)
#       | libonnxruntime.so      (linux)
#       | onnxruntime.dll        (windows)
#     models/ppocrv5_det.onnx    PP-OCRv5 detection
#     models/ppocrv5_rec.onnx    PP-OCRv5 recognition
#     models/ppocrv5_dict.txt    PP-OCRv5 dictionary
#
# Usage (flags or env; flags win):
#
#     scripts/package-vision-plugin.sh \
#       --version 0.1.0 \
#       --ort-dir /path/to/extracted/onnxruntime \
#       --models-dir /path/to/dir/with/ppocrv5_*.onnx+dict \
#       --out-dir /path/to/output
#
# Equivalent env vars: VERSION, ORT_DIR, MODELS_DIR, OUT_DIR.
#
# Builds for the HOST platform only (go env GOOS/GOARCH); the release matrix
# runs this once per native runner. Echoes the tarball path + sha256, and
# appends them to $GITHUB_OUTPUT (as `tarball`, `tarball_name`, `sha256`).
set -euo pipefail

VERSION="${VERSION:-}"
ORT_DIR="${ORT_DIR:-}"
MODELS_DIR="${MODELS_DIR:-}"
OUT_DIR="${OUT_DIR:-}"

while [ $# -gt 0 ]; do
  case "$1" in
    --version)     VERSION="$2"; shift 2 ;;
    --ort-dir)     ORT_DIR="$2"; shift 2 ;;
    --models-dir)  MODELS_DIR="$2"; shift 2 ;;
    --out-dir)     OUT_DIR="$2"; shift 2 ;;
    --version=*)    VERSION="${1#*=}"; shift ;;
    --ort-dir=*)    ORT_DIR="${1#*=}"; shift ;;
    --models-dir=*) MODELS_DIR="${1#*=}"; shift ;;
    --out-dir=*)    OUT_DIR="${1#*=}"; shift ;;
    -h|--help) sed -n '2,40p' "$0"; exit 0 ;;
    *) echo "package-vision-plugin: unknown argument: $1" >&2; exit 2 ;;
  esac
done

: "${VERSION:?--version (or \$VERSION) is required}"
: "${ORT_DIR:?--ort-dir (or \$ORT_DIR) is required: extracted onnxruntime dir}"
: "${MODELS_DIR:?--models-dir (or \$MODELS_DIR) is required: dir with ppocrv5_* assets}"
: "${OUT_DIR:?--out-dir (or \$OUT_DIR) is required}"

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
PLUGIN_DIR="$REPO_ROOT/plugins/vision"
[ -f "$PLUGIN_DIR/go.mod" ] || { echo "package-vision-plugin: cannot find plugins/vision under $REPO_ROOT" >&2; exit 1; }

GOOS="$(go env GOOS)"
GOARCH="$(go env GOARCH)"

BIN_NAME="kapi-vision"
[ "$GOOS" = "windows" ] && BIN_NAME="kapi-vision.exe"

case "$GOOS" in
  darwin)  ORT_LIB_NAME="libonnxruntime.dylib" ;;
  windows) ORT_LIB_NAME="onnxruntime.dll" ;;
  *)       ORT_LIB_NAME="libonnxruntime.so" ;;
esac

mkdir -p "$OUT_DIR"; OUT_DIR="$(cd "$OUT_DIR" && pwd)"
STAGE="$OUT_DIR/stage-${GOOS}-${GOARCH}"
rm -rf "$STAGE"; mkdir -p "$STAGE/lib" "$STAGE/models"

# ── build the plugin binary (cgo, -tags onnx) ─────────────────────────────────
# VISION_PREBUILT_BIN lets the release workflow inject an already-signed binary.
if [ -n "${VISION_PREBUILT_BIN:-}" ]; then
  [ -f "$VISION_PREBUILT_BIN" ] || { echo "package-vision-plugin: VISION_PREBUILT_BIN not a file: $VISION_PREBUILT_BIN" >&2; exit 1; }
  echo "package-vision-plugin: using pre-built (signed) binary $VISION_PREBUILT_BIN"
  cp "$VISION_PREBUILT_BIN" "$STAGE/${BIN_NAME}"; chmod +x "$STAGE/${BIN_NAME}"
else
  VERSION_PKG="github.com/neokapi/neokapi/core/version"
  CGO_LDFLAGS_VAL=""
  [ "$GOOS" = "windows" ] && CGO_LDFLAGS_VAL="-static"
  echo "package-vision-plugin: building ${BIN_NAME} ${VERSION} for ${GOOS}/${GOARCH} (-tags onnx)"
  (
    cd "$PLUGIN_DIR"
    GOWORK=off CGO_ENABLED=1 CGO_LDFLAGS="${CGO_LDFLAGS_VAL}" \
      go build -tags onnx -trimpath \
        -ldflags "-s -w -X ${VERSION_PKG}.Version=${VERSION}" \
        -o "$STAGE/${BIN_NAME}" ./cmd/kapi-vision
  )
fi

cp "$PLUGIN_DIR/manifest.json" "$STAGE/manifest.json"

# ── bundle the model assets (read from manifest's declared filenames) ─────────
for f in ppocrv5_det.onnx ppocrv5_rec.onnx ppocrv5_dict.txt; do
  [ -f "$MODELS_DIR/$f" ] || { echo "package-vision-plugin: missing model asset $MODELS_DIR/$f" >&2; exit 1; }
  cp "$MODELS_DIR/$f" "$STAGE/models/$f"
done

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
[ -n "$ORT_SRC" ] && [ -f "$ORT_SRC" ] || { echo "package-vision-plugin: could not find onnxruntime shared library under $ORT_DIR" >&2; exit 1; }
echo "package-vision-plugin: bundling onnxruntime: $ORT_SRC -> lib/${ORT_LIB_NAME}"
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
TARBALL="kapi-vision_${VERSION}_${GOOS}_${GOARCH}.tar.gz"
TARBALL_PATH="$OUT_DIR/$TARBALL"
tar -czf "$TARBALL_PATH" -C "$STAGE" "$BIN_NAME" manifest.json lib models

if command -v sha256sum >/dev/null 2>&1; then
  SHA256="$(sha256sum "$TARBALL_PATH" | awk '{print $1}')"
else
  SHA256="$(shasum -a 256 "$TARBALL_PATH" | awk '{print $1}')"
fi
echo "$SHA256  $TARBALL" > "$TARBALL_PATH.sha256"
echo "package-vision-plugin: wrote $TARBALL_PATH"
echo "package-vision-plugin: sha256 $SHA256"
if [ -n "${GITHUB_OUTPUT:-}" ]; then
  { echo "tarball=$TARBALL_PATH"; echo "tarball_name=$TARBALL"; echo "sha256=$SHA256"; } >> "$GITHUB_OUTPUT"
fi
