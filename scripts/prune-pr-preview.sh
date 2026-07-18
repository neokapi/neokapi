#!/usr/bin/env bash
#
# Remove a closed PR's preview from the AWS S3 preview bucket (and invalidate its
# paths on the preview.<domain> CloudFront distribution). Replaces the old
# git-based prune (which deleted files from the Pages repo's working tree but left
# every binary in git history forever) and the interim Cloudflare R2 delete. An S3
# delete is instant and reclaims the storage for real. See deploy/preview-site/README.md.
#
# Usage: prune-pr-preview.sh <pr-number>
#
# Auth via the environment's default AWS credential chain — the GitHub Actions
# OIDC preview role in CI, or an `aws sso login` profile locally.
#
# Env:
#   PREVIEW_BUCKET            S3 preview origin bucket
#   PREVIEW_DISTRIBUTION_ID   (optional) CloudFront distribution to invalidate
#   AWS_REGION                (optional) bucket region (default eu-north-1)

set -euo pipefail
cd "$(dirname "$0")/.."

PR="${1:-}"
case "$PR" in
  '' | *[!0-9]*) echo "usage: prune-pr-preview.sh <pr-number>" >&2; exit 2 ;;
esac

: "${PREVIEW_BUCKET:?set PREVIEW_BUCKET (S3 preview origin bucket)}"
command -v aws >/dev/null || { echo "error: aws CLI not found (brew install awscli)"; exit 1; }

export AWS_DEFAULT_REGION="${AWS_REGION:-${AWS_DEFAULT_REGION:-eu-north-1}}"

paths=()
for sub in "web/prs/${PR}" "storybook/prs/${PR}"; do
  DST="s3://${PREVIEW_BUCKET}/${sub}"
  echo "→ removing ${DST}"
  # NB: `aws s3 rm` accepts --quiet but NOT --no-progress (that's sync/cp only).
  aws s3 rm "${DST}" --recursive --quiet || true
  paths+=("/${sub}/*")
done

# Purge the closed PR's paths from the edge so a stale copy isn't served from
# cache after the objects are gone. Best-effort — the delete above is the real
# teardown; a failed invalidation just means the edge ages out on its own TTL.
if [ -n "${PREVIEW_DISTRIBUTION_ID:-}" ]; then
  aws cloudfront create-invalidation \
    --distribution-id "$PREVIEW_DISTRIBUTION_ID" \
    --paths "${paths[@]}" >/dev/null \
    && echo "→ invalidated ${paths[*]}" \
    || echo "::warning::CloudFront invalidation failed (objects still deleted)"
fi

echo "✓ pruned preview for PR #${PR}"
