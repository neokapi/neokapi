#!/usr/bin/env bash
#
# stage-scenes.sh — copy recorded scene .webm files into static/video/bowrain/
# so the published Docusaurus site embeds them at /video/bowrain/{filename}.
#
# The walkthrough engine records scenes at:
#   bowrain/web/docs/scenes/{walkthrough-id}/0N-{scene-id}.webm
#
# This script flat-copies them as:
#   bowrain/web/docs/static/video/bowrain/{walkthrough-id}/0N-{scene-id}.webm
#
# Run as part of the docs-bowrain CI workflow before `vpx docusaurus build`. Idempotent.
#
# The .webm files are gitignored under static/. They live in scenes/ as
# tracked artifacts so reviewers can see what shipped, but the staged
# copies under static/video/ are build outputs.

set -euo pipefail

REPO_ROOT="$(git rev-parse --show-toplevel)"
SCENES="$REPO_ROOT/bowrain/web/docs/scenes"
STAGE="$REPO_ROOT/bowrain/web/docs/static/video/bowrain"

shopt -s nullglob
for webm in "$SCENES"/*/0*.webm; do
  walkthrough_dir="$(dirname "$webm")"
  walkthrough_id="$(basename "$walkthrough_dir")"
  filename="$(basename "$webm")"
  dest_dir="$STAGE/$walkthrough_id"
  mkdir -p "$dest_dir"
  cp "$webm" "$dest_dir/$filename"
  echo "staged $walkthrough_id/$filename"
done
