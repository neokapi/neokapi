#!/usr/bin/env bash
#
# setup-workspace-api.sh — Create a Bowrain workspace and project via REST API.
#
# Usage: ./scripts/setup-workspace-api.sh <workspace-dir>
# Example: ./scripts/setup-workspace-api.sh workspaces/excalidraw
#
# Environment:
#   BOWRAIN_URL     Server URL (default: http://localhost:8080)
#   JWT_SECRET      JWT signing secret (required)

set -euo pipefail

BOWRAIN_URL="${BOWRAIN_URL:-http://localhost:8080}"
JWT_SECRET="${JWT_SECRET:?JWT_SECRET is required}"

if [ $# -lt 1 ]; then
  echo "Usage: $0 <workspace-dir>"
  exit 1
fi

WS_DIR="$1"
WS_YAML="${WS_DIR}/workspace.yaml"

if [ ! -f "$WS_YAML" ]; then
  echo "ERROR: ${WS_YAML} not found" >&2
  exit 1
fi

# Parse workspace.yaml
WS_NAME=$(grep '^name:' "$WS_YAML" | head -1 | sed 's/^name: *//')
WS_SLUG=$(grep '^slug:' "$WS_YAML" | head -1 | sed 's/^slug: *//')
WS_DESC=$(grep '^description:' "$WS_YAML" | head -1 | sed 's/^description: *"//' | sed 's/"$//')
SOURCE_LANG=$(grep '^source_language:' "$WS_YAML" | head -1 | sed 's/^source_language: *//')
TARGET_LANGS=$(grep -A 10 '^target_languages:' "$WS_YAML" | grep '^\s*-' | sed 's/.*- *//' | tr '\n' ',' | sed 's/,$//')

echo "Setting up workspace: ${WS_NAME} (${WS_SLUG})"
echo "  Source: ${SOURCE_LANG}"
echo "  Targets: ${TARGET_LANGS}"

# Generate token for the first agent (becomes workspace owner)
FIRST_AGENT=$(ls "$WS_DIR/agents/" 2>/dev/null | grep -v _template | head -1)
if [ -z "$FIRST_AGENT" ]; then
  echo "ERROR: No agents found in ${WS_DIR}/agents/"
  exit 1
fi

make_token() {
  local sub="$1" email="$2" name="$3"
  python3 -c "
import hmac,hashlib,base64,json,time
def b64url(d): return base64.urlsafe_b64encode(d).rstrip(b'=').decode()
s='${JWT_SECRET}'; n=int(time.time())
h=b64url(json.dumps({'alg':'HS256','typ':'JWT'},separators=(',',':')).encode())
p=b64url(json.dumps({'sub':s,'email':'$email','name':'$name','iat':n,'exp':n+365*86400}.items() | dict(sub='$sub').items(),separators=(',',':')).encode())
" 2>/dev/null || python3 -c "
import hmac,hashlib,base64,json,time
def b64url(d): return base64.urlsafe_b64encode(d).rstrip(b'=').decode()
s='${JWT_SECRET}'; n=int(time.time())
h=b64url(json.dumps({'alg':'HS256','typ':'JWT'},separators=(',',':')).encode())
p=b64url(json.dumps({'sub':'$sub','email':'$email','name':'$name','iat':n,'exp':n+365*86400},separators=(',',':')).encode())
m=f'{h}.{p}'.encode()
print(f'{h}.{p}.{b64url(hmac.new(s.encode(),m,hashlib.sha256).digest())}')
"
}

OWNER_TOKEN=$(make_token "agent-${FIRST_AGENT}" "${FIRST_AGENT}@${WS_SLUG}.bowrain.test" "Agent ${FIRST_AGENT}")

# Auto-provision owner user
curl -sf "${BOWRAIN_URL}/api/v1/workspaces" \
  -H "Authorization: Bearer ${OWNER_TOKEN}" > /dev/null

# Create workspace
echo ""
echo "Creating workspace..."
WS_RESULT=$(curl -s -X POST "${BOWRAIN_URL}/api/v1/workspaces" \
  -H "Authorization: Bearer ${OWNER_TOKEN}" \
  -H "Content-Type: application/json" \
  -d "{\"name\":\"${WS_NAME}\",\"slug\":\"${WS_SLUG}\",\"description\":\"${WS_DESC}\"}")

WS_ID=$(echo "$WS_RESULT" | python3 -c "import json,sys; d=json.load(sys.stdin); print(d.get('id',''))" 2>/dev/null)
if [ -z "$WS_ID" ]; then
  echo "  Workspace creation response: ${WS_RESULT}"
  echo "  (may already exist)"
  # Try to get existing workspace ID
  WS_ID=$(curl -sf "${BOWRAIN_URL}/api/v1/workspaces" \
    -H "Authorization: Bearer ${OWNER_TOKEN}" | \
    python3 -c "import json,sys; ws=json.load(sys.stdin); print(next((w['id'] for w in ws if w['slug']=='${WS_SLUG}'),''))" 2>/dev/null)
fi
echo "  Workspace ID: ${WS_ID}"

# Add other agents as members
echo ""
echo "Adding agent members..."
for agent_dir in "$WS_DIR/agents"/*/; do
  agent_name=$(basename "$agent_dir")
  [ "$agent_name" = "_template" ] && continue
  [ "$agent_name" = "$FIRST_AGENT" ] && continue

  agent_token=$(make_token "agent-${agent_name}" "${agent_name}@${WS_SLUG}.bowrain.test" "Agent ${agent_name}")
  # Auto-provision user
  curl -sf "${BOWRAIN_URL}/api/v1/workspaces" \
    -H "Authorization: Bearer ${agent_token}" > /dev/null

  # Add as member
  code=$(curl -s -o /dev/null -w "%{http_code}" -X POST \
    "${BOWRAIN_URL}/api/v1/workspaces/${WS_SLUG}/members" \
    -H "Authorization: Bearer ${OWNER_TOKEN}" \
    -H "Content-Type: application/json" \
    -d "{\"user_id\":\"agent-${agent_name}\",\"role\":\"member\"}")
  echo "  ${agent_name}: HTTP ${code}"
done

# Add dashboard viewer
DASH_TOKEN=$(make_token "agent-dashboard" "dashboard@bowrain.test" "Dashboard")
curl -sf "${BOWRAIN_URL}/api/v1/workspaces" \
  -H "Authorization: Bearer ${DASH_TOKEN}" > /dev/null
curl -s -o /dev/null -w "  dashboard: HTTP %{http_code}\n" -X POST \
  "${BOWRAIN_URL}/api/v1/workspaces/${WS_SLUG}/members" \
  -H "Authorization: Bearer ${OWNER_TOKEN}" \
  -H "Content-Type: application/json" \
  -d '{"user_id":"agent-dashboard","role":"viewer"}'

# Create project
echo ""
echo "Creating project..."
TARGET_JSON=$(echo "$TARGET_LANGS" | python3 -c "import sys; print('[' + ','.join('\"'+t.strip()+'\"' for t in sys.stdin.read().split(',') if t.strip()) + ']')")
PROJECT_RESULT=$(curl -s -X POST "${BOWRAIN_URL}/api/v1/workspaces/${WS_SLUG}/projects" \
  -H "Authorization: Bearer ${OWNER_TOKEN}" \
  -H "Content-Type: application/json" \
  -d "{\"name\":\"${WS_NAME}\",\"default_source_language\":\"${SOURCE_LANG}\",\"target_languages\":${TARGET_JSON}}")
PROJECT_ID=$(echo "$PROJECT_RESULT" | python3 -c "import json,sys; d=json.load(sys.stdin); print(d.get('id',''))" 2>/dev/null)
echo "  Project ID: ${PROJECT_ID}"

echo ""
echo "Done! Workspace '${WS_SLUG}' is ready."
echo "  Workspace ID: ${WS_ID}"
echo "  Project ID: ${PROJECT_ID}"
