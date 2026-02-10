#!/bin/bash
# Start the e2e Docker Compose stack for VHS tape recordings.
# Seeds a test user and exports GOKAPI_TOKEN for use in tapes.
set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
E2E_DIR="$SCRIPT_DIR/../../e2e"

# Check for Docker
if ! command -v docker &> /dev/null; then
  echo "Docker not found. Server-backed tapes will be skipped."
  exit 1
fi

echo "Starting e2e stack for recordings..."
bash "$E2E_DIR/setup.sh"

# Perform device auth flow to get a token.
# Step 1: Start device auth.
START_RESP=$(curl -sf -X POST -d "client_id=vhs-recorder" \
  http://localhost:8080/api/v1/auth/device/start)

DEVICE_CODE=$(echo "$START_RESP" | python3 -c "import sys,json; print(json.load(sys.stdin)['device_code'])")
USER_CODE=$(echo "$START_RESP" | python3 -c "import sys,json; print(json.load(sys.stdin)['user_code'])")

# Step 2: Verify with test user.
curl -sf -X POST -d "user_code=$USER_CODE&email=admin@example.com&name=Admin User" \
  http://localhost:8080/api/v1/auth/device/verify > /dev/null

# Step 3: Poll for token.
TOKEN_RESP=$(curl -sf -X POST -d "device_code=$DEVICE_CODE&grant_type=urn:ietf:params:oauth:grant-type:device_code" \
  http://localhost:8080/api/v1/auth/device/poll)

GOKAPI_TOKEN=$(echo "$TOKEN_RESP" | python3 -c "import sys,json; print(json.load(sys.stdin)['access_token'])")

if [ -z "$GOKAPI_TOKEN" ]; then
  echo "Failed to obtain token."
  exit 1
fi

# Create a default workspace for demos.
curl -sf -X POST \
  -H "Authorization: Bearer $GOKAPI_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"name":"Personal","slug":"personal"}' \
  http://localhost:8080/api/v1/workspaces > /dev/null 2>&1 || true

echo ""
echo "Server ready. Token obtained."
echo "Export with: export GOKAPI_TOKEN=$GOKAPI_TOKEN"

# Write token to a temp file for other scripts to source.
TOKEN_FILE="$SCRIPT_DIR/.server-token"
echo "export GOKAPI_TOKEN=$GOKAPI_TOKEN" > "$TOKEN_FILE"
