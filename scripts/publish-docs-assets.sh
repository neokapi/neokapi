#!/usr/bin/env bash
#
# Publish web/docs/static/{img,video} to the "docs-assets" GitHub release.
#
# This MERGES into whatever is already in the release tarball, so it never drops
# other assets: it downloads the current docs-assets.tar.gz, overlays the local
# img/ and video/ trees (local wins, nothing is deleted), re-packs, and re-uploads.
# The docs deploy pulls this back down via `make fetch-docs-assets`.
#
# Usage (from repo root):  bash scripts/publish-docs-assets.sh
# Requires: gh (authenticated), tar, rsync.

set -euo pipefail
cd "$(dirname "$0")/.."

STATIC="web/docs/static"
TARBALL="docs-assets.tar.gz"
RELEASE="docs-assets"

command -v gh >/dev/null || { echo "error: gh CLI not found"; exit 1; }

WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT

echo "→ fetching current $RELEASE tarball to merge into…"
if gh release download "$RELEASE" --pattern "$TARBALL" --dir "$WORK" --clobber 2>/dev/null; then
  tar xzf "$WORK/$TARBALL" -C "$WORK"
  rm -f "$WORK/$TARBALL"
  echo "  base extracted (existing assets preserved)"
else
  echo "  no existing tarball — creating a fresh one"
fi

# Overlay local img/ + video/ onto the extracted base (additive: never deletes).
for d in img video; do
  if [ -d "$STATIC/$d" ] && [ -n "$(find "$STATIC/$d" -type f -print -quit)" ]; then
    mkdir -p "$WORK/$d"
    rsync -a "$STATIC/$d/" "$WORK/$d/"
    echo "  overlaid local $d/"
  fi
done

# Pack whichever of img/ video/ ended up present.
DIRS=()
[ -d "$WORK/img" ] && DIRS+=("img")
[ -d "$WORK/video" ] && DIRS+=("video")
if [ ${#DIRS[@]} -eq 0 ]; then
  echo "error: nothing to publish (no img/ or video/ under $STATIC)"; exit 1
fi

echo "→ packing $TARBALL (${DIRS[*]})…"
( cd "$WORK" && tar czf "$TARBALL" "${DIRS[@]}" )
COUNT="$(cd "$WORK" && find "${DIRS[@]}" -type f | wc -l | tr -d ' ')"
echo "  $COUNT files, $(du -h "$WORK/$TARBALL" | cut -f1)"

echo "→ uploading to $RELEASE release…"
gh release upload "$RELEASE" "$WORK/$TARBALL" --clobber
echo "✓ published. The docs deploy picks it up via 'make fetch-docs-assets'."
