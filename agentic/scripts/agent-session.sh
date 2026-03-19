#!/usr/bin/env bash
#
# agent-session.sh — Run a single agent session via the Bowrain API.
#
# This script simulates an agent working session: fetching blocks,
# reviewing translations, and recording activity. It uses the REST API
# directly (the same endpoints the web UI calls).
#
# Usage:
#   AGENT=sophie ROLE=translator LOCALE=fr-FR ./agent-session.sh
#
# Environment:
#   BOWRAIN_URL     Server URL (default: http://localhost:8080)
#   JWT_SECRET      JWT signing secret (default: dev-secret-change-in-production)
#   WORKSPACE       Workspace slug (default: excalidraw-l10n)
#   PROJECT_ID      Project ID (default: auto-detected)
#   AGENT           Agent short name (required)
#   ROLE            Agent role: translator|reviewer|engineer (required)
#   LOCALE          Target locale for translators (e.g., fr-FR)
#   BATCH_SIZE      Number of blocks to process per session (default: 10)

set -euo pipefail

BOWRAIN_URL="${BOWRAIN_URL:-http://localhost:8080}"
JWT_SECRET="${JWT_SECRET:-dev-secret-change-in-production}"
WORKSPACE="${WORKSPACE:-excalidraw-l10n}"
AGENT="${AGENT:?AGENT is required}"
ROLE="${ROLE:?ROLE is required}"
LOCALE="${LOCALE:-}"
BATCH_SIZE="${BATCH_SIZE:-10}"

# --- Generate JWT token ---
TOKEN=$(python3 -c "
import hmac,hashlib,base64,json,time
def b64url(d): return base64.urlsafe_b64encode(d).rstrip(b'=').decode()
s='${JWT_SECRET}'; n=int(time.time())
h=b64url(json.dumps({'alg':'HS256','typ':'JWT'},separators=(',',':')).encode())
p=b64url(json.dumps({'sub':'agent-${AGENT}','email':'${AGENT}@${WORKSPACE}.bowrain.test','name':'Agent ${AGENT}','iat':n,'exp':n+3600},separators=(',',':')).encode())
m=f'{h}.{p}'.encode()
print(f'{h}.{p}.{b64url(hmac.new(s.encode(),m,hashlib.sha256).digest())}')
")

API="${BOWRAIN_URL}/api/v1/workspaces/${WORKSPACE}"

api() {
  curl -sf "$API/$1" -H "Authorization: Bearer ${TOKEN}" "${@:2}"
}

echo "[$(date -Iseconds)] Agent ${AGENT} (${ROLE}) starting session..."

# --- Auto-detect project ---
if [ -z "${PROJECT_ID:-}" ]; then
  PROJECT_ID=$(api "projects" | python3 -c "import json,sys; ps=json.load(sys.stdin); print(ps[0]['id'] if ps else '')")
fi
if [ -z "$PROJECT_ID" ]; then
  echo "ERROR: No project found in workspace ${WORKSPACE}"
  exit 1
fi
echo "  Project: ${PROJECT_ID}"

# --- Role-specific work ---
case "$ROLE" in
  translator)
    [ -z "$LOCALE" ] && { echo "ERROR: LOCALE required for translator"; exit 1; }
    echo "  Translating ${LOCALE}, batch size ${BATCH_SIZE}"

    # Fetch untranslated blocks (blocks without target for this locale)
    BLOCKS=$(api "projects/${PROJECT_ID}/sync/blocks?limit=${BATCH_SIZE}")
    TOTAL=$(echo "$BLOCKS" | python3 -c "import json,sys; bs=json.load(sys.stdin); print(len([b for b in bs if '${LOCALE}' not in b.get('targets',{})]))")
    echo "  Found ${TOTAL} untranslated blocks for ${LOCALE}"

    if [ "$TOTAL" -eq 0 ]; then
      echo "  All blocks translated for ${LOCALE}. Session complete."
      exit 0
    fi

    echo "  Session complete. ${TOTAL} blocks would be translated."
    ;;

  reviewer)
    echo "  Reviewing translations across all locales"

    # Fetch blocks and check for quality issues
    BLOCKS=$(api "projects/${PROJECT_ID}/sync/blocks?limit=100")
    STATS=$(echo "$BLOCKS" | python3 -c "
import json,sys
blocks = json.load(sys.stdin)
total = len(blocks)
locales = {}
for b in blocks:
    for loc in b.get('targets', {}):
        locales[loc] = locales.get(loc, 0) + 1
print(f'Total blocks: {total}')
for loc, count in sorted(locales.items()):
    print(f'  {loc}: {count}/{total} ({count*100//total}%)')
")
    echo "$STATS"
    echo "  Review session complete."
    ;;

  engineer)
    echo "  Checking project status and sync state"

    # Get project info from list
    api "projects" | python3 -c "
import json,sys
ps = json.load(sys.stdin)
for p in ps:
    if p['id'] == '${PROJECT_ID}':
        print(f'  Project: {p[\"name\"]}')
        print(f'  Source: {p[\"default_source_language\"]}')
        print(f'  Targets: {\", \".join(p[\"target_languages\"])}')
"

    # Check block count
    BLOCKS=$(api "projects/${PROJECT_ID}/sync/blocks?limit=2000")
    COUNT=$(echo "$BLOCKS" | python3 -c "import json,sys; print(len(json.load(sys.stdin)))")
    echo "  Total blocks: ${COUNT}"
    echo "  Engineering session complete."
    ;;

  *)
    echo "ERROR: Unknown role ${ROLE}. Use translator|reviewer|engineer"
    exit 1
    ;;
esac

echo "[$(date -Iseconds)] Agent ${AGENT} session finished."
