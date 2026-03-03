#!/bin/bash
# Generate Bowrain CLI demo videos using VHS
# Requires: vhs (brew install charmbracelet/tap/vhs)
#
# Some tapes require a running bowrain-server (via Docker or manually started).
# Server-backed tapes are skipped if no server is available.
#
# For kapi-only demos, see website/tapes/

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
echo "Running Bowrain CLI tests before recording..."
echo "============================================"
echo ""

if ! bash "$SCRIPT_DIR/test-cli.sh"; then
  echo ""
  echo "CLI tests failed. Fix issues before recording."
  exit 1
fi

echo ""

# Add bin to PATH (kapi + bowrain were built by test-cli.sh)
export PATH="$SCRIPT_DIR/../../../bin:$PATH"

# Use isolated config for recordings
export KAPI_CONFIG_DIR="$(mktemp -d)"
export BOWRAIN_CONFIG_DIR="$(mktemp -d)"

# Create output directory
mkdir -p output

# Tapes that need a running server
SERVER_TAPES="workspaces walkthrough-init walkthrough-push walkthrough-pull"

# Check if server-backed recordings are possible.
SERVER_AVAILABLE=false
STARTED_SERVER=false
BOWRAIN_SERVER_URL="${BOWRAIN_SERVER_URL:-http://localhost:8080}"

if curl -sf "${BOWRAIN_SERVER_URL}/api/v1/health" > /dev/null 2>&1; then
  echo ""
  echo "Server already running at $BOWRAIN_SERVER_URL"
  SERVER_AVAILABLE=true
  export BOWRAIN_SERVER_URL

  # Obtain auth token for server-backed tapes if not already provided.
  if [ -z "${BOWRAIN_TOKEN:-}" ]; then
    echo "  Acquiring auth token..."
    START_RESP=$(curl -sf -X POST -d "client_id=vhs-recorder" \
      "${BOWRAIN_SERVER_URL}/api/v1/auth/device/start") || true
    if [ -n "$START_RESP" ]; then
      DEVICE_CODE=$(echo "$START_RESP" | python3 -c "import sys,json; print(json.load(sys.stdin)['device_code'])")
      USER_CODE=$(echo "$START_RESP" | python3 -c "import sys,json; print(json.load(sys.stdin)['user_code'])")
      curl -sf -X POST -d "user_code=$USER_CODE&email=admin@example.com&name=Admin User" \
        "${BOWRAIN_SERVER_URL}/api/v1/auth/device/verify" > /dev/null
      TOKEN_RESP=$(curl -sf -X POST \
        -d "device_code=$DEVICE_CODE&grant_type=urn:ietf:params:oauth:grant-type:device_code" \
        "${BOWRAIN_SERVER_URL}/api/v1/auth/device/poll")
      export BOWRAIN_TOKEN
      BOWRAIN_TOKEN=$(echo "$TOKEN_RESP" | python3 -c "import sys,json; print(json.load(sys.stdin)['access_token'])")
      # Create default workspace for demos.
      curl -sf -X POST \
        -H "Authorization: Bearer $BOWRAIN_TOKEN" \
        -H "Content-Type: application/json" \
        -d '{"name":"Personal","slug":"personal"}' \
        "${BOWRAIN_SERVER_URL}/api/v1/workspaces" > /dev/null 2>&1 || true
      echo "  Token obtained."
    else
      echo "  Warning: could not acquire auth token."
    fi
  fi
elif command -v docker &> /dev/null && docker compose version &> /dev/null 2>&1; then
  echo ""
  echo "Starting server for live recordings..."
  E2E_DIR="$SCRIPT_DIR/../../../e2e"
  if bash "$E2E_DIR/setup.sh"; then
    # Perform device auth flow
    START_RESP=$(curl -sf -X POST -d "client_id=vhs-recorder" \
      http://localhost:8080/api/v1/auth/device/start)
    DEVICE_CODE=$(echo "$START_RESP" | python3 -c "import sys,json; print(json.load(sys.stdin)['device_code'])")
    USER_CODE=$(echo "$START_RESP" | python3 -c "import sys,json; print(json.load(sys.stdin)['user_code'])")
    curl -sf -X POST -d "user_code=$USER_CODE&email=admin@example.com&name=Admin User" \
      http://localhost:8080/api/v1/auth/device/verify > /dev/null
    TOKEN_RESP=$(curl -sf -X POST \
      -d "device_code=$DEVICE_CODE&grant_type=urn:ietf:params:oauth:grant-type:device_code" \
      http://localhost:8080/api/v1/auth/device/poll)
    export BOWRAIN_TOKEN
    BOWRAIN_TOKEN=$(echo "$TOKEN_RESP" | python3 -c "import sys,json; print(json.load(sys.stdin)['access_token'])")
    # Create default workspace
    curl -sf -X POST \
      -H "Authorization: Bearer $BOWRAIN_TOKEN" \
      -H "Content-Type: application/json" \
      -d '{"name":"Personal","slug":"personal"}' \
      http://localhost:8080/api/v1/workspaces > /dev/null 2>&1 || true
    SERVER_AVAILABLE=true
    STARTED_SERVER=true
  else
    echo "  Could not start server. Server-backed tapes will be skipped."
  fi
else
  echo ""
  echo "Docker not available. Server-backed tapes will be skipped."
fi

# Set BOWRAIN_SERVER_URL for walkthrough tapes when server is available.
if [ "$SERVER_AVAILABLE" = true ]; then
  export BOWRAIN_SERVER_URL="${BOWRAIN_SERVER_URL:-http://localhost:8080}"
fi

# Set up clean walkthrough directory for walkthrough tapes.
WALKTHROUGH_DIR=""
if [ "$SERVER_AVAILABLE" = true ]; then
  echo ""
  echo "Setting up walkthrough test directory..."
  WALKTHROUGH_DIR="$(bash "$SCRIPT_DIR/scripts/setup-walkthrough.sh")"
  export WALKTHROUGH_DIR
  echo "  Walkthrough dir: $WALKTHROUGH_DIR"
fi

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

# Generate all tapes
echo "Generating Bowrain CLI demo videos..."
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
    if timeout 180 vhs "$tape" 2>&1; then
      echo "    Done"
    else
      exitcode=$?
      if [ $exitcode -eq 124 ]; then
        echo "    Timed out (3 min limit)"
      else
        echo "    Failed (exit $exitcode)"
      fi
      failed=$((failed + 1))
    fi
  fi
done

# Stop server only if we started it ourselves.
if [ "$STARTED_SERVER" = true ]; then
  echo ""
  echo "Stopping server..."
  E2E_DIR="$SCRIPT_DIR/../../../e2e"
  bash "$E2E_DIR/teardown.sh" || true
fi

# Clean up temp dirs
rm -rf "$KAPI_CONFIG_DIR" "$BOWRAIN_CONFIG_DIR"
if [ -n "$WALKTHROUGH_DIR" ]; then
  rm -rf "$(dirname "$WALKTHROUGH_DIR")"
fi

if [ $failed -gt 0 ]; then
  echo ""
  echo "Warning: $failed recording(s) failed."
fi

# Copy to docs
DOCS_VIDEO_DIR="$SCRIPT_DIR/../../../website/static/video/bowrain-cli"
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
