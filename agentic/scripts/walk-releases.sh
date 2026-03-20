#!/usr/bin/env bash
#
# walk-releases.sh — Walk excalidraw releases and push content to Bowrain.
#
# For each release tag (v0.14.0 through v0.18.0), this script:
#   1. Extracts en.json, flattens nested keys to dot-separated block names
#   2. Computes the delta vs the previous release
#   3. Pushes source blocks via the sync/push API
#   4. Extracts fr-FR.json and de-DE.json community translations
#   5. Pushes translations via the editor block update API
#   6. Creates a stream named after the release tag
#   7. Logs progress with key counts and translation coverage
#
# Usage:
#   ./walk-releases.sh [--workspace SLUG] [--project-id ID] [--reset]
#
# Environment:
#   BOWRAIN_URL     Server URL (default: http://localhost:8080)
#   JWT_SECRET      JWT signing secret (default: dev-secret-change-in-production)
#   FORK_REPO       Path to the excalidraw fork (default: /tmp/excalidraw-check)

set -euo pipefail

BOWRAIN_URL="${BOWRAIN_URL:-http://localhost:8080}"
JWT_SECRET="${JWT_SECRET:-dev-secret-change-in-production}"
FORK_REPO="${FORK_REPO:-/tmp/excalidraw-check}"
WORKSPACE="${WORKSPACE:-excalidraw-l10n}"
PROJECT_ID="${PROJECT_ID:-}"
PROJECT_NAME="${PROJECT_NAME:-Excalidraw}"
RESET="${RESET:-false}"

# Release tags to walk (major versions only)
RELEASE_TAGS=(v0.14.0 v0.14.2 v0.15.0 v0.16.0 v0.17.0 v0.18.0)

# Temp directory for intermediate files
TMPDIR_WALK=$(mktemp -d)
trap 'rm -rf "$TMPDIR_WALK"' EXIT

# Parse arguments
while [[ $# -gt 0 ]]; do
  case "$1" in
    --workspace) WORKSPACE="$2"; shift 2 ;;
    --project-id) PROJECT_ID="$2"; shift 2 ;;
    --reset) RESET=true; shift ;;
    *) echo "Unknown option: $1"; exit 1 ;;
  esac
done

# --- JWT token generation ---
generate_token() {
  local sub="$1" email="$2" name="$3"
  python3 << PYEOF
import hmac,hashlib,base64,json,time
def b64url(d): return base64.urlsafe_b64encode(d).rstrip(b'=').decode()
s='${JWT_SECRET}'; n=int(time.time())
h=b64url(json.dumps({'alg':'HS256','typ':'JWT'},separators=(',',':')).encode())
p=b64url(json.dumps({'sub':'$sub','email':'$email','name':'$name','iat':n,'exp':n+365*86400},separators=(',',':')).encode())
m=f'{h}.{p}'.encode()
print(f'{h}.{p}.{b64url(hmac.new(s.encode(),m,hashlib.sha256).digest())}')
PYEOF
}

TOKEN=$(generate_token "agent-alex" "alex@${WORKSPACE}.bowrain.test" "Agent Alex")
API="${BOWRAIN_URL}/api/v1/workspaces/${WORKSPACE}"

api_get() {
  curl -sf "$API/$1" -H "Authorization: Bearer ${TOKEN}"
}

api_post() {
  local url="$1" body_file="$2"
  curl -sf "$API/$url" -H "Authorization: Bearer ${TOKEN}" \
    -H "Content-Type: application/json" -d "@${body_file}"
}

api_post_inline() {
  local url="$1" body="$2"
  curl -sf "$API/$url" -H "Authorization: Bearer ${TOKEN}" \
    -H "Content-Type: application/json" -d "$body"
}

api_put_file() {
  local url="$1" body_file="$2"
  curl -sf "$API/$url" -H "Authorization: Bearer ${TOKEN}" \
    -H "Content-Type: application/json" -X PUT -d "@${body_file}"
}

api_delete() {
  curl -sf "$API/$1" -H "Authorization: Bearer ${TOKEN}" -X DELETE
}

# --- Locale file path for a given tag ---
locale_path() {
  local tag="$1" locale="$2"
  case "$tag" in
    v0.18.0) echo "packages/excalidraw/locales/${locale}.json" ;;
    *)       echo "src/locales/${locale}.json" ;;
  esac
}

# --- Extract and flatten a locale JSON from git into a file ---
extract_flat_json() {
  local tag="$1" locale="$2" outfile="$3"
  local path
  path=$(locale_path "$tag" "$locale")
  cd "$FORK_REPO"
  git show "${tag}:${path}" 2>/dev/null | python3 -c "
import json, sys
def flatten(d, prefix=''):
    items = {}
    for k,v in d.items():
        key = prefix+'.'+k if prefix else k
        if isinstance(v, dict):
            items.update(flatten(v, key))
        else:
            items[key] = str(v)
    return items
try:
    d = json.load(sys.stdin)
    json.dump(flatten(d), open('$outfile', 'w'), ensure_ascii=False)
except:
    json.dump({}, open('$outfile', 'w'))
" 2>/dev/null || echo '{}' > "$outfile"
}

echo "========================================"
echo "  Excalidraw Release Walker"
echo "========================================"
echo "  Server:    ${BOWRAIN_URL}"
echo "  Workspace: ${WORKSPACE}"
echo "  Fork:      ${FORK_REPO}"
echo "  Releases:  ${RELEASE_TAGS[*]}"
echo ""

# --- Auto-detect or reset project ---
if [ -z "$PROJECT_ID" ]; then
  api_get "projects" > "$TMPDIR_WALK/projects.json"
  PROJECT_ID=$(python3 -c "
import json
ps = json.load(open('$TMPDIR_WALK/projects.json'))
for p in ps:
    if p['name']=='${PROJECT_NAME}':
        print(p['id'])
        break
" 2>/dev/null || echo "")
fi

if [ "$RESET" = "true" ] && [ -n "$PROJECT_ID" ]; then
  echo "Resetting: deleting project ${PROJECT_ID}..."
  # Delete uses the non-workspace-scoped JWT route
  curl -sf "${BOWRAIN_URL}/api/v1/projects/${PROJECT_ID}" \
    -H "Authorization: Bearer ${TOKEN}" -X DELETE || true
  sleep 1
  PROJECT_ID=""
fi

if [ -z "$PROJECT_ID" ]; then
  echo "Creating project '${PROJECT_NAME}'..."
  echo "{\"name\":\"${PROJECT_NAME}\",\"default_source_language\":\"en\",\"target_languages\":[\"fr-FR\",\"de-DE\"],\"workspace\":\"${WORKSPACE}\"}" > "$TMPDIR_WALK/create_project.json"
  api_post "projects" "$TMPDIR_WALK/create_project.json" > "$TMPDIR_WALK/created.json"
  PROJECT_ID=$(python3 -c "import json; print(json.load(open('$TMPDIR_WALK/created.json'))['id'])")
  echo "  Created project: ${PROJECT_ID}"
fi

echo "  Project ID: ${PROJECT_ID}"
echo ""

# --- Walk each release ---
echo '{}' > "$TMPDIR_WALK/prev_keys.json"

for tag in "${RELEASE_TAGS[@]}"; do
  echo "--- Release: ${tag} ---"

  # Extract English source
  extract_flat_json "$tag" "en" "$TMPDIR_WALK/en_curr.json"
  EN_COUNT=$(python3 -c "import json; print(len(json.load(open('$TMPDIR_WALK/en_curr.json'))))")

  # Compute delta
  python3 -c "
import json
prev = json.load(open('$TMPDIR_WALK/prev_keys.json'))
curr = json.load(open('$TMPDIR_WALK/en_curr.json'))
new_keys = [k for k in curr if k not in prev]
removed_keys = [k for k in prev if k not in curr]
changed_keys = [k for k in curr if k in prev and curr[k] != prev[k]]
print(f'+{len(new_keys)} new, {len(changed_keys)} changed, -{len(removed_keys)} removed, {len(curr)} total')
" > "$TMPDIR_WALK/delta.txt"
  DELTA_MSG=$(cat "$TMPDIR_WALK/delta.txt")

  # Build blocks array for push
  python3 -c "
import json
data = json.load(open('$TMPDIR_WALK/en_curr.json'))
blocks = []
for key, text in data.items():
    blocks.append({
        'id': key,
        'text': text,
        'name': key,
        'item_name': 'en.json'
    })
json.dump({'blocks': blocks}, open('$TMPDIR_WALK/push_body.json', 'w'), ensure_ascii=False)
"

  # Push source blocks
  api_post "projects/${PROJECT_ID}/sync/push" "$TMPDIR_WALK/push_body.json" > /dev/null 2>&1 || echo "  WARNING: push failed"
  echo "  Source: ${DELTA_MSG}"

  # Save current keys as previous for next iteration
  cp "$TMPDIR_WALK/en_curr.json" "$TMPDIR_WALK/prev_keys.json"

  # Fetch all blocks to get server-side IDs for translation updates
  api_get "projects/${PROJECT_ID}/sync/blocks" > "$TMPDIR_WALK/blocks.json" 2>/dev/null || echo '[]' > "$TMPDIR_WALK/blocks.json"

  # Build name-to-id map
  python3 -c "
import json
blocks = json.load(open('$TMPDIR_WALK/blocks.json'))
mapping = {b['name']: b['id'] for b in blocks}
json.dump(mapping, open('$TMPDIR_WALK/block_map.json', 'w'))
"

  # Push translations for each target locale
  for LOCALE in fr-FR de-DE; do
    extract_flat_json "$tag" "$LOCALE" "$TMPDIR_WALK/trans_${LOCALE}.json"

    # Generate translation update commands
    python3 << PYEOF
import json, sys

translations = json.load(open('$TMPDIR_WALK/trans_${LOCALE}.json'))
block_map = json.load(open('$TMPDIR_WALK/block_map.json'))

matched = 0
commands = []
for key, text in translations.items():
    if key in block_map:
        bid = block_map[key]
        body = json.dumps({'target_locale': '${LOCALE}', 'text': text}, ensure_ascii=False)
        commands.append({'bid': bid, 'body': body, 'key': key})
        matched += 1

# Write commands file
json.dump(commands, open('$TMPDIR_WALK/trans_cmds_${LOCALE}.json', 'w'), ensure_ascii=False)
print(f'{matched}/{len(translations)}')
PYEOF
    MATCH_INFO=$(python3 << PYEOF
import json
cmds = json.load(open('$TMPDIR_WALK/trans_cmds_${LOCALE}.json'))
trans = json.load(open('$TMPDIR_WALK/trans_${LOCALE}.json'))
print(f'{len(cmds)}/{len(trans)}')
PYEOF
)

    # Execute translation updates
    python3 << PYEOF
import json, urllib.request, sys

cmds = json.load(open('$TMPDIR_WALK/trans_cmds_${LOCALE}.json'))
api_base = '${API}'
token = '${TOKEN}'
project_id = '${PROJECT_ID}'

pushed = 0
failed = 0
for cmd in cmds:
    bid = cmd['bid']
    body = cmd['body'].encode('utf-8')
    url = f'{api_base}/editor/projects/{project_id}/blocks/{bid}'
    req = urllib.request.Request(url, data=body, method='PUT')
    req.add_header('Authorization', f'Bearer {token}')
    req.add_header('Content-Type', 'application/json')
    try:
        urllib.request.urlopen(req)
        pushed += 1
    except Exception as e:
        failed += 1

print(f'  ${LOCALE}: pushed {pushed} translations ({failed} failed)', file=sys.stderr)
PYEOF

    echo "  ${LOCALE}: ${MATCH_INFO} keys matched"
  done

  # Create a stream named after the release tag (best effort, may fail if exists)
  echo "{\"name\":\"${tag}\",\"description\":\"Release ${tag}\"}" > "$TMPDIR_WALK/stream_body.json"
  api_post "projects/${PROJECT_ID}/streams" "$TMPDIR_WALK/stream_body.json" > /dev/null 2>&1 || true

  echo "  Release ${tag}: ${DELTA_MSG}"
  echo ""
done

echo "========================================"
echo "  Walk complete!"
echo "========================================"

# Final summary
api_get "projects/${PROJECT_ID}/sync/blocks" > "$TMPDIR_WALK/final_blocks.json" 2>/dev/null || echo '[]' > "$TMPDIR_WALK/final_blocks.json"
python3 -c "
import json
blocks = json.load(open('$TMPDIR_WALK/final_blocks.json'))
total = len(blocks)
locales = {}
for b in blocks:
    for loc, text in b.get('targets', {}).items():
        if text:
            locales[loc] = locales.get(loc, 0) + 1
print(f'  Total blocks: {total}')
for loc, count in sorted(locales.items()):
    pct = count * 100 // total if total > 0 else 0
    print(f'  {loc}: {count}/{total} ({pct}%)')
"
