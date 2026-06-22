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

# `aws s3 sync` against R2 occasionally drops individual PUTs under high
# concurrency — silently, so the deploy stays green while a handful of objects
# (e.g. a lazy JS chunk) never land, and the site then 404s that chunk at runtime
# ("Loading chunk failed" / "This page crashed"). Cap concurrency to make PUTs
# more reliable; syncOne below then verifies the EXACT set of objects landed
# (not just the count — a count can match while specific files are missing) and
# re-uploads precisely the missing ones, failing loud if it still can't.
aws configure set default.s3.max_concurrent_requests 6 2>/dev/null || true

# remoteKeys lists the object keys under a dst prefix, relative to that prefix
# (so they line up with `find`-relative local paths).
remoteKeys() {
  local dst="$1" prefix
  prefix="${dst#s3://"$R2_BUCKET"/}/" # e.g. previews/web/prs/976/
  "${S3[@]}" ls --recursive "$dst" 2>/dev/null | awk '{print $4}' |
    grep -v '/$' | sed "s|^${prefix}||" | sort
}

# syncOne mirrors a local subtree to R2 and verifies completeness by SET, not
# count. Pass 1 uses --delete (prune stale objects from earlier builds); then it
# diffs the local file set against the remote key set and, for any objects still
# missing, re-uploads exactly those with `cp` (a single targeted PUT is reliable
# where a bulk concurrent sync silently dropped it). Repeats a few times; if any
# object is still missing it exits non-zero so the partial deploy goes red rather
# than serving a site that 404s a chunk.
syncOne() {
  local src="$1" dst="$2" attempt missing rel
  local localKeys
  localKeys=$( (cd "$src" && find . -type f | sed 's|^\./||') | sort )
  for attempt in 1 2 3 4; do
    if [ "$attempt" -eq 1 ]; then
      "${S3[@]}" sync "$src" "$dst" --delete --no-progress
    fi
    missing=$(comm -23 <(printf '%s\n' "$localKeys") <(remoteKeys "$dst"))
    if [ -z "$missing" ]; then
      echo "  ✓ ${dst}: all $(printf '%s\n' "$localKeys" | grep -c .) objects present"
      return 0
    fi
    echo "::warning::preview sync ${dst}: $(printf '%s\n' "$missing" | grep -c .) object(s) missing after attempt ${attempt}; re-uploading them"
    while IFS= read -r rel; do
      [ -n "$rel" ] && "${S3[@]}" cp "$src/$rel" "$dst/$rel" --no-progress
    done <<< "$missing"
  done
  echo "::error::preview sync for ${dst} still missing objects after retries:"
  printf '  %s\n' $missing >&2
  return 1
}

published=0
for sub in "web/prs/${PR}" "storybook/prs/${PR}"; do
  if [ -d "${ROOT}/${sub}" ] && [ -n "$(ls -A "${ROOT}/${sub}" 2>/dev/null)" ]; then
    DST="s3://${R2_BUCKET}/previews/${sub}"
    echo "→ syncing ${ROOT}/${sub} → ${DST}"
    # Content types are set authoritatively by the Worker, so no per-extension
    # passes are needed here.
    syncOne "${ROOT}/${sub}" "${DST}"
    published=1
  fi
done

if [ "$published" = "0" ]; then
  echo "::warning::no preview subtree found under ${ROOT} for PR #${PR}; nothing published"
fi
echo "✓ preview for PR #${PR} published to R2 previews/ prefix"
