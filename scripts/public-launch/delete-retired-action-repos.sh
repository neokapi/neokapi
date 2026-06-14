#!/usr/bin/env bash
#
# delete-retired-action-repos.sh — hard-delete the two CI action repos that are
# retired now that bowrain is a kapi plugin (its CI folds into setup-kapi +
# kapi-action). See the public-launch tracking issue.
#
# IRREVERSIBLE. DRY RUN by default; pass --execute to delete.
# Requires: gh authenticated with the `delete_repo` scope
#   (gh auth refresh -s delete_repo).
set -euo pipefail

EXECUTE=0
[ "${1:-}" = "--execute" ] && EXECUTE=1

REPOS="neokapi/setup-bowrain neokapi/bowrain-action"

for repo in $REPOS; do
  if ! gh repo view "$repo" >/dev/null 2>&1; then
    echo "skip ${repo} (already gone)"; continue
  fi
  if [ "$EXECUTE" = "1" ]; then
    echo "DELETE ${repo}"
    gh repo delete "$repo" --yes
  else
    echo "would delete ${repo}"
  fi
done

[ "$EXECUTE" = "1" ] || echo "(DRY RUN — pass --execute to delete)"
