#!/usr/bin/env bash
#
# Deploy already-built docs/landing sites to the neokapi.github.io org pages
# repo from a developer's machine — the local equivalent of pages-deploy.yml.
#
# It does NOT build anything: the Makefile targets (publish-website /
# publish-landing) build the dist trees with the PRODUCTION base URL pinned,
# then call this script to clone the pages repo, slot the builds into their
# path-based destinations, and push. Build vs deploy stay separate (mirroring
# docs-build vs publish-docs-assets) so you can inspect a dist before shipping.
#
# Usage:
#   scripts/publish-pages.sh <site>...
#   sites: neokapi-docs | bowrain-docs | bowrain-landing | all
#
# Auth: pushes over SSH to git@github.com (override with PAGES_REPO). Any org
# member with write access + an SSH key already has what CI's PAGES_DEPLOY_KEY
# provides — no PAT or deploy key needed. Set GIT_SSH_COMMAND yourself to use a
# specific key.
#
# Safety:
#   DRY_RUN=1            build the deploy tree and stop before committing/pushing
#                        (the clone is left in place and its path is printed).
#   PAGES_PUBLISH_YES=1  skip the interactive "deploy to production?" prompt.
#
# This pushes to the LIVE production branch the CI deploy also writes to, so it
# rebase-retries on a racing deploy. Avoid running it while an Actions deploy is
# in flight, and never point the local targets at PR-preview slots.

set -euo pipefail

PAGES_REPO="${PAGES_REPO:-git@github.com:neokapi/neokapi.github.io.git}"

# repo root = parent of this script's dir
ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

if [ "$#" -eq 0 ]; then
  echo "usage: $0 <site>...  (neokapi-docs | bowrain-docs | bowrain-landing | all)" >&2
  exit 2
fi

# Expand "all"
sites=()
for arg in "$@"; do
  if [ "$arg" = "all" ]; then
    sites=(neokapi-docs bowrain-docs bowrain-landing)
  else
    sites+=("$arg")
  fi
done

# site → "source_dist|dest_slot|kind"  (kind: docs = replace slot wholesale;
# landing = replace slot contents but PRESERVE a co-located docs/ subdir).
site_spec() {
  case "$1" in
    neokapi-docs)    echo "web/docs/build|web/neokapi|docs" ;;
    bowrain-docs)    echo "bowrain/web/docs/build|web/bowrain/docs|docs" ;;
    bowrain-landing) echo "bowrain/web/landing/dist|web/bowrain|landing" ;;
    *) return 1 ;;
  esac
}

# Validate sites and that each build output exists & is non-empty before we
# touch the pages repo — fail fast rather than push a half-built tree.
for s in "${sites[@]}"; do
  spec="$(site_spec "$s")" || { echo "✗ unknown site: $s" >&2; exit 2; }
  src="${spec%%|*}"
  if [ ! -d "$src" ] || [ -z "$(ls -A "$src" 2>/dev/null)" ]; then
    echo "✗ $s: build output '$src' is missing or empty — build it first (make publish-website / publish-landing)." >&2
    exit 1
  fi
done

# Confirm SSH push access before cloning, with a clear message on failure.
if ! GIT_SSH_COMMAND="${GIT_SSH_COMMAND:-ssh -o BatchMode=yes}" \
     git ls-remote "$PAGES_REPO" >/dev/null 2>&1; then
  echo "✗ cannot reach $PAGES_REPO over git/SSH." >&2
  echo "  You need write access to the org pages repo and an SSH key loaded" >&2
  echo "  (gh auth setup-git / ssh-add). Override the URL with PAGES_REPO=…" >&2
  exit 1
fi

# Production confirmation guard (skipped for DRY_RUN or PAGES_PUBLISH_YES=1).
if [ "${DRY_RUN:-}" != "1" ] && [ "${PAGES_PUBLISH_YES:-}" != "1" ]; then
  echo "About to deploy to PRODUCTION (${PAGES_REPO}):"
  for s in "${sites[@]}"; do
    spec="$(site_spec "$s")"; echo "  - $s → /$(echo "$spec" | cut -d'|' -f2)/"
  done
  read -r -p "Proceed? [y/N] " reply
  case "$reply" in y|Y|yes|Yes) ;; *) echo "aborted."; exit 1 ;; esac
fi

workdir="$(mktemp -d)"
pages="$workdir/pages"
cleanup() { [ "${DRY_RUN:-}" = "1" ] || rm -rf "$workdir"; }
trap cleanup EXIT

echo "→ Cloning $PAGES_REPO (depth 50)…"
# Depth 50 (not 1): the push step may rebase onto a concurrent deploy's commit,
# which needs a merge base a depth-1 clone lacks.
git clone --depth 50 "$PAGES_REPO" "$pages" >/dev/null 2>&1
git -C "$pages" config user.name "$(git config user.name || echo 'pages-deploy')"
git -C "$pages" config user.email "$(git config user.email || echo 'pages-deploy@local')"

for s in "${sites[@]}"; do
  spec="$(site_spec "$s")"
  src="${spec%%|*}"; rest="${spec#*|}"; dest="${rest%%|*}"; kind="${rest#*|}"
  echo "→ Slotting $s: $src → /$dest/"
  mkdir -p "$pages/$dest"
  if [ "$kind" = "landing" ]; then
    # Replace only the app's own files; preserve a docs/ subdir from a docs deploy.
    ( cd "$pages/$dest" && find . -mindepth 1 -maxdepth 1 ! -name docs -exec rm -rf {} + )
  else
    rm -rf "${pages:?}/$dest"
    mkdir -p "$pages/$dest"
  fi
  cp -R "$src/." "$pages/$dest/"
done

cd "$pages"
git add -A
if git diff --cached --quiet; then
  echo "✓ No changes to deploy."
  exit 0
fi

if [ "${DRY_RUN:-}" = "1" ]; then
  echo "── DRY RUN — staged changes (not committed/pushed):"
  git diff --cached --stat
  echo "Clone left at: $pages"
  exit 0
fi

git commit -q -m "deploy: $(IFS=', '; echo "${sites[*]}") (local publish)"

# Cross-source deploys write disjoint subtrees, so rebasing onto a deploy that
# landed first is conflict-free; retry before failing rather than abort.
for attempt in 1 2 3 4 5; do
  if git push origin HEAD:main 2>/dev/null; then
    echo "✓ Pushed on attempt $attempt."
    break
  fi
  if [ "$attempt" = "5" ]; then
    echo "✗ pages push failed after $attempt attempts" >&2
    exit 1
  fi
  echo "  push rejected (attempt $attempt); rebasing onto latest main…"
  git fetch -q origin main
  if ! git rebase -q origin/main; then
    git rebase --abort || true
    echo "✗ pages rebase hit a conflict (overlapping subtree?)" >&2
    exit 1
  fi
  sleep $((attempt * 3))
done

echo ""
echo "✓ Deployed. Live URLs:"
for s in "${sites[@]}"; do
  spec="$(site_spec "$s")"; echo "  https://neokapi.github.io/$(echo "$spec" | cut -d'|' -f2)/"
done
