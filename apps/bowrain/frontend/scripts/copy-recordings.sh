#!/bin/bash
# Copy Playwright recording videos to docs folder with clean names.
#
# Usage:
#   THEME=dark  ./copy-recordings.sh   # copy to website/static/video/bowrain/dark/
#   THEME=light ./copy-recordings.sh   # copy to website/static/video/bowrain/light/
#   ./copy-recordings.sh               # copy to website/static/video/bowrain/dark/ (default)

set -euo pipefail

THEME="${THEME:-dark}"
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
FRONTEND_DIR="$(dirname "$SCRIPT_DIR")"
RECORDINGS_DIR="$FRONTEND_DIR/recordings-output"
OUTPUT_DIR="$FRONTEND_DIR/../../../website/static/video/bowrain/$THEME"

# Known recording names (must match test names in recordings.spec.ts)
KNOWN_NAMES=(
  "create-project-flow"
  "translation-editor-workflow"
  "focus-view-editing"
  "tm-explorer"
  "flow-editor"
  "end-to-end-translation-workflow"
  "tm-leverage-workflow"
  "term-explorer"
  "context-panel"
  "workspace-switcher"
  "account-and-authentication"
  "workspace-project-management"
  "settings-configuration"
)

if [ ! -d "$RECORDINGS_DIR" ]; then
  echo "ERROR: No recordings found at $RECORDINGS_DIR"
  echo "Run 'npm run recordings' first to generate videos."
  exit 1
fi

# Create output directory (clean existing files to avoid stale leftovers)
rm -rf "$OUTPUT_DIR"
mkdir -p "$OUTPUT_DIR"

# Find all video files and copy them with clean names
echo "Copying recording videos to $OUTPUT_DIR..."

copied=0
for dir in "$RECORDINGS_DIR"/*/; do
  [ -d "$dir" ] || continue
  video_file="$dir/video.webm"
  [ -f "$video_file" ] || continue

  dir_name=$(basename "$dir")
  dir_lower=$(echo "$dir_name" | tr '[:upper:]' '[:lower:]')

  # Skip directories for the other theme
  if [[ "$THEME" == "dark" && "$dir_lower" == *"-light-"* ]]; then continue; fi
  if [[ "$THEME" == "light" && "$dir_lower" == *"-dark-"* ]]; then continue; fi

  # Strip common prefixes from Playwright test output directory names
  name=$(echo "$dir_lower" | sed -E 's/^recordings-video-recording[s]?-//')
  name=$(echo "$name" | sed -E 's/^record-//')
  name=$(echo "$name" | sed -E 's/^[0-9a-f]+-?//')

  # Strip theme suffix from test name (e.g. "create-project-flow-dark-" -> "create-project-flow")
  name=$(echo "$name" | sed -E "s/-${THEME}(-|$)/\1/")

  # Clean trailing hyphens/whitespace
  name=$(echo "$name" | sed -E 's/[-]+$//; s/^[-]+//')

  # Match against known recording names using longest match.
  # Strategy 1: dir name contains the full known name (exact substring).
  # Strategy 2: known name ends with the (possibly truncated) dir name fragment (suffix match).
  matched=""
  best_len=0
  for known in "${KNOWN_NAMES[@]}"; do
    if echo "$name" | grep -qF -- "$known"; then
      # Full match: dir name contains the entire known name
      name_len=${#known}
      if [ "$name_len" -gt "$best_len" ]; then
        matched="$known"
        best_len=$name_len
      fi
    elif [ -n "$name" ] && echo "$known" | grep -qF -- "$name"; then
      # Suffix match: known name contains the (truncated) fragment
      name_len=${#known}
      if [ "$name_len" -gt "$best_len" ]; then
        matched="$known"
        best_len=$name_len
      fi
    fi
  done

  if [ -z "$matched" ]; then
    # Fallback: clean up the name
    matched=$(echo "$name" | sed -E 's/[^a-z0-9]+/-/g; s/^-//; s/-$//')
    echo "  ? Unknown recording: $dir_name -> $matched.webm"
  fi

  cp "$video_file" "$OUTPUT_DIR/${matched}.webm"
  size=$(du -h "$video_file" | cut -f1)
  echo "  ✓ ${matched}.webm (${size}) -> $THEME/"
  ((copied++))
done

if [ $copied -eq 0 ]; then
  echo "No videos found in $RECORDINGS_DIR"
  echo "Run 'npm run recordings' first to generate videos."
  exit 1
fi

echo ""
echo "Done! $copied videos copied to $OUTPUT_DIR"
