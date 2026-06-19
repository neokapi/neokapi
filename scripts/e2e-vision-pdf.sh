#!/usr/bin/env bash
# End-to-end check of the PDF tier-3 chain: the real kapi CLI drives the
# kapi-pdfium plugin (renders each page) and the kapi-vision plugin (runs the
# layout model over the raster), and the host decorator turns the result into
# structured blocks. This is the cross-process wiring the per-module tests and
# the vision-onnx smoke lane do NOT cover (those run pdfium and vision in
# isolation). Vision model *numerics* are covered by the smoke lane; this proves
# kapi → pdfium(render) → vision(layout) → structured blocks composes for real.
#
# Required env:
#   KAPI_BIN                 path to the built kapi binary
#   KAPI_PLUGINS_DIR         dir containing pdfium/ and vision/ plugin installs
#   KAPI_VISION_ORT_LIB      path to libonnxruntime (passed through to the plugin)
#   KAPI_VISION_MODELS_DIR   dir with the 4 PP-OCRv5/PP-DocLayoutV3 model assets
#   FIXTURE_PDF              a structured PDF to read (defaults to the repo sample)
set -euo pipefail

: "${KAPI_BIN:?KAPI_BIN is required}"
: "${KAPI_PLUGINS_DIR:?KAPI_PLUGINS_DIR is required}"
: "${KAPI_VISION_ORT_LIB:?KAPI_VISION_ORT_LIB is required}"
: "${KAPI_VISION_MODELS_DIR:?KAPI_VISION_MODELS_DIR is required}"
FIXTURE_PDF="${FIXTURE_PDF:-web/static/samples/report.pdf}"

[ -f "$FIXTURE_PDF" ] || { echo "::error::fixture PDF not found: $FIXTURE_PDF"; exit 1; }
FIXTURE_PDF="$(cd "$(dirname "$FIXTURE_PDF")" && pwd)/$(basename "$FIXTURE_PDF")"

WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT
PROJ="$WORK/proj"; mkdir -p "$PROJ"
cp "$FIXTURE_PDF" "$PROJ/doc.pdf"

# Recipe: read the PDF with tier3 on (per-item format config, applied at extract).
# The pdfium plugin renders each page and emits the raster + raw blocks; the host
# vision decorator runs the layout model and emits structured blocks.
cat > "$PROJ/proj.kapi" <<'YAML'
version: v1
name: vision-pdf-e2e
defaults:
  source_language: en
  target_languages: [qps]
content:
  - path: doc.pdf
    format:
      name: pdf
      config:
        tier3: true
    target: "out/{lang}.md"
YAML

# Isolate kapi from any developer/system config; discover ONLY our staged plugins.
export KAPI_PLUGINS_DIR_ONLY=1
export KAPI_CONFIG_DIR="$WORK/config"
export XDG_DATA_HOME="$WORK/data"
export XDG_CACHE_HOME="$WORK/cache"

cd "$PROJ"
"$KAPI_BIN" init >/dev/null

echo "== tier-3 extract (pdfium render -> vision layout) =="
t0=$(date +%s)
"$KAPI_BIN" extract 2>&1 | tee "$WORK/extract.log"
t1=$(date +%s)
echo "extract took $((t1 - t0))s"

xliff="$(find . -name '*.xliff' -type f | head -1)"
[ -n "$xliff" ] || { echo "::error::no extracted XLIFF produced"; exit 1; }

units=$(grep -coE "<unit |<segment |<trans-unit " "$xliff" || true)
echo "extracted units: $units"

# The fast path (no tier-3) yields ~1 whole-page block per page; the tier-3 chain
# segments the page into many structured blocks. A low count means the chain did
# not run (plugin not found, render missing, decorator not wired, …).
if [ "${units:-0}" -lt 10 ]; then
  echo "::error::expected >=10 structured units from the tier-3 chain, got ${units:-0}"
  exit 1
fi

# Sanity: the document's real source text made it through the chain.
if ! grep -qiE "revenue|operating|margin" "$xliff"; then
  echo "::error::expected the document's source text in the extracted XLIFF"
  exit 1
fi

echo "OK: tier-3 PDF chain produced $units structured units with source text intact"
