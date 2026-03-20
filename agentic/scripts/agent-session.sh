#!/usr/bin/env bash
#
# agent-session.sh — Run a single agent session via the Bowrain API.
#
# This script runs a real agent working session: fetching blocks,
# pushing translations, reviewing coverage, and checking upstream.
# It uses the REST API directly (the same endpoints the web UI calls).
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
#   BATCH_SIZE      Number of blocks to process per session (default: 20)
#   FORK_REPO       Path to excalidraw fork (default: /tmp/excalidraw-check)

set -euo pipefail

BOWRAIN_URL="${BOWRAIN_URL:-http://localhost:8080}"
JWT_SECRET="${JWT_SECRET:-dev-secret-change-in-production}"
WORKSPACE="${WORKSPACE:-excalidraw-l10n}"
AGENT="${AGENT:?AGENT is required}"
ROLE="${ROLE:?ROLE is required}"
LOCALE="${LOCALE:-}"
BATCH_SIZE="${BATCH_SIZE:-20}"
FORK_REPO="${FORK_REPO:-/tmp/excalidraw-check}"

# Temp directory for intermediate files
TMPDIR_SESSION=$(mktemp -d)
trap 'rm -rf "$TMPDIR_SESSION"' EXIT

# --- Generate JWT token ---
TOKEN=$(python3 << PYEOF
import hmac,hashlib,base64,json,time
def b64url(d): return base64.urlsafe_b64encode(d).rstrip(b'=').decode()
s='${JWT_SECRET}'; n=int(time.time())
h=b64url(json.dumps({'alg':'HS256','typ':'JWT'},separators=(',',':')).encode())
p=b64url(json.dumps({'sub':'agent-${AGENT}','email':'${AGENT}@${WORKSPACE}.bowrain.test','name':'Agent ${AGENT}','iat':n,'exp':n+3600},separators=(',',':')).encode())
m=f'{h}.{p}'.encode()
print(f'{h}.{p}.{b64url(hmac.new(s.encode(),m,hashlib.sha256).digest())}')
PYEOF
)

API="${BOWRAIN_URL}/api/v1/workspaces/${WORKSPACE}"

api_get() {
  curl -sf "$API/$1" -H "Authorization: Bearer ${TOKEN}"
}

api_put_file() {
  local url="$1" body_file="$2"
  curl -sf "$API/$url" -H "Authorization: Bearer ${TOKEN}" \
    -H "Content-Type: application/json" -X PUT -d "@${body_file}"
}

echo "[$(date -Iseconds)] Agent ${AGENT} (${ROLE}) starting session..."

# --- Auto-detect project ---
if [ -z "${PROJECT_ID:-}" ]; then
  api_get "projects" > "$TMPDIR_SESSION/projects.json"
  PROJECT_ID=$(python3 -c "import json; ps=json.load(open('$TMPDIR_SESSION/projects.json')); print(ps[0]['id'] if ps else '')")
fi
if [ -z "$PROJECT_ID" ]; then
  echo "ERROR: No project found in workspace ${WORKSPACE}"
  exit 1
fi
echo "  Project: ${PROJECT_ID}"

# --- Helper: flatten nested JSON ---
flatten_json() {
  python3 -c "
import json, sys
def flatten(d, prefix=''):
    items = {}
    for k,v in d.items():
        key = prefix+'.'+k if prefix else k
        if isinstance(v, dict): items.update(flatten(v, key))
        else: items[key] = str(v)
    return items
d = json.load(sys.stdin)
json.dump(flatten(d), sys.stdout, ensure_ascii=False)
"
}

# --- Role-specific work ---
case "$ROLE" in
  translator)
    [ -z "$LOCALE" ] && { echo "ERROR: LOCALE required for translator"; exit 1; }
    echo "  Translating ${LOCALE}, batch size ${BATCH_SIZE}"

    # Fetch all blocks
    api_get "projects/${PROJECT_ID}/sync/blocks" > "$TMPDIR_SESSION/all_blocks.json"

    # Find untranslated blocks for this locale
    python3 << PYEOF
import json
blocks = json.load(open('$TMPDIR_SESSION/all_blocks.json'))
untranslated = [b for b in blocks if '$LOCALE' not in b.get('targets', {}) or not b['targets'].get('$LOCALE', '')]
json.dump(untranslated[:$BATCH_SIZE], open('$TMPDIR_SESSION/untranslated.json', 'w'), ensure_ascii=False)
print(f'{len(untranslated)} untranslated, processing {min(len(untranslated), $BATCH_SIZE)}')
PYEOF
    UNTRANS_INFO=$(python3 -c "import json; u=json.load(open('$TMPDIR_SESSION/untranslated.json')); print(len(u))")

    if [ "$UNTRANS_INFO" -eq 0 ]; then
      echo "  All blocks translated for ${LOCALE}. Session complete."
      exit 0
    fi

    # Try to get community translations from the fork repo
    COMMUNITY_FILE="$TMPDIR_SESSION/community.json"
    echo '{}' > "$COMMUNITY_FILE"
    if [ -d "$FORK_REPO" ]; then
      # Get the latest tag's locale file
      LATEST_TAG=$(cd "$FORK_REPO" && git tag -l 'v0.*' --sort=version:refname | tail -1)
      if [ -n "$LATEST_TAG" ]; then
        case "$LATEST_TAG" in
          v0.18.0) LOCALE_PATH="packages/excalidraw/locales/${LOCALE}.json" ;;
          *)       LOCALE_PATH="src/locales/${LOCALE}.json" ;;
        esac
        cd "$FORK_REPO" && git show "${LATEST_TAG}:${LOCALE_PATH}" 2>/dev/null | flatten_json > "$COMMUNITY_FILE" 2>/dev/null || echo '{}' > "$COMMUNITY_FILE"
      fi
    fi

    # Push translations
    python3 << PYEOF
import json, urllib.request, sys

untranslated = json.load(open('$TMPDIR_SESSION/untranslated.json'))
community = json.load(open('$COMMUNITY_FILE'))
api_base = '${API}'
token = '${TOKEN}'
project_id = '${PROJECT_ID}'
locale = '${LOCALE}'

pushed = 0
placeholder = 0
for block in untranslated:
    bid = block['id']
    name = block.get('name', '')
    source = block.get('source', '')

    # Look up community translation first
    if name in community:
        text = community[name]
    else:
        # Generate placeholder
        lang_prefix = locale.split('-')[0].upper()
        text = f'[{lang_prefix}] {source}'
        placeholder += 1

    body = json.dumps({'target_locale': locale, 'text': text}, ensure_ascii=False).encode('utf-8')
    url = f'{api_base}/editor/projects/{project_id}/blocks/{bid}'
    req = urllib.request.Request(url, data=body, method='PUT')
    req.add_header('Authorization', f'Bearer {token}')
    req.add_header('Content-Type', 'application/json')
    try:
        urllib.request.urlopen(req)
        pushed += 1
        if name in community:
            print(f'  Translated: {name} ({locale})')
        else:
            print(f'  Placeholder: {name} -> [{locale.split("-")[0].upper()}] {source[:40]}...')
    except Exception as e:
        print(f'  FAILED: {name}: {e}', file=sys.stderr)

print(f'  Summary: {pushed} translations pushed ({pushed - placeholder} from community, {placeholder} placeholders)')
PYEOF
    ;;

  reviewer)
    echo "  Reviewing translations across all locales"

    # Fetch all blocks
    api_get "projects/${PROJECT_ID}/sync/blocks" > "$TMPDIR_SESSION/all_blocks.json"

    # Analyze coverage and quality
    python3 << PYEOF
import json, sys

blocks = json.load(open('$TMPDIR_SESSION/all_blocks.json'))
total = len(blocks)
if total == 0:
    print('  No blocks found.')
    sys.exit(0)

locales = {}
placeholder_count = {}
empty_count = {}
for b in blocks:
    targets = b.get('targets', {})
    for loc, text in targets.items():
        if text:
            locales[loc] = locales.get(loc, 0) + 1
            # Detect placeholder translations
            if text.startswith('[') and '] ' in text[:8]:
                placeholder_count[loc] = placeholder_count.get(loc, 0) + 1
        else:
            empty_count[loc] = empty_count.get(loc, 0) + 1

print(f'  Total blocks: {total}')
alerts = []
for loc in sorted(set(list(locales.keys()) + list(empty_count.keys()))):
    translated = locales.get(loc, 0)
    placeholders = placeholder_count.get(loc, 0)
    real = translated - placeholders
    pct = translated * 100 // total
    real_pct = real * 100 // total
    print(f'  {loc}: {translated}/{total} ({pct}%) -- {real} real, {placeholders} placeholders')
    if pct < 90:
        alerts.append(f'{loc}: {pct}% coverage (below 90% threshold)')

if alerts:
    print()
    print('  ALERTS:')
    for a in alerts:
        print(f'    - {a}')
    # Write alerts to file for potential GH issue creation
    with open('$TMPDIR_SESSION/alerts.txt', 'w') as f:
        f.write('\\n'.join(alerts))
else:
    print()
    print('  All locales above 90% coverage threshold.')
PYEOF

    # If there are alerts and gh is available, file an issue
    if [ -f "$TMPDIR_SESSION/alerts.txt" ] && command -v gh &>/dev/null; then
      ALERTS=$(cat "$TMPDIR_SESSION/alerts.txt")
      echo ""
      echo "  Coverage alerts detected — filing GH issue..."
      gh issue create \
        --repo neokapi/agent-feedback \
        --title "[${AGENT_ID}] Coverage below 90% — $(date +%Y-%m-%d)" \
        --body "Agent **${AGENT_ID}** (reviewer) detected the following locales below the 90% coverage threshold:

${ALERTS}

Workspace: \`${WS_SLUG}\`
Project: \`${PROJECT_ID}\`" \
        --label "coverage-alert" 2>/dev/null || echo "  (could not create GH issue)"
    fi

    echo "  Review session complete."
    ;;

  engineer)
    echo "  Checking project status and upstream releases"

    # Get project info
    api_get "projects" > "$TMPDIR_SESSION/projects.json"
    python3 -c "
import json
ps = json.load(open('$TMPDIR_SESSION/projects.json'))
for p in ps:
    if p['id'] == '${PROJECT_ID}':
        print(f'  Project: {p[\"name\"]}')
        print(f'  Source: {p[\"default_source_language\"]}')
        print(f'  Targets: {\", \".join(p[\"target_languages\"])}')
"

    # Check block count
    api_get "projects/${PROJECT_ID}/sync/blocks" > "$TMPDIR_SESSION/all_blocks.json"
    BLOCK_COUNT=$(python3 -c "import json; print(len(json.load(open('$TMPDIR_SESSION/all_blocks.json'))))")
    echo "  Total blocks: ${BLOCK_COUNT}"

    # Check upstream for new releases
    if [ -d "$FORK_REPO" ]; then
      LATEST_TAG=$(cd "$FORK_REPO" && git tag -l 'v0.*' --sort=version:refname | tail -1)
      ALL_TAGS=$(cd "$FORK_REPO" && git tag -l 'v0.*' --sort=version:refname | wc -l | tr -d ' ')
      echo "  Upstream tags: ${ALL_TAGS} total, latest: ${LATEST_TAG}"

      # Compare current block count with latest release
      case "$LATEST_TAG" in
        v0.18.0) LOCALE_PATH="packages/excalidraw/locales/en.json" ;;
        *)       LOCALE_PATH="src/locales/en.json" ;;
      esac
      if [ -n "$LATEST_TAG" ]; then
        UPSTREAM_KEYS=$(cd "$FORK_REPO" && git show "${LATEST_TAG}:${LOCALE_PATH}" 2>/dev/null | flatten_json | python3 -c "import json,sys; print(len(json.load(sys.stdin)))" 2>/dev/null || echo "?")
        echo "  Latest release (${LATEST_TAG}): ${UPSTREAM_KEYS} source keys"
        if [ "$UPSTREAM_KEYS" != "?" ]; then
          DIFF=$((BLOCK_COUNT - UPSTREAM_KEYS))
          if [ "$DIFF" -gt 0 ]; then
            echo "  Block delta: +${DIFF} (includes orphaned blocks from removed keys)"
          elif [ "$DIFF" -lt 0 ]; then
            echo "  Block delta: ${DIFF} (upstream has new content not yet pushed)"
          else
            echo "  Block delta: 0 (in sync with upstream)"
          fi
        fi
      fi
    else
      echo "  Fork repo not found at ${FORK_REPO}"
    fi

    echo "  Engineering session complete."
    ;;

  *)
    echo "ERROR: Unknown role ${ROLE}. Use translator|reviewer|engineer"
    exit 1
    ;;
esac

# --- Persist session memory ---
MEMORY_REPO_DIR="${MEMORY_REPO_DIR:-/tmp/agent-memory}"
AGENT_ID="agent-${AGENT}"

persist_memory() {
  if ! command -v git &>/dev/null; then return; fi

  # Clone or update the memory repo
  if [ -d "$MEMORY_REPO_DIR/.git" ]; then
    git -C "$MEMORY_REPO_DIR" pull --rebase origin main 2>/dev/null || true
  else
    git clone --depth 1 https://github.com/neokapi/agent-memory.git "$MEMORY_REPO_DIR" 2>/dev/null || return
  fi

  # Write session log
  local session_dir="$MEMORY_REPO_DIR/${AGENT_ID}/sessions"
  mkdir -p "$session_dir"
  local ts=$(date -u +%Y-%m-%dT%H:%M:%SZ)
  local fname="${ts//:/}-${ROLE}.md"
  cat > "$session_dir/$fname" <<MEMEOF
---
agent: ${AGENT_ID}
role: ${ROLE}
workspace: ${WORKSPACE}
timestamp: ${ts}
---

# Session: ${ROLE} (${ts})

$([ "${ROLE}" = "translator" ] && echo "- Locale: ${LOCALE}" || true)
- Workspace: ${WORKSPACE}
- Project: ${PROJECT_ID}
MEMEOF

  # Commit and push
  cd "$MEMORY_REPO_DIR"
  git add "${AGENT_ID}/" 2>/dev/null
  if ! git diff --cached --quiet 2>/dev/null; then
    git commit -m "${AGENT_ID}: ${ROLE} session ${ts}" 2>/dev/null || true
    git push origin main 2>/dev/null || true
  fi
}

persist_memory 2>/dev/null || true

echo "[$(date -Iseconds)] Agent ${AGENT} session finished."
