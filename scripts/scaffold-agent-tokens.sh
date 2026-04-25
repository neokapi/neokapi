#!/usr/bin/env bash
#
# scaffold-agent-tokens.sh — provision a long-lived BOWRAIN_TOKEN for
# Claude (or any agent) to use against dev.bowrain.cloud, and distribute
# it to local development (.env.local) and/or CI (GitHub Action secrets).
#
# What it does
# ------------
#   1. Acquires a user JWT via the device-auth flow (delegates to
#      bowrain/scripts/device-auth.sh).
#   2. Finds-or-creates the agent-sandbox workspace.
#   3. (Optional) Bumps the workspace plan to Pro via admin endpoint
#      so token creation works (Pro+ plan gates the API-access feature).
#      Requires BOWRAIN_ADMIN_TOKEN env var.
#   4. Revokes any existing token with the same name (idempotency).
#   5. Creates a new long-lived workspace-scoped API token.
#   6. Writes outputs to .env.local and/or runs `gh secret set`.
#
# Usage
# -----
#   scripts/scaffold-agent-tokens.sh                        # both local + CI
#   scripts/scaffold-agent-tokens.sh --local                # local only
#   scripts/scaffold-agent-tokens.sh --ci                   # CI only
#   scripts/scaffold-agent-tokens.sh --reset                # delete + recreate workspace
#   scripts/scaffold-agent-tokens.sh --workspace foo        # override slug (default: agent-sandbox-$USER)
#   scripts/scaffold-agent-tokens.sh --server URL           # override backend URL
#   scripts/scaffold-agent-tokens.sh --expires-days 30      # token expiry (default 90)
#   scripts/scaffold-agent-tokens.sh --token-name claude    # token name (default: claude-rework)
#
# Environment variables
# ---------------------
#   BOWRAIN_BACKEND_URL    Override --server (default https://dev.bowrain.cloud)
#   BOWRAIN_ADMIN_TOKEN    Admin OIDC access token (paste from ctrl.bowrain.cloud
#                          devtools or admin login). Optional but enables plan upgrade.
#   BOWRAIN_USER_EMAIL     Email used in device-auth (default: claude-agent@bowrain.cloud)
#   BOWRAIN_USER_NAME      Display name in device-auth (default: Claude Agent)

set -euo pipefail

# ── Defaults ─────────────────────────────────────────────────────────────
SERVER_URL="${BOWRAIN_BACKEND_URL:-https://dev.bowrain.cloud}"
WORKSPACE_SLUG=""
WORKSPACE_NAME=""
TOKEN_NAME="claude-rework"
TOKEN_EXPIRES_DAYS=90
DO_LOCAL=true
DO_CI=true
DO_RESET=false
USER_EMAIL="${BOWRAIN_USER_EMAIL:-claude-agent@bowrain.cloud}"
USER_NAME="${BOWRAIN_USER_NAME:-Claude Agent}"

# ── Args ─────────────────────────────────────────────────────────────────
while [ $# -gt 0 ]; do
  case "$1" in
    --local)         DO_LOCAL=true; DO_CI=false ;;
    --ci)            DO_LOCAL=false; DO_CI=true ;;
    --both)          DO_LOCAL=true; DO_CI=true ;;
    --reset)         DO_RESET=true ;;
    --workspace)     WORKSPACE_SLUG="$2"; shift ;;
    --server)        SERVER_URL="$2"; shift ;;
    --expires-days)  TOKEN_EXPIRES_DAYS="$2"; shift ;;
    --token-name)    TOKEN_NAME="$2"; shift ;;
    -h|--help)       sed -n '1,/^set -euo/p' "$0" | sed 's/^# \?//' | head -n -1; exit 0 ;;
    *)               echo "unknown flag: $1" >&2; exit 2 ;;
  esac
  shift
done

# ── Pre-flight ───────────────────────────────────────────────────────────
need() { command -v "$1" >/dev/null 2>&1 || { echo "missing tool: $1" >&2; exit 1; }; }
need curl
need jq
need git
$DO_CI && need gh

REPO_ROOT="$(git rev-parse --show-toplevel)"
DEVICE_AUTH="$REPO_ROOT/bowrain/scripts/device-auth.sh"
ENV_FILE="$REPO_ROOT/.env.local"

[ -x "$DEVICE_AUTH" ] || chmod +x "$DEVICE_AUTH" 2>/dev/null
[ -r "$DEVICE_AUTH" ] || { echo "device-auth.sh missing at $DEVICE_AUTH" >&2; exit 1; }

if [ -z "$WORKSPACE_SLUG" ]; then
  user_part="$(echo "${USER:-anon}" | tr -dc 'a-z0-9' | head -c 16)"
  WORKSPACE_SLUG="agent-sandbox-${user_part}"
fi
if [ -z "$WORKSPACE_NAME" ]; then
  WORKSPACE_NAME="Claude Agent Sandbox (${USER:-anon})"
fi

# ── Acquire user token ───────────────────────────────────────────────────
echo "==> Acquiring user JWT via device auth"
echo "    server:    $SERVER_URL"
echo "    email:     $USER_EMAIL"
USER_TOKEN="$(bash "$DEVICE_AUTH" "$SERVER_URL" "$USER_EMAIL" "$USER_NAME" 2>&1 | tail -1)"
if [ -z "$USER_TOKEN" ] || [[ "$USER_TOKEN" =~ ^[Ff]ailed ]]; then
  echo "    ✗ device auth failed" >&2
  exit 1
fi
echo "    ✓ user token acquired"

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

ws_exists() {
  api_user --fail "$WS_URL" >/dev/null 2>&1
}

if ws_exists; then
  if $DO_RESET; then
    echo "==> --reset: deleting existing workspace $WORKSPACE_SLUG"
    api_user -X DELETE "$WS_URL" >/dev/null
  else
    echo "==> Reusing existing workspace $WORKSPACE_SLUG"
  fi
fi

if ! ws_exists; then
  echo "==> Creating workspace $WORKSPACE_SLUG"
  body="$(jq -nc --arg n "$WORKSPACE_NAME" --arg s "$WORKSPACE_SLUG" '{name:$n, slug:$s}')"
  resp="$(api_user -X POST "$SERVER_URL/api/v1/workspaces" -d "$body")" || {
    echo "    ✗ create failed: $resp" >&2; exit 1; }
fi
echo "    ✓ workspace ready: $WORKSPACE_SLUG"

# ── Plan upgrade (admin) ─────────────────────────────────────────────────
if [ -n "${BOWRAIN_ADMIN_TOKEN:-}" ]; then
  echo "==> Upgrading workspace plan to Pro via admin token"
  if api_admin -X PUT "$SERVER_URL/api/admin/workspaces/$WORKSPACE_SLUG/plan" \
       -d '{"plan":"pro"}' >/dev/null; then
    echo "    ✓ plan upgraded"
  else
    echo "    ⚠ plan upgrade failed; token creation may hit the Pro guard" >&2
  fi
else
  echo "==> Skipping plan upgrade (no BOWRAIN_ADMIN_TOKEN set)"
  echo "    If token creation fails with 402/403 (Pro plan required), either"
  echo "    set BOWRAIN_ADMIN_TOKEN or upgrade the workspace via admin portal."
fi

# ── Idempotency: revoke existing token with same name ────────────────────
echo "==> Checking for existing tokens named '$TOKEN_NAME'"
existing="$(api_user "$WS_URL/tokens" 2>/dev/null \
  | jq -r --arg n "$TOKEN_NAME" '.tokens[]? | select(.name==$n) | .id' || true)"
if [ -n "$existing" ]; then
  for tid in $existing; do
    echo "    ↳ revoking existing token $tid"
    api_user -X DELETE "$WS_URL/tokens/$tid" >/dev/null || true
  done
fi

# ── Create token ─────────────────────────────────────────────────────────
echo "==> Creating workspace API token: $TOKEN_NAME (expire_days=$TOKEN_EXPIRES_DAYS)"
body="$(jq -nc \
  --arg n "$TOKEN_NAME" \
  --argjson d "$TOKEN_EXPIRES_DAYS" \
  '{name:$n, expire_days:$d, scopes:["*"]}')"
resp="$(api_user -X POST "$WS_URL/tokens" -d "$body")"
BOWRAIN_TOKEN="$(echo "$resp" | jq -r '.token // empty')"
if [ -z "$BOWRAIN_TOKEN" ]; then
  echo "    ✗ token creation failed: $resp" >&2
  exit 1
fi
TOKEN_PREFIX="$(echo "$resp" | jq -r '.token_prefix // empty')"
echo "    ✓ token created (prefix: ${TOKEN_PREFIX:-?}, expires_in_days=$TOKEN_EXPIRES_DAYS)"

# ── Distribute: local .env.local ─────────────────────────────────────────
if $DO_LOCAL; then
  echo "==> Writing $ENV_FILE"
  upsert_env() {
    local key="$1" val="$2"
    if [ -f "$ENV_FILE" ] && grep -q "^${key}=" "$ENV_FILE"; then
      # in-place replace
      tmp="$(mktemp)"
      awk -v k="$key" -v v="$val" 'BEGIN{FS=OFS="="} $1==k {$2=v; print; next} 1' "$ENV_FILE" > "$tmp"
      mv "$tmp" "$ENV_FILE"
    else
      echo "${key}=${val}" >> "$ENV_FILE"
    fi
  }
  if [ ! -f "$ENV_FILE" ]; then
    {
      echo "# Created by scripts/scaffold-agent-tokens.sh"
      echo "# Re-run that script (or with --reset) to refresh."
    } > "$ENV_FILE"
    chmod 600 "$ENV_FILE"
  fi
  upsert_env BOWRAIN_BACKEND_URL "$SERVER_URL"
  upsert_env BOWRAIN_WORKSPACE   "$WORKSPACE_SLUG"
  upsert_env BOWRAIN_TOKEN       "$BOWRAIN_TOKEN"
  echo "    ✓ wrote $ENV_FILE"
fi

# ── Distribute: GitHub Actions ───────────────────────────────────────────
if $DO_CI; then
  echo "==> Setting GitHub Actions secret + variables"
  printf '%s' "$BOWRAIN_TOKEN" | gh secret set BOWRAIN_TOKEN --body -
  gh variable set BOWRAIN_BACKEND_URL --body "$SERVER_URL"   2>/dev/null \
    || echo "    ⚠ could not set BOWRAIN_BACKEND_URL variable (it may already exist with a different scope)"
  gh variable set BOWRAIN_WORKSPACE   --body "$WORKSPACE_SLUG" 2>/dev/null \
    || echo "    ⚠ could not set BOWRAIN_WORKSPACE variable"
  echo "    ✓ gh secret set + variables updated"
fi

echo
echo "Done."
echo "  BOWRAIN_BACKEND_URL=$SERVER_URL"
echo "  BOWRAIN_WORKSPACE=$WORKSPACE_SLUG"
echo "  BOWRAIN_TOKEN=*** (${TOKEN_PREFIX:-?}…, $TOKEN_EXPIRES_DAYS-day expiry)"
$DO_LOCAL && echo "  → local: $ENV_FILE"
$DO_CI    && echo "  → CI:    gh secret BOWRAIN_TOKEN + vars BOWRAIN_BACKEND_URL, BOWRAIN_WORKSPACE"
