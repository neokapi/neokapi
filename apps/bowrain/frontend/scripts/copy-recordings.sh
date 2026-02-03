#!/bin/bash
# Copy Playwright recording videos to docs folder with clean names

set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
FRONTEND_DIR="$(dirname "$SCRIPT_DIR")"
RECORDINGS_DIR="$FRONTEND_DIR/recordings-output"
OUTPUT_DIR="$FRONTEND_DIR/../../../website/static/video/bowrain"

# Create output directory
mkdir -p "$OUTPUT_DIR"

# Find all video files and copy them with clean names
echo "Copying recording videos to $OUTPUT_DIR..."

copied=0
for dir in "$RECORDINGS_DIR"/*; do
  if [ -d "$dir" ]; then
    video_file="$dir/video.webm"
    if [ -f "$video_file" ]; then
      # Extract test name from directory name
      dir_name=$(basename "$dir")
      
      # Remove various prefixes that Playwright uses
      # e.g. "recordings-Video-Recordings-record-create-project-flow"
      # e.g. "recordings-Video-Recording-12896-translation-editor-workflow"
      clean_name=$(echo "$dir_name" \
        | sed -E 's/^recordings-Video-Recording[s]?-[0-9]*-?//' \
        | sed -E 's/^record-//' \
        | tr '[:upper:]' '[:lower:]' \
        | tr ' ' '-')
      
      dest="$OUTPUT_DIR/${clean_name}.webm"
      cp "$video_file" "$dest"
      size=$(du -h "$dest" | cut -f1)
      echo "  ✓ ${clean_name}.webm (${size})"
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
