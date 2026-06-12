#!/usr/bin/env bash
#
# publish-corpus.sh — Publish staged per-format Tier B corpora to the
# format-corpus-vN GitHub release (docs/internals/format-maturity.md §2.5).
#
# Staging convention: corpus-staging/<id>/ (repo root, gitignored) holds the
# NEW or updated corpus files for format <id>, laid out exactly as they should
# appear under corpus/<tag>/<id>/ after `make fetch-corpus`. Each staged <id>
# must correspond to a real core/formats/<id>/ package. Only staged formats
# are touched; a one-format respin never re-ships every binary because the
# release carries one corpus-<id>.tar.gz asset per format.
#
# This MERGES per format and never drops: for each staged format it downloads
# that format's current corpus-<id>.tar.gz from the release, extracts it,
# overlays the staged tree (local wins, nothing is deleted), re-packs, and
# re-uploads with --clobber. The release itself is auto-created on first
# publish (--latest=false so it never shadows real product releases) — the
# same idiom as scripts/publish-docs-assets.sh + the first-publish guard in
# scripts/publish-bowrain-docs-assets.sh.
#
# Environment:
#   CORPUS_TAG — Override the target release tag (default: lexically-latest
#                format-corpus-v* tag, or format-corpus-v1 on first publish).
#
# Usage (from repo root):  bash scripts/publish-corpus.sh
# Requires: gh (authenticated), tar, rsync.

set -euo pipefail
cd "$(dirname "$0")/.."

STAGING="corpus-staging"
TAG_PREFIX="format-corpus-v"
DEFAULT_TAG="${TAG_PREFIX}1"

command -v gh >/dev/null || { echo "error: gh CLI not found"; exit 1; }

if [ ! -d "$STAGING" ]; then
  echo "error: nothing staged — put new corpus files under $STAGING/<id>/ first"
  echo "  (laid out as they should appear under corpus/<tag>/<id>/ after fetch)"
  exit 1
fi

# Collect staged format ids: every corpus-staging/<id>/ dir with at least one
# file, validated against the real format packages.
FORMATS=()
for dir in "$STAGING"/*/; do
  [ -d "$dir" ] || continue
  id="$(basename "$dir")"
  if [ -z "$(find "$dir" -type f -print -quit)" ]; then
    echo "  skipping $id (nothing staged)"
    continue
  fi
  if [ ! -d "core/formats/$id" ]; then
    echo "error: staged '$id' has no core/formats/$id/ package"
    exit 1
  fi
  FORMATS+=("$id")
done
if [ ${#FORMATS[@]} -eq 0 ]; then
  echo "error: nothing staged under $STAGING/<id>/"
  exit 1
fi

# Resolve the target tag: explicit override, else the lexically-latest
# format-corpus-v* release, else first publish under DEFAULT_TAG.
TAG="${CORPUS_TAG:-}"
if [ -z "$TAG" ]; then
  TAG="$(gh release list --limit 1000 --json tagName \
    --jq "[.[].tagName | select(startswith(\"$TAG_PREFIX\"))] | sort | last // empty")"
fi
if [ -z "$TAG" ]; then
  TAG="$DEFAULT_TAG"
fi

# Ensure the release exists (first publish creates it). `gh release view`
# exits non-zero when the release is absent.
if ! gh release view "$TAG" >/dev/null 2>&1; then
  echo "→ creating $TAG release (first publish)…"
  gh release create "$TAG" \
    --title "format corpus ($TAG)" \
    --notes "Tier B format corpora — one corpus-<id>.tar.gz asset per format, fetched by 'make fetch-corpus' into corpus/$TAG/<id>/. Managed by scripts/publish-corpus.sh — do not edit by hand." \
    --latest=false
fi

WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT

for id in "${FORMATS[@]}"; do
  ASSET="corpus-$id.tar.gz"
  BASE="$WORK/$id"
  mkdir -p "$BASE"

  echo "→ $id: fetching current $ASSET to merge into…"
  if gh release download "$TAG" --pattern "$ASSET" --dir "$WORK" --clobber 2>/dev/null; then
    tar xzf "$WORK/$ASSET" -C "$BASE"
    rm -f "$WORK/$ASSET"
    echo "  base extracted (existing files preserved)"
  else
    echo "  no existing asset — creating a fresh one"
  fi

  # Overlay the staged tree onto the extracted base (additive: never deletes).
  rsync -a "$STAGING/$id/" "$BASE/"

  echo "→ $id: packing $ASSET…"
  ( cd "$BASE" && tar czf "$WORK/$ASSET" . )
  COUNT="$(find "$BASE" -type f | wc -l | tr -d ' ')"
  echo "  $COUNT files, $(du -h "$WORK/$ASSET" | cut -f1)"

  echo "→ $id: uploading to $TAG…"
  gh release upload "$TAG" "$WORK/$ASSET" --clobber
  rm -f "$WORK/$ASSET"
done

echo "✓ published ${#FORMATS[@]} format corpus asset(s) to $TAG."
echo "  Fetch with 'make fetch-corpus' (corpus/<tag>/<id>/); staged files in $STAGING/ are yours to clean up."
