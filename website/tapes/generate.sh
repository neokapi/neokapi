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

# Run CLI tests first
echo "============================================"
echo "Running CLI tests before recording..."
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

# Tapes that need a running server
SERVER_TAPES="workspaces"

# Check if server-backed recordings are possible
SERVER_AVAILABLE=false
if command -v docker &> /dev/null && docker compose version &> /dev/null 2>&1; then
  echo ""
  echo "Starting server for live recordings..."
  if bash "$SCRIPT_DIR/start-server.sh"; then
    source "$SCRIPT_DIR/.server-token" 2>/dev/null || true
    SERVER_AVAILABLE=true
  else
    echo "  Could not start server. Server-backed tapes will be skipped."
  fi
else
  echo ""
  echo "Docker not available. Server-backed tapes will be skipped."
fi

# Generate all tapes
echo ""
echo "Generating CLI demo videos..."
echo "(Note: Requires local terminal with TTY access)"
echo ""

failed=0
for tape in *.tape; do
  if [ -f "$tape" ]; then
    name="${tape%.tape}"

    # Check if this tape needs the server
    is_server_tape=false
    for st in $SERVER_TAPES; do
      if [ "$name" = "$st" ]; then
        is_server_tape=true
        break
      fi
    done

    if [ "$is_server_tape" = true ] && [ "$SERVER_AVAILABLE" = false ]; then
      echo "  Skipping: $name (requires server)"
      continue
    fi

    echo "  Recording: $name"
    if vhs "$tape" 2>&1; then
      echo "    Done"
    else
      echo "    Failed (may need local TTY)"
      failed=$((failed + 1))
    fi
  fi
done

# Stop server if we started it
if [ "$SERVER_AVAILABLE" = true ]; then
  echo ""
  echo "Stopping server..."
  bash "$SCRIPT_DIR/stop-server.sh" || true
fi

# Clean up temp config
rm -rf "$KAPI_CONFIG_DIR"

if [ $failed -gt 0 ]; then
  echo ""
  echo "Some recordings failed. VHS requires a local terminal with TTY access."
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
    echo "  $name ($size)"
  fi
done

# Also copy GIFs for README embeds
for gif in output/*.gif; do
  if [ -f "$gif" ]; then
    name=$(basename "$gif")
    cp "$gif" "$DOCS_VIDEO_DIR/"
    size=$(du -h "$DOCS_VIDEO_DIR/$name" | cut -f1)
    echo "  $name ($size)"
  fi
done

echo ""
echo "Done!"
