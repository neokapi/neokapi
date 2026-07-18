#!/usr/bin/env bash
#
# Publish a PR's preview tree to the AWS S3 preview bucket, served by the
# preview.<domain> CloudFront distribution at $DOCS_PREVIEW_URL/web/prs/<N>/… and
# /storybook/prs/<N>/… — instead of being committed into the neokapi.github.io
# org Pages repo (which used to bloat that repo to multiple GB). PR previews used
# to live in a Cloudflare R2 bucket fronted by a Worker; that host broke when the
# bowrain.cloud zone moved to Route53, so previews now ride the same S3+CloudFront
# stack as the rest of the platform. See deploy/preview-site/README.md.
#
# Usage: publish-pr-preview.sh <pr-number> [assembled-root]
#   assembled-root (default ".") contains the SAME path layout pages-deploy.yml
#   builds for the Pages repo:
#     <root>/web/prs/<N>/…        <root>/storybook/prs/<N>/…
#
# Auth via the environment's default AWS credential chain — the GitHub Actions
# OIDC preview role in CI (set by aws-actions/configure-aws-credentials), or an
# `aws sso login` profile locally.
#
# Env:
#   PREVIEW_BUCKET            S3 preview origin bucket (bowrain-<env>-preview-<region>-<acct>)
#   PREVIEW_DISTRIBUTION_ID   (optional) CloudFront distribution to invalidate
#                             the PR's paths on after the sync, so a re-pushed
#                             preview shows immediately
#   AWS_REGION                (optional) bucket region (default eu-north-1)
#
# Requires: aws CLI (preinstalled on GitHub runners; `brew install awscli`).

set -euo pipefail
cd "$(dirname "$0")/.."

PR="${1:-}"
ROOT="${2:-.}"
case "$PR" in
  '' | *[!0-9]*) echo "usage: publish-pr-preview.sh <pr-number> [assembled-root]" >&2; exit 2 ;;
esac

: "${PREVIEW_BUCKET:?set PREVIEW_BUCKET (S3 preview origin bucket)}"
command -v aws >/dev/null || { echo "error: aws CLI not found (brew install awscli)"; exit 1; }

# Native S3 — credentials come from the environment's default chain (the OIDC
# preview role in CI, or an `aws sso login` profile locally).
export AWS_DEFAULT_REGION="${AWS_REGION:-${AWS_DEFAULT_REGION:-eu-north-1}}"

# syncTree mirrors one preview subtree to S3 with --delete (so a removed page or
# renamed chunk from an earlier build of the SAME PR is pruned), then fixes the
# handful of objects whose Content-Type aws s3 guesses wrong. aws s3 guesses
# every text/image type correctly (.html→text/html, .css, .js, .svg, …); the only
# miss is WebAssembly — .wasm and the pre-gzipped .wasm.gz — which it stores as
# application/octet-stream. The runtime's instantiateStreaming path needs
# application/wasm, so a server-side self-copy (no re-upload) rewrites just those.
# The pre-gzipped .wasm.gz is deliberately served as an OPAQUE application/wasm
# blob with NO Content-Encoding — the runtime fetches `${wasmUrl}.gz` and
# self-inflates via DecompressionStream; a Content-Encoding: gzip would make the
# browser double-inflate. Same contract as publish-cdn-assets.sh / GitHub Pages.
syncTree() {
  local src="$1" dst="$2"
  aws s3 sync "$src" "$dst" --delete --no-progress
  # --recursive self-copy with REPLACE metadata; a no-match set is a no-op.
  aws s3 cp "$dst" "$dst" --recursive --metadata-directive REPLACE \
    --exclude '*' --include '*.wasm' --include '*.wasm.gz' \
    --content-type 'application/wasm' \
    --cache-control 'public, max-age=86400' --no-progress
}

published=0
paths=()
for sub in "web/prs/${PR}" "storybook/prs/${PR}"; do
  if [ -d "${ROOT}/${sub}" ] && [ -n "$(ls -A "${ROOT}/${sub}" 2>/dev/null)" ]; then
    DST="s3://${PREVIEW_BUCKET}/${sub}"
    echo "→ syncing ${ROOT}/${sub} → ${DST}"
    syncTree "${ROOT}/${sub}" "${DST}"
    count=$(aws s3 ls --recursive "${DST}/" | grep -vc '/$' || true)
    echo "  ✓ ${DST}: ${count} objects"
    paths+=("/${sub}/*")
    published=1
  fi
done

if [ "$published" = "0" ]; then
  echo "::warning::no preview subtree found under ${ROOT} for PR #${PR}; nothing published"
  exit 0
fi

# Invalidate just this PR's paths so a re-pushed preview is served fresh
# regardless of edge cache TTL. Best-effort: a missing invalidation permission or
# a transient error must not fail an otherwise-successful publish.
if [ -n "${PREVIEW_DISTRIBUTION_ID:-}" ]; then
  echo "→ invalidating ${paths[*]} on ${PREVIEW_DISTRIBUTION_ID}"
  aws cloudfront create-invalidation \
    --distribution-id "$PREVIEW_DISTRIBUTION_ID" \
    --paths "${paths[@]}" >/dev/null \
    && echo "  ✓ invalidation created" \
    || echo "::warning::CloudFront invalidation failed (preview still published)"
fi

echo "✓ preview for PR #${PR} published to s3://${PREVIEW_BUCKET}"
