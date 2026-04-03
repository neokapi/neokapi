#!/usr/bin/env bash
# Obtain a JWT token from a Bowrain server via the device auth flow.
# Outputs the token to stdout.
#
# Usage:
#   TOKEN=$(bash device-auth.sh https://dev.bowrain.cloud)
#   TOKEN=$(bash device-auth.sh http://localhost:8080 user@example.com "User Name")

set -euo pipefail

SERVER_URL="${1:?Usage: device-auth.sh <server-url> [email] [name]}"
EMAIL="${2:-ci@bowrain.cloud}"
NAME="${3:-CI Runner}"

START_RESP=$(curl -sf -X POST -d "client_id=ci" \
  "$SERVER_URL/api/v1/auth/device/start")
DEVICE_CODE=$(echo "$START_RESP" | python3 -c "import sys,json; print(json.load(sys.stdin)['device_code'])")
USER_CODE=$(echo "$START_RESP" | python3 -c "import sys,json; print(json.load(sys.stdin)['user_code'])")

curl -s -X POST \
  -d "user_code=$USER_CODE&email=$EMAIL&name=$NAME" \
  "$SERVER_URL/api/v1/auth/device/verify" > /dev/null

TOKEN=$(curl -sf -X POST \
  -d "device_code=$DEVICE_CODE&grant_type=urn:ietf:params:oauth:grant-type:device_code" \
  "$SERVER_URL/api/v1/auth/device/poll" | python3 -c "import sys,json; print(json.load(sys.stdin)['access_token'])")

if [ -z "$TOKEN" ]; then
  echo "Failed to obtain token" >&2
  exit 1
fi

echo "$TOKEN"
