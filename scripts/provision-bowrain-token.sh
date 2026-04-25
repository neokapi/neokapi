#!/usr/bin/env bash
#
# provision-bowrain-token.sh — provision a long-lived Bowrain workspace API
# token. Print just the token on stdout.
#
# Composable building block: ensures a workspace exists, ensures it's on a
# plan that allows API tokens (Pro+), revokes any existing token with the
# same name (idempotency), and creates a new one. Prints the plaintext token
# (shown once by the server) to stdout. Logs go to stderr.
#
# Usage
# -----
#   BOWRAIN_TOKEN=$(scripts/provision-bowrain-token.sh \
#       --workspace agent-sandbox-asgeirf \
#       --token-name claude-rework)
#
# Flags
# -----
#   --workspace <slug>   Workspace slug (required)
#   --workspace-name <s> Display name when creating (default: humanized slug)
#   --server <url>       Backend URL (default: BOWRAIN_BACKEND_URL or https://dev.bowrain.cloud)
#   --token-name <name>  Token name (default: agent-bot)
#   --expires-days <n>   Token expiry (default: 90)
#   --reset              Delete + recreate the workspace before token creation
#
# Environment
# -----------
#   BOWRAIN_ADMIN_TOKEN  Admin OIDC token. Used to ensure Pro plan. Get one via:
#                          BOWRAIN_ADMIN_TOKEN=$(scripts/fetch-bowrain-admin-token.sh ...)
#                        Optional: if not set and the workspace is on Free plan,
#                        token creation will fail with a clear billing error.
#   BOWRAIN_USER_TOKEN   User JWT for create/list/delete on workspace + tokens.
#                        Optional: if not set, runs device-auth.sh to mint one.
#   BOWRAIN_BACKEND_URL  Override --server.
#   BOWRAIN_USER_EMAIL   Email for device-auth (default: claude-agent@bowrain.cloud)
#   BOWRAIN_USER_NAME    Name for device-auth (default: Claude Agent)

set -euo pipefail

SERVER_URL="${BOWRAIN_BACKEND_URL:-https://dev.bowrain.cloud}"
WORKSPACE_SLUG=""
WORKSPACE_NAME=""
TOKEN_NAME="agent-bot"
TOKEN_EXPIRES_DAYS=90
DO_RESET=false

while [ $# -gt 0 ]; do
  case "$1" in
    --workspace)       WORKSPACE_SLUG="$2"; shift ;;
    --workspace-name)  WORKSPACE_NAME="$2"; shift ;;
    --server)          SERVER_URL="$2"; shift ;;
    --token-name)      TOKEN_NAME="$2"; shift ;;
    --expires-days)    TOKEN_EXPIRES_DAYS="$2"; shift ;;
    --reset)           DO_RESET=true ;;
    -h|--help)         sed -n '1,/^set -euo/p' "$0" | sed 's/^# \?//' | head -n -1; exit 0 ;;
    *)                 echo "unknown flag: $1" >&2; exit 2 ;;
  esac
  shift
done

[ -n "$WORKSPACE_SLUG" ] || { echo "missing --workspace <slug>" >&2; exit 2; }
[ -n "$WORKSPACE_NAME" ] || WORKSPACE_NAME="$(echo "$WORKSPACE_SLUG" | tr '-' ' ')"

need() { command -v "$1" >/dev/null 2>&1 || { echo "missing tool: $1" >&2; exit 1; }; }
need curl
need jq

log() { printf '==> %s\n' "$*" >&2; }

# ── User JWT ─────────────────────────────────────────────────────────────
USER_TOKEN="${BOWRAIN_USER_TOKEN:-}"
if [ -z "$USER_TOKEN" ]; then
  REPO_ROOT="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
  DEVICE_AUTH="$REPO_ROOT/bowrain/scripts/device-auth.sh"
  [ -r "$DEVICE_AUTH" ] || { echo "BOWRAIN_USER_TOKEN unset and device-auth.sh not found at $DEVICE_AUTH" >&2; exit 1; }
  log "minting user JWT via device-auth.sh"
  USER_TOKEN="$(bash "$DEVICE_AUTH" "$SERVER_URL" \
    "${BOWRAIN_USER_EMAIL:-claude-agent@bowrain.cloud}" \
    "${BOWRAIN_USER_NAME:-Claude Agent}" 2>/dev/null | tail -1)"
  [ -n "$USER_TOKEN" ] || { echo "device-auth failed" >&2; exit 1; }
fi

api_user() {
  curl -sS \
    -H "Authorization: Bearer $USER_TOKEN" \
    -H "Content-Type: application/json" \
    "$@"
}

api_admin() {
  [ -n "${BOWRAIN_ADMIN_TOKEN:-}" ] || return 1
  curl -sS \
    -H "Authorization: Bearer $BOWRAIN_ADMIN_TOKEN" \
    -H "Content-Type: application/json" \
    "$@"
}

# ── Workspace lifecycle ──────────────────────────────────────────────────
WS_URL="$SERVER_URL/api/v1/$WORKSPACE_SLUG"

ws_exists() { api_user --fail "$WS_URL" >/dev/null 2>&1; }

if ws_exists; then
  if $DO_RESET; then
    log "deleting workspace $WORKSPACE_SLUG (--reset)"
    api_user -X DELETE "$WS_URL" >/dev/null
  else
    log "reusing workspace $WORKSPACE_SLUG"
  fi
fi

if ! ws_exists; then
  log "creating workspace $WORKSPACE_SLUG"
  body="$(jq -nc --arg n "$WORKSPACE_NAME" --arg s "$WORKSPACE_SLUG" '{name:$n, slug:$s}')"
  resp="$(api_user -X POST "$SERVER_URL/api/v1/workspaces" -d "$body")" || {
    echo "create workspace failed: $resp" >&2; exit 1; }
fi

# ── Plan upgrade (admin) ─────────────────────────────────────────────────
if [ -n "${BOWRAIN_ADMIN_TOKEN:-}" ]; then
  log "ensuring Pro plan via admin endpoint"
  api_admin -X PUT "$SERVER_URL/api/admin/workspaces/$WORKSPACE_SLUG/plan" \
    -d '{"plan":"pro"}' >/dev/null \
    || echo "  ⚠ plan upgrade failed; token creation may hit Pro guard" >&2
fi

# ── Idempotent token revoke ──────────────────────────────────────────────
# Tokens list endpoint returns a bare array on current bowrain-server; older
# versions returned {tokens:[...]}. Handle both.
existing="$(api_user "$WS_URL/tokens" 2>/dev/null \
  | jq -r --arg n "$TOKEN_NAME" '
      (if type == "array" then . else (.tokens // []) end)
      | .[]? | select(.name==$n) | .id
    ' 2>/dev/null || true)"
if [ -n "$existing" ]; then
  log "revoking existing tokens named '$TOKEN_NAME'"
  for tid in $existing; do
    api_user -X DELETE "$WS_URL/tokens/$tid" >/dev/null || true
  done
fi

# ── Create token ─────────────────────────────────────────────────────────
log "creating workspace API token: $TOKEN_NAME (expire_days=$TOKEN_EXPIRES_DAYS)"
body="$(jq -nc \
  --arg n "$TOKEN_NAME" \
  --argjson d "$TOKEN_EXPIRES_DAYS" \
  '{name:$n, expire_days:$d, scopes:["*"]}')"
resp="$(api_user -X POST "$WS_URL/tokens" -d "$body")"
TOKEN="$(echo "$resp" | jq -r '.token // empty')"
[ -n "$TOKEN" ] || { echo "token creation failed: $resp" >&2; exit 1; }

# Stdout: the token, nothing else.
printf '%s' "$TOKEN"
