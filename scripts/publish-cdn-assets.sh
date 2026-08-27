#!/usr/bin/env bash
#
# Publish the large, immutable docs assets to the S3 CDN origin bucket that backs
# $DOCS_CDN_URL (cdn.<domain>, fronted by CloudFront), so the GitHub Pages
# artifact stays small and deploys fast. One bucket backs both docs sites;
# objects are scoped per-site (kapi/ vs bowrain/).
# See web/docs/contribute/implementation/repo/cdn-assets.md.
#
# Families:
#   wasm           web/static/wasm               → s3://$CDN_BUCKET/kapi/wasm/$VERSION/
#   vision-models  (vision-models-v1 release, whole — no Pages size split)
#                                                 → s3://$CDN_BUCKET/kapi/models/vision/
#   video-kapi     web/static/video              → s3://$CDN_BUCKET/kapi/video/
#   video-bowrain  bowrain/web/docs/static/video → s3://$CDN_BUCKET/bowrain/video/
#
# The wasm family is published by CI on every push-to-main docs build (it builds
# the binary). The other families are published out-of-band by the maintainer,
# mirroring the old docs-assets release flow: vision-models are pulled from the
# pinned vision-models-v1 GitHub release and re-uploaded (rerun only when that
# release changes); videos are produced on the desktop by the harness.
#
# Auth via env:
#   CDN_BUCKET                 S3 CDN origin bucket (bowrain-<env>-cdn-<region>-<acct>)
#   AWS credentials            from the environment's default chain — the GitHub
#                              Actions OIDC deploy role in CI (set by
#                              configure-aws-credentials), or the maintainer's
#                              `aws sso login` profile locally.
# Optional:
#   CDN_VERSION                wasm cache-bust path segment (default: git short sha)
#   AWS_REGION                 CDN bucket region (default: eu-north-1)
#
# Requires: aws CLI (preinstalled on GitHub runners; `brew install awscli` locally).
# For the vision-models family: gh (authenticated) or curl.

set -euo pipefail
cd "$(dirname "$0")/.."

FAMILY="${1:-}"
case "$FAMILY" in
  wasm | vision-models | video-kapi | video-bowrain | icu | images-kapi | images-bowrain | eval-artifacts) ;;
  *)
    echo "usage: publish-cdn-assets.sh <wasm|vision-models|video-kapi|video-bowrain|icu|images-kapi|images-bowrain|eval-artifacts>" >&2
    exit 2
    ;;
esac

: "${CDN_BUCKET:?set CDN_BUCKET (S3 CDN origin bucket, e.g. bowrain-prod-cdn-<region>-<acct>)}"
command -v aws >/dev/null || { echo "error: aws CLI not found (brew install awscli)"; exit 1; }

# Native S3 — credentials come from the environment's default chain (the OIDC
# deploy role in CI, or an `aws sso login` profile locally).
export AWS_DEFAULT_REGION="${AWS_REGION:-${AWS_DEFAULT_REGION:-eu-north-1}}"
S3=(aws s3)
IMMUTABLE="public, max-age=31536000, immutable"
VIDEO_CACHE="public, max-age=86400"
VERSION="${CDN_VERSION:-$(git rev-parse --short HEAD 2>/dev/null || echo dev)}"
# Vision model-set version (path segment under kapi/models/vision/). Pinned in
# web/models.version; overridable via env for ad-hoc publishes.
MODELS_VERSION="${DOCS_VISION_MODELS_VERSION:-$(cat web/models.version 2>/dev/null || echo v1)}"

case "$FAMILY" in
  wasm)
    SRC="web/static/wasm"
    [ -d "$SRC" ] || { echo "error: $SRC missing — run 'make web-wasm-demo web-wasm-cli web-pdfium-wasm'"; exit 1; }
    DST="s3://$CDN_BUCKET/kapi/wasm/$VERSION"
    echo "→ syncing $SRC → $DST (immutable, version $VERSION)…"
    # Three passes so each object gets the right Content-Type (aws s3's guess for
    # .wasm / .wasm.gz is wrong for the browser).
    # 1) Raw wasm binaries (kapi-cli.wasm, kapi.wasm, pdfium.wasm).
    "${S3[@]}" sync "$SRC" "$DST" --exclude '*' --include '*.wasm' \
      --cache-control "$IMMUTABLE" --content-type "application/wasm"
    # 2) The pre-gzipped wasm (kapi-cli.wasm.gz) — served as an OPAQUE
    #    application/wasm blob with NO Content-Encoding: the runtime fetches
    #    `${wasmUrl}.gz` and self-inflates it via DecompressionStream
    #    (fetchWasmBytes in runtime.ts), exactly as it does on GitHub Pages.
    #    Setting Content-Encoding here would make the browser double-inflate and
    #    silently fall back to the ~76 MB raw binary.
    "${S3[@]}" sync "$SRC" "$DST" --exclude '*' --include '*.wasm.gz' \
      --cache-control "$IMMUTABLE" --content-type "application/wasm"
    # 3) The JS glue (wasm_exec.js) — let the CLI guess text/javascript.
    "${S3[@]}" sync "$SRC" "$DST" --exclude '*.wasm' --exclude '*.wasm.gz' \
      --cache-control "$IMMUTABLE"
    echo "✓ wasm published → $DST"
    ;;

  vision-models)
    # Publish the WHOLE models (S3 has no small per-object limit, so the ~132 MB
    # layout model needs no split + manifest — the browser fetchModel() handles
    # either form). Pulled from the pinned vision-models-v1 release.
    REL="https://github.com/neokapi/neokapi/releases/download/vision-models-v1"
    TMP="$(mktemp -d)"; trap 'rm -rf "$TMP"' EXIT
    for f in ppocrv5_det.onnx ppocrv5_rec.onnx ppocrv5_dict.txt ppdoclayoutv3.onnx; do
      echo "→ fetching ${f}…"
      gh release download vision-models-v1 -p "$f" -O "$TMP/$f" 2>/dev/null \
        || curl -sSfL -o "$TMP/$f" "$REL/$f"
    done
    DST="s3://$CDN_BUCKET/kapi/models/vision/$MODELS_VERSION"
    echo "→ syncing models → $DST (immutable, version $MODELS_VERSION)…"
    "${S3[@]}" sync "$TMP" "$DST" --cache-control "$IMMUTABLE"
    echo "✓ vision models published (whole, no split) → $DST"
    ;;

  video-kapi)
    SRC="web/static/video"
    [ -d "$SRC" ] || { echo "error: $SRC missing — render via harness (make harness-videos)"; exit 1; }
    DST="s3://$CDN_BUCKET/kapi/video"
    echo "→ syncing $SRC → ${DST}…"
    "${S3[@]}" sync "$SRC" "$DST" --cache-control "$VIDEO_CACHE"
    echo "✓ kapi videos published → $DST"
    ;;

  video-bowrain)
    SRC="bowrain/web/docs/static/video"
    [ -d "$SRC" ] || { echo "error: $SRC missing — render via harness (make harness-videos-staged)"; exit 1; }
    DST="s3://$CDN_BUCKET/bowrain/video"
    echo "→ syncing $SRC → ${DST}…"
    "${S3[@]}" sync "$SRC" "$DST" --cache-control "$VIDEO_CACHE"
    echo "✓ bowrain videos published → $DST"
    ;;

  icu)
    # ICU4X wasm (Segmentation Lab) — the `icu` npm package's prebuilt wasm,
    # versioned by package version so the path is immutable. Served with
    # Content-Type application/wasm so the lab's instantiateStreaming accepts it.
    # `icu`'s exports block require.resolve, so read the manifest by path (the
    # pnpm web/node_modules/icu symlink). require() of an explicit .json path is
    # not gated by the package's exports map.
    ICU_ROOT="web/node_modules/icu"
    SRC="$ICU_ROOT/icu_capi.wasm"
    [ -f "$SRC" ] || { echo "error: $SRC not found — run 'vp install'"; exit 1; }
    ICU_VER="$(node -e "process.stdout.write(require('./$ICU_ROOT/package.json').version)")"
    DST="s3://$CDN_BUCKET/kapi/icu/$ICU_VER"
    echo "→ uploading $SRC → ${DST}/icu_capi.wasm (immutable, application/wasm)…"
    "${S3[@]}" cp "$SRC" "$DST/icu_capi.wasm" \
      --cache-control "$IMMUTABLE" --content-type "application/wasm"
    echo "✓ icu4x wasm published → $DST/icu_capi.wasm"
    ;;

  # Screenshots (ThemedImage) are large and release-only (gitignored), so they
  # live on the CDN like videos rather than in git or the Pages artifact. Synced
  # whole; ThemedImage references them by URL when the CDN is on. The committed
  # small images a site references same-origin are a harmless superset here.
  images-kapi)
    SRC="web/static/img"
    [ -d "$SRC" ] || { echo "error: $SRC missing"; exit 1; }
    DST="s3://$CDN_BUCKET/kapi/img"
    echo "→ syncing $SRC → ${DST}…"
    "${S3[@]}" sync "$SRC" "$DST" --cache-control "$VIDEO_CACHE"
    echo "✓ kapi images published → $DST"
    ;;

  # What the agents in the skill eval actually produced: the translated .docx,
  # the rewritten .pptx, the catalog. The dataset can only carry a byte count
  # for these, and a byte count is the weakest form of the claim — a reader who
  # can open the document can check faithful write-back against their own idea
  # of it, and the control arm's version sits beside it.
  #
  # Staged by the sweep, gitignored, and about 50MB a run. Not immutable: a
  # re-run overwrites the same keys, so these get the same day-long cache the
  # videos use rather than a year.
  #
  # --delete, because a scenario that is renamed or retired leaves its old tree
  # behind otherwise, and a page linking a document from a scenario that no
  # longer exists is worse than a missing link.
  eval-artifacts)
    # Both halves of a sweep's evidence: the documents the agents produced, and
    # the full session transcripts. The transcripts were committed and capped
    # until the caps were found to cut the part a reader wants — the file the
    # agent read, the error it got. Uncapped they are too large for git.
    SRC="web/static/skill-eval"
    [ -d "$SRC" ] || { echo "error: $SRC missing — run a sweep first (make skill-eval-completion)"; exit 1; }
    DST="s3://$CDN_BUCKET/kapi/skill-eval"
    echo "→ syncing $SRC → ${DST}…"
    "${S3[@]}" sync "$SRC" "$DST" --cache-control "$VIDEO_CACHE" --delete
    echo "✓ skill-eval artefacts published → $DST"
    ;;

  images-bowrain)
    SRC="bowrain/web/docs/static/img"
    [ -d "$SRC" ] || { echo "error: $SRC missing"; exit 1; }
    DST="s3://$CDN_BUCKET/bowrain/img"
    echo "→ syncing $SRC → ${DST}…"
    "${S3[@]}" sync "$SRC" "$DST" --cache-control "$VIDEO_CACHE"
    echo "✓ bowrain images published → $DST"
    ;;
esac
