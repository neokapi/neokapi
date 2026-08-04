#!/usr/bin/env bash
# Mint a long-lived workspace API token via the device-auth flow.
#
# device-auth.sh hands back an access JWT, which expires on the access-token
# TTL — fine for one command, wrong for a dogfood loop or a CI job, where the
# mid-session expiry surfaces as an "invalid token" push half an hour in. This
# script trades that JWT for a workspace API token (the api_tokens table), the
# credential BOWRAIN_AUTH_TOKEN is actually meant to carry in long-running use.
#
# Usage:
#   BOWRAIN_AUTH_TOKEN=$(bash dev-token.sh http://localhost:8080 bowrain-hq)
#   BOWRAIN_AUTH_TOKEN=$(bash dev-token.sh http://localhost:8080 bowrain-hq user@example.com "User Name" 30)

set -euo pipefail

SERVER_URL="${1:?Usage: dev-token.sh <server-url> <workspace-slug> [email] [name] [expire-days]}"
WORKSPACE="${2:?Usage: dev-token.sh <server-url> <workspace-slug> [email] [name] [expire-days]}"
EMAIL="${3:-ci@bowrain.cloud}"
NAME="${4:-CI Runner}"
EXPIRE_DAYS="${5:-30}"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
JWT=$(bash "$SCRIPT_DIR/device-auth.sh" "$SERVER_URL" "$EMAIL" "$NAME")

TOKEN=$(curl -sf -X POST \
  -H "Authorization: Bearer $JWT" \
  -H 'Content-Type: application/json' \
  -d "{\"name\":\"dev-token $(date -u +%Y-%m-%dT%H:%M:%SZ)\",\"expire_days\":$EXPIRE_DAYS}" \
  "$SERVER_URL/api/v1/$WORKSPACE/tokens" | python3 -c "import sys,json;d=json.load(sys.stdin);print(d.get('token') or d.get('access_token') or '')")

if [ -z "$TOKEN" ]; then
  echo "Failed to mint API token for workspace $WORKSPACE" >&2
  exit 1
fi

echo "$TOKEN"
