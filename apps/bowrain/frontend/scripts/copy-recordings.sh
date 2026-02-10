#!/bin/bash
# Copy Playwright recording videos to docs folder with clean names

set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
FRONTEND_DIR="$(dirname "$SCRIPT_DIR")"
RECORDINGS_DIR="$FRONTEND_DIR/recordings-output"
OUTPUT_DIR="$FRONTEND_DIR/../../../website/static/video/bowrain"

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

# Create output directory
mkdir -p "$OUTPUT_DIR"

# Find all video files and copy them with clean names
echo "Copying recording videos to $OUTPUT_DIR..."

copied=0
for dir in "$RECORDINGS_DIR"/*; do
  if [ -d "$dir" ]; then
    video_file="$dir/video.webm"
    if [ -f "$video_file" ]; then
      dir_name=$(basename "$dir")
      dir_lower=$(echo "$dir_name" | tr '[:upper:]' '[:lower:]')

      # Match against known recording names using suffix matching.
      # Playwright may truncate names and prepend a hex hash, so the
      # directory name contains a suffix of the test name.
      matched=""
      best_len=0
      for name in "${KNOWN_NAMES[@]}"; do
        if echo "$dir_lower" | grep -qF -- "$name"; then
          # Full match
          name_len=${#name}
          if [ "$name_len" -gt "$best_len" ]; then
            matched="$name"
            best_len=$name_len
          fi
        else
          # Try suffix matching: does the dir name end with the tail of a known name?
          # Extract the fragment after stripping the prefix
          fragment=$(echo "$dir_lower" \
            | sed -E 's/^recordings-video-recording[s]?-//' \
            | sed -E 's/^record-//' \
            | sed -E 's/^[0-9a-f]+-?//')
          # Check if the known name ends with this fragment
          if [ -n "$fragment" ] && echo "$name" | grep -qF -- "$fragment"; then
            name_len=${#name}
            if [ "$name_len" -gt "$best_len" ]; then
              matched="$name"
              best_len=$name_len
            fi
          fi
        fi
      done

      if [ -z "$matched" ]; then
        # Fallback: strip prefix and clean up
        matched=$(echo "$dir_name" \
          | sed -E 's/^recordings-Video-Recording[s]?-([0-9a-fA-F]+-)?//' \
          | sed -E 's/^record-//' \
          | sed -E 's/^-//' \
          | tr '[:upper:]' '[:lower:]' \
          | tr ' ' '-')
        echo "  ? ${matched}.webm (unrecognized dir: $dir_name)"
      fi

      dest="$OUTPUT_DIR/${matched}.webm"
      cp "$video_file" "$dest"
      size=$(du -h "$dest" | cut -f1)
      echo "  ✓ ${matched}.webm (${size})"
      copied=$((copied + 1))
    fi
  fi
done

if [ $copied -eq 0 ]; then
  echo "No videos found in $RECORDINGS_DIR"
  echo "Run 'npm run recordings' first to generate videos."
  exit 1
fi

echo ""
echo "Done! $copied videos copied to $OUTPUT_DIR"
