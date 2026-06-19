#!/usr/bin/env bash
#
# Publish the large, immutable docs assets to the Cloudflare R2 CDN bucket that
# backs $DOCS_CDN_URL, so the GitHub Pages artifact stays small and deploys fast.
# One bucket backs both docs sites; objects are scoped per-site (kapi/ vs
# bowrain/). See web/docs/contribute/notes-internal/cdn-assets.md.
#
# Families:
#   wasm           web/static/wasm               → s3://$R2_BUCKET/kapi/wasm/$VERSION/
#   vision-models  (vision-models-v1 release, whole — no Pages size split)
#                                                 → s3://$R2_BUCKET/kapi/models/vision/
#   video-kapi     web/static/video              → s3://$R2_BUCKET/kapi/video/
#   video-bowrain  bowrain/web/docs/static/video → s3://$R2_BUCKET/bowrain/video/
#
# The wasm family is published by CI on every docs build (it builds the binary).
# The other families are published out-of-band by the maintainer, mirroring the
# old docs-assets release flow: vision-models are pulled from the pinned
# vision-models-v1 GitHub release and re-uploaded (rerun only when that release
# changes); videos are produced on the desktop by the harness.
#
# Auth via env (an R2 S3-compatible API token):
#   R2_BUCKET                  bucket name
#   R2_ENDPOINT                https://<account-id>.r2.cloudflarestorage.com
#   AWS_ACCESS_KEY_ID          R2 access key id
#   AWS_SECRET_ACCESS_KEY      R2 secret access key
# Optional:
#   R2_VERSION                 wasm cache-bust path segment (default: git short sha)
#
# Requires: aws CLI (preinstalled on GitHub runners; `brew install awscli` locally).
# For the vision-models family: gh (authenticated) or curl.

set -euo pipefail
cd "$(dirname "$0")/.."

FAMILY="${1:-}"
case "$FAMILY" in
  wasm | vision-models | video-kapi | video-bowrain) ;;
  *)
    echo "usage: publish-cdn-assets.sh <wasm|vision-models|video-kapi|video-bowrain>" >&2
    exit 2
    ;;
esac

: "${R2_BUCKET:?set R2_BUCKET}"
: "${R2_ENDPOINT:?set R2_ENDPOINT (https://<account-id>.r2.cloudflarestorage.com)}"
: "${AWS_ACCESS_KEY_ID:?set AWS_ACCESS_KEY_ID (R2 access key id)}"
: "${AWS_SECRET_ACCESS_KEY:?set AWS_SECRET_ACCESS_KEY (R2 secret access key)}"
command -v aws >/dev/null || { echo "error: aws CLI not found (brew install awscli)"; exit 1; }

# R2 ignores the region but the SDK requires one; "auto" is the documented value.
export AWS_DEFAULT_REGION="${AWS_DEFAULT_REGION:-auto}"
S3=(aws s3 --endpoint-url "$R2_ENDPOINT")
IMMUTABLE="public, max-age=31536000, immutable"
VIDEO_CACHE="public, max-age=86400"
VERSION="${R2_VERSION:-$(git rev-parse --short HEAD 2>/dev/null || echo dev)}"

case "$FAMILY" in
  wasm)
    SRC="web/static/wasm"
    [ -d "$SRC" ] || { echo "error: $SRC missing — run 'make web-wasm-demo web-wasm-cli web-pdfium-wasm'"; exit 1; }
    DST="s3://$R2_BUCKET/kapi/wasm/$VERSION"
    echo "→ syncing $SRC → $DST (immutable, version $VERSION)…"
    # Two passes so the wasm binaries get the right Content-Type.
    # 1) Raw wasm binaries (kapi-cli.wasm, kapi.wasm, pdfium.wasm).
    "${S3[@]}" sync "$SRC" "$DST" --exclude '*' --include '*.wasm' \
      --cache-control "$IMMUTABLE" --content-type "application/wasm"
    # 2) Everything else — the pre-gzipped wasm (kapi-cli.wasm.gz) and the JS glue
    #    (wasm_exec.js). The pre-gzipped file is served as an OPAQUE blob with NO
    #    Content-Encoding: the runtime fetches `${wasmUrl}.gz` and self-inflates it
    #    via DecompressionStream (fetchWasmBytes in runtime.ts), exactly as it does
    #    on GitHub Pages. Setting Content-Encoding here would make the browser
    #    double-inflate and silently fall back to the 71 MB raw binary.
    "${S3[@]}" sync "$SRC" "$DST" --exclude '*.wasm' \
      --cache-control "$IMMUTABLE"
    echo "✓ wasm published → $DST"
    ;;

  vision-models)
    # Publish the WHOLE models (R2 has no 100 MB/file limit, so the ~132 MB
    # layout model needs no split + manifest — the browser fetchModel() handles
    # either form). Pulled from the pinned vision-models-v1 release.
    REL="https://github.com/neokapi/neokapi/releases/download/vision-models-v1"
    TMP="$(mktemp -d)"; trap 'rm -rf "$TMP"' EXIT
    for f in ppocrv5_det.onnx ppocrv5_rec.onnx ppocrv5_dict.txt ppdoclayoutv3.onnx; do
      echo "→ fetching ${f}…"
      gh release download vision-models-v1 -p "$f" -O "$TMP/$f" 2>/dev/null \
        || curl -sSfL -o "$TMP/$f" "$REL/$f"
    done
    DST="s3://$R2_BUCKET/kapi/models/vision"
    echo "→ syncing models → $DST (immutable)…"
    "${S3[@]}" sync "$TMP" "$DST" --cache-control "$IMMUTABLE"
    echo "✓ vision models published (whole, no split) → $DST"
    ;;

  video-kapi)
    SRC="web/static/video"
    [ -d "$SRC" ] || { echo "error: $SRC missing — run 'make fetch-docs-assets' or render via harness"; exit 1; }
    DST="s3://$R2_BUCKET/kapi/video"
    echo "→ syncing $SRC → ${DST}…"
    "${S3[@]}" sync "$SRC" "$DST" --cache-control "$VIDEO_CACHE"
    echo "✓ kapi videos published → $DST"
    ;;

  video-bowrain)
    SRC="bowrain/web/docs/static/video"
    [ -d "$SRC" ] || { echo "error: $SRC missing — run 'make fetch-bowrain-docs-assets' or render via harness"; exit 1; }
    DST="s3://$R2_BUCKET/bowrain/video"
    echo "→ syncing $SRC → ${DST}…"
    "${S3[@]}" sync "$SRC" "$DST" --cache-control "$VIDEO_CACHE"
    echo "✓ bowrain videos published → $DST"
    ;;
esac
