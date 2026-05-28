#!/usr/bin/env bash
#
# Publish bowrain/web/docs/static/{img,video} to the "bowrain-docs-assets"
# GitHub release.
#
# Bowrain's framed walkthrough videos (the harness-rendered bowrain-web/ and
# bowrain-desktop/ trees) are recorded on a desktop against a real running
# stack — CI can't reproduce that — and static/video is gitignored. So, exactly
# like kapi's docs-assets pipeline (scripts/publish-docs-assets.sh), the rendered
# assets are published to a GitHub release and the docs build stages them back
# down (see the "Stage bowrain framed videos" step in docs-bowrain.yml).
#
# This MERGES into whatever is already in the release tarball, so it never drops
# other assets: it downloads the current bowrain-docs-assets.tar.gz, overlays the
# local img/ and video/ trees (local wins, nothing is deleted), re-packs, and
# re-uploads.
#
# It is intentionally separate from kapi's `docs-assets` release so the two
# sites' asset bundles stay independent.
#
# Usage (from repo root):  bash scripts/publish-bowrain-docs-assets.sh
# Requires: gh (authenticated), tar, rsync.

set -euo pipefail
cd "$(dirname "$0")/.."

STATIC="bowrain/web/docs/static"
TARBALL="bowrain-docs-assets.tar.gz"
RELEASE="bowrain-docs-assets"

command -v gh >/dev/null || { echo "error: gh CLI not found"; exit 1; }

# Ensure the release exists (first publish creates it). `gh release view` exits
# non-zero when the release is absent.
if ! gh release view "$RELEASE" >/dev/null 2>&1; then
  echo "→ creating $RELEASE release (first publish)…"
  gh release create "$RELEASE" \
    --title "bowrain docs assets" \
    --notes "Rendered bowrain docs videos/images, staged by docs-bowrain.yml. Managed by scripts/publish-bowrain-docs-assets.sh — do not edit by hand." \
    --latest=false
fi

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
echo "✓ published. The docs deploy stages it in docs-bowrain.yml (bowrain-docs-assets)."
