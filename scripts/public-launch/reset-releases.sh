#!/usr/bin/env bash
#
# reset-releases.sh — delete old version releases + tags across the neokapi
# repos so a clean 1.0.0 baseline can be cut for the public launch.
#
# SAFETY MODEL
#   - DRY RUN by default. Pass --execute to actually delete.
#   - Only deletes git tags that look like SEMVER releases (v<MAJOR>.<MINOR>.<PATCH>
#     with optional -rcN / sat- prefix). Named, non-version "asset" releases are
#     NEVER matched, and an explicit KEEP list is honoured on top of that.
#   - Run this ONLY AFTER the new v1.0.0 (and sat-v1.0.0 / okapi-bridge v1.0.0)
#     have been cut and verified — otherwise the products are left with no
#     installable release.
#
# Requires: gh (authenticated with delete:repo-content / repo scope).
#
# Usage:
#   scripts/public-launch/reset-releases.sh                 # dry run, all repos
#   scripts/public-launch/reset-releases.sh --execute       # actually delete
#   scripts/public-launch/reset-releases.sh neokapi         # one repo, dry run
set -euo pipefail

EXECUTE=0
ONLY_REPO=""
for arg in "$@"; do
  case "$arg" in
    --execute) EXECUTE=1 ;;
    *) ONLY_REPO="$arg" ;;
  esac
done

# Tags matching this regex are CANDIDATES for deletion. Anything else (named
# asset releases like docs-assets, snapshot, okapi-testdata-*, okapi-surefire-*)
# is never touched.
VERSION_RE='^(sat-)?v[0-9]+\.[0-9]+\.[0-9]+(-rc[0-9]+)?$'

# Per-repo KEEP list: version-shaped tags to PRESERVE even though they match the
# regex (i.e. the freshly-cut 1.0.0 baseline). Asset releases are preserved
# automatically by the regex and listed here only for documentation.
keep_for() {
  case "$1" in
    neokapi/neokapi)        echo "v1.0.0 sat-v1.0.0" ;;        # + asset: docs-assets bowrain-docs-assets
    neokapi/okapi-bridge)   echo "v1.0.0" ;;                   # + asset: snapshot okapi-testdata-1.48.0 okapi-surefire-1.48.0
    neokapi/setup-kapi)     echo "v1.0.0 v1" ;;
    neokapi/kapi-action)    echo "v1.0.0 v1" ;;
    *)                      echo "" ;;
  esac
}

reset_repo() {
  local repo="$1"
  local keep; keep=" $(keep_for "$repo") "
  echo "==== ${repo} ===="

  # All tags via the git refs API (covers tags with and without a release).
  local tags; tags=$(gh api "repos/${repo}/git/refs/tags" --paginate \
    --jq '.[].ref | sub("refs/tags/"; "")' 2>/dev/null || true)

  for tag in $tags; do
    if ! [[ "$tag" =~ $VERSION_RE ]]; then
      continue   # named asset release / non-version tag — never touch
    fi
    if [[ "$keep" == *" $tag "* ]]; then
      echo "  keep   ${tag}"
      continue
    fi
    if [ "$EXECUTE" = "1" ]; then
      echo "  DELETE ${tag}"
      gh release delete "$tag" --repo "$repo" --yes --cleanup-tag 2>/dev/null \
        || gh api -X DELETE "repos/${repo}/git/refs/tags/${tag}" 2>/dev/null \
        || echo "    (already gone)"
    else
      echo "  would delete ${tag}"
    fi
  done
}

REPOS="neokapi/neokapi neokapi/okapi-bridge neokapi/setup-kapi neokapi/kapi-action"
[ -n "$ONLY_REPO" ] && REPOS="neokapi/${ONLY_REPO#neokapi/}"

[ "$EXECUTE" = "1" ] || echo "(DRY RUN — pass --execute to delete)"
for r in $REPOS; do reset_repo "$r"; done
echo "done."
