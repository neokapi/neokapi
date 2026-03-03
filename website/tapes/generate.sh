#!/bin/bash
# Generate kapi CLI demo videos using VHS
# Requires: vhs (brew install charmbracelet/tap/vhs)
#
# For Bowrain CLI demos, see bowrain/e2e/tapes/

set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$SCRIPT_DIR"

# Check for VHS
if ! command -v vhs &> /dev/null; then
  echo "VHS not found. Install with: brew install charmbracelet/tap/vhs"
  exit 1
fi

# Run CLI tests first
echo "============================================"
echo "Running kapi CLI tests before recording..."
echo "============================================"
echo ""

if ! bash "$SCRIPT_DIR/test-cli.sh"; then
  echo ""
  echo "CLI tests failed. Fix issues before recording."
  exit 1
fi

echo ""

# Add bin to PATH (kapi was built by test-cli.sh)
export PATH="$SCRIPT_DIR/../../bin:$PATH"

# Use isolated config for recordings
export KAPI_CONFIG_DIR="$(mktemp -d)"

# Create output directory
mkdir -p output

# Detect CI
if [ -n "$CI" ] || [ -n "$GITHUB_ACTIONS" ]; then
  echo "Running in CI mode (headless)"
  if [ -z "$DISPLAY" ]; then
    echo "Warning: DISPLAY not set in CI. VHS may fail."
  fi
else
  echo "Running in local mode"
fi
echo ""

# Generate kapi demo videos
echo "Generating kapi CLI demo videos..."
echo ""

failed=0
for tape in overview.tape word-count.tape pseudo-translate.tape; do
  name="${tape%.tape}"
  echo "  Recording: $name"
  if timeout 180 vhs "$tape" 2>&1; then
    echo "    ✓ Done"
  else
    exitcode=$?
    if [ $exitcode -eq 124 ]; then
      echo "    ✗ Timed out (3 min limit)"
    else
      echo "    ✗ Failed (exit $exitcode)"
    fi
    failed=$((failed + 1))
  fi
done

# Clean up temp dir
rm -rf "$KAPI_CONFIG_DIR"

if [ $failed -gt 0 ]; then
  echo ""
  echo "Warning: $failed recording(s) failed."
fi

# Copy to docs
DOCS_VIDEO_DIR="$SCRIPT_DIR/../../website/static/video/kapi"
mkdir -p "$DOCS_VIDEO_DIR"

echo ""
echo "Copying videos to $DOCS_VIDEO_DIR..."
for video in output/*.webm; do
  if [ -f "$video" ]; then
    name=$(basename "$video")
    cp "$video" "$DOCS_VIDEO_DIR/"
    size=$(du -h "$DOCS_VIDEO_DIR/$name" | cut -f1)
    echo "  $name ($size)"
  fi
done

echo ""
echo "Done!"
