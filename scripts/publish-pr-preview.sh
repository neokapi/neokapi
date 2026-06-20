#!/usr/bin/env bash
#
# Publish a PR's preview tree to the Cloudflare R2 previews prefix, so it is
# served by the neokapi-pr-previews Worker (deploy/preview-worker) at
# $DOCS_PREVIEW_URL/web/prs/<N>/… and /storybook/prs/<N>/… — instead of being
# committed into the neokapi.github.io org Pages repo (which used to bloat that
# repo to multiple GB). See deploy/preview-worker/README.md.
#
# Usage: publish-pr-preview.sh <pr-number> [assembled-root]
#   assembled-root (default ".") contains the SAME path layout pages-deploy.yml
#   builds for the Pages repo:
#     <root>/web/prs/<N>/…        <root>/storybook/prs/<N>/…
#
# Auth via env (an R2 S3-compatible API token), matching publish-cdn-assets.sh:
#   R2_BUCKET, R2_ENDPOINT, AWS_ACCESS_KEY_ID, AWS_SECRET_ACCESS_KEY
#
# Requires: aws CLI (preinstalled on GitHub runners; `brew install awscli`).

set -euo pipefail
cd "$(dirname "$0")/.."

PR="${1:-}"
ROOT="${2:-.}"
case "$PR" in
  '' | *[!0-9]*) echo "usage: publish-pr-preview.sh <pr-number> [assembled-root]" >&2; exit 2 ;;
esac

: "${R2_BUCKET:?set R2_BUCKET}"
: "${R2_ENDPOINT:?set R2_ENDPOINT (https://<account-id>.r2.cloudflarestorage.com)}"
: "${AWS_ACCESS_KEY_ID:?set AWS_ACCESS_KEY_ID (R2 access key id)}"
: "${AWS_SECRET_ACCESS_KEY:?set AWS_SECRET_ACCESS_KEY (R2 secret access key)}"
command -v aws >/dev/null || { echo "error: aws CLI not found (brew install awscli)"; exit 1; }

# R2 ignores the region but the SDK requires one; "auto" is the documented value.
export AWS_DEFAULT_REGION="${AWS_DEFAULT_REGION:-auto}"
S3=(aws s3 --endpoint-url "$R2_ENDPOINT")

published=0
for sub in "web/prs/${PR}" "storybook/prs/${PR}"; do
  if [ -d "${ROOT}/${sub}" ] && [ -n "$(ls -A "${ROOT}/${sub}" 2>/dev/null)" ]; then
    DST="s3://${R2_BUCKET}/previews/${sub}"
    echo "→ syncing ${ROOT}/${sub} → ${DST}"
    # --delete so a re-push that drops files (renamed routes, removed stories)
    # doesn't leave stale objects behind. Content types are set authoritatively
    # by the Worker, so no per-extension passes are needed here.
    "${S3[@]}" sync "${ROOT}/${sub}" "${DST}" --delete --no-progress
    published=1
  fi
done

if [ "$published" = "0" ]; then
  echo "::warning::no preview subtree found under ${ROOT} for PR #${PR}; nothing published"
fi
echo "✓ preview for PR #${PR} published to R2 previews/ prefix"
