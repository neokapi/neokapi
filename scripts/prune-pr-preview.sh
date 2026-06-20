#!/usr/bin/env bash
#
# Remove a closed PR's preview from the Cloudflare R2 previews prefix. Replaces
# the old git-based prune (which deleted files from the Pages repo's working
# tree but left every binary in git history forever). An R2 delete is instant
# and reclaims the storage for real.
#
# Usage: prune-pr-preview.sh <pr-number>
#
# Auth via env (an R2 S3-compatible API token):
#   R2_BUCKET, R2_ENDPOINT, AWS_ACCESS_KEY_ID, AWS_SECRET_ACCESS_KEY

set -euo pipefail
cd "$(dirname "$0")/.."

PR="${1:-}"
case "$PR" in
  '' | *[!0-9]*) echo "usage: prune-pr-preview.sh <pr-number>" >&2; exit 2 ;;
esac

: "${R2_BUCKET:?set R2_BUCKET}"
: "${R2_ENDPOINT:?set R2_ENDPOINT}"
: "${AWS_ACCESS_KEY_ID:?set AWS_ACCESS_KEY_ID}"
: "${AWS_SECRET_ACCESS_KEY:?set AWS_SECRET_ACCESS_KEY}"
command -v aws >/dev/null || { echo "error: aws CLI not found (brew install awscli)"; exit 1; }

export AWS_DEFAULT_REGION="${AWS_DEFAULT_REGION:-auto}"
S3=(aws s3 --endpoint-url "$R2_ENDPOINT")

for sub in "web/prs/${PR}" "storybook/prs/${PR}"; do
  DST="s3://${R2_BUCKET}/previews/${sub}"
  echo "→ removing ${DST}"
  # NB: `aws s3 rm` accepts --quiet but NOT --no-progress (that's sync/cp only).
  "${S3[@]}" rm "${DST}" --recursive --quiet || true
done
echo "✓ pruned preview for PR #${PR}"
