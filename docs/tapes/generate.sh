#!/bin/bash
# Generate CLI demo videos using VHS
# Requires: vhs (brew install charmbracelet/tap/vhs)
# Note: Must be run in a local terminal with TTY access (not SSH/CI)

set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$SCRIPT_DIR"

# Check for VHS
if ! command -v vhs &> /dev/null; then
  echo "VHS not found. Install with: brew install charmbracelet/tap/vhs"
  exit 1
fi

# Build kapi if needed
echo "Building kapi..."
(cd ../.. && go build -o bin/kapi ./cmd/kapi)

# Add bin to PATH
export PATH="$SCRIPT_DIR/../../bin:$PATH"

# Create output directory
mkdir -p output

# Generate all tapes
echo ""
echo "Generating CLI demo videos..."
echo "(Note: Requires local terminal with TTY access)"
echo ""

failed=0
for tape in *.tape; do
  if [ -f "$tape" ]; then
    name="${tape%.tape}"
    echo "  Recording: $name"
    if vhs "$tape" 2>&1; then
      echo "    ✓ Done"
    else
      echo "    ⚠ Failed (may need local TTY)"
      failed=$((failed + 1))
    fi
  fi
done

if [ $failed -gt 0 ]; then
  echo ""
  echo "⚠ Some recordings failed. VHS requires a local terminal with TTY access."
  echo "  Run this script directly in a terminal (not via SSH or CI)."
fi

# Copy to docs
DOCS_VIDEO_DIR="$SCRIPT_DIR/../../website/static/video/cli"
mkdir -p "$DOCS_VIDEO_DIR"

echo ""
echo "Copying videos to $DOCS_VIDEO_DIR..."
for video in output/*.webm; do
  if [ -f "$video" ]; then
    name=$(basename "$video")
    cp "$video" "$DOCS_VIDEO_DIR/"
    size=$(du -h "$DOCS_VIDEO_DIR/$name" | cut -f1)
    echo "  ✓ $name ($size)"
  fi
done

# Also copy GIFs for README embeds
for gif in output/*.gif; do
  if [ -f "$gif" ]; then
    name=$(basename "$gif")
    cp "$gif" "$DOCS_VIDEO_DIR/"
    size=$(du -h "$DOCS_VIDEO_DIR/$name" | cut -f1)
    echo "  ✓ $name ($size)"
  fi
done

echo ""
echo "Done!"
