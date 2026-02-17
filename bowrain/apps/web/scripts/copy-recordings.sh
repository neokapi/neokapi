#!/usr/bin/env bash
# Copy Playwright recording videos to the documentation website.
# Matches test names to video filenames.
#
# Usage:
#   THEME=glass  ./copy-recordings.sh   # copy to website/static/video/web-app/glass/
#   THEME=light  ./copy-recordings.sh   # copy to website/static/video/web-app/light/
#   THEME=aurora ./copy-recordings.sh   # copy to website/static/video/web-app/aurora/
#   ./copy-recordings.sh                # copy to website/static/video/web-app/glass/ (default)
set -euo pipefail

THEME="${THEME:-glass}"
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
WEB_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
RECORDINGS_DIR="$WEB_DIR/recordings-output"
OUTPUT_DIR="$WEB_DIR/../../website/static/video/web-app/$THEME"

# Known recording names (must match test names in recordings.spec.ts)
KNOWN_RECORDINGS=(
  "login-and-workspace"
  "translation-editor"
  "focus-view"
  "pseudo-translation"
  "tm-explorer"
  "term-explorer"
  "context-panel"
  "settings"
  "invite-workflow"
)

if [ ! -d "$RECORDINGS_DIR" ]; then
  echo "ERROR: No recordings found at $RECORDINGS_DIR"
  echo "Run: npx playwright test --config playwright.recordings.config.ts"
  exit 1
fi

mkdir -p "$OUTPUT_DIR"

copied=0
for dir in "$RECORDINGS_DIR"/*/; do
  [ -d "$dir" ] || continue
  video="$dir/video.webm"
  [ -f "$video" ] || continue

  dirname=$(basename "$dir")
  name=$(echo "$dirname" | tr '[:upper:]' '[:lower:]')

  # Skip directories for other themes
  for skip_theme in glass light aurora; do
    if [[ "$THEME" != "$skip_theme" && "$name" == *"-${skip_theme}-"* ]]; then continue 2; fi
  done

  # Strip common prefixes from Playwright test output directory names
  name=$(echo "$name" | sed -E 's/^recordings-video-recording[s]?-//')
  name=$(echo "$name" | sed -E 's/^record-//')
  name=$(echo "$name" | sed -E 's/^web-app-recordings-//')
  name=$(echo "$name" | sed -E 's/^[0-9a-f]+-//')

  # Strip theme suffix from test name (e.g. "login-and-workspace-dark-" -> "login-and-workspace")
  name=$(echo "$name" | sed -E "s/-${THEME}(-|$)/\1/")

  matched=""
  for known in "${KNOWN_RECORDINGS[@]}"; do
    if [[ "$name" == *"$known"* ]]; then
      matched="$known"
      break
    fi
  done

  if [ -z "$matched" ]; then
    # Clean up: replace spaces/special chars with hyphens
    matched=$(echo "$name" | sed -E 's/[^a-z0-9]+/-/g; s/^-//; s/-$//')
    echo "  ? Unknown recording: $dirname -> $matched.webm"
  fi

  cp "$video" "$OUTPUT_DIR/$matched.webm"
  size=$(du -h "$video" | cut -f1)
  echo "  ✓ $matched.webm ($size) -> $THEME/"
  copied=$((copied + 1))
done

echo ""
echo "Copied $copied recordings to $OUTPUT_DIR"
