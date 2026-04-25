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
#   2. (--from-az) Mints a BOWRAIN_ADMIN_TOKEN by reading the Keycloak
#      master admin secret from Azure Key Vault, creating an `agent-bot`
#      service-account client in the bowrain-admin realm if absent, and
#      issuing a client_credentials access token. AdminGuard checks only
#      issuer/audience (not roles) so any bowrain-admin-realm token works.
#   3. Finds-or-creates the agent-sandbox workspace.
#   4. (If admin token present) Bumps the workspace plan to Pro so token
#      creation works (Pro+ plan gates the API-access feature).
#   5. Revokes any existing token with the same name (idempotency).
#   6. Creates a new long-lived workspace-scoped API token.
#   7. Writes outputs to .env.local and/or runs `gh secret set`.
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
#   scripts/scaffold-agent-tokens.sh --from-az              # mint admin token via Azure
#                                    --rg <rg>              # ↳ resource group (required with --from-az)
#                                    --env dev|prod         # ↳ infra environment (default: dev)
#
# Environment variables
# ---------------------
#   BOWRAIN_BACKEND_URL    Override --server (default https://dev.bowrain.cloud)
#   BOWRAIN_ADMIN_TOKEN    Pre-baked admin OIDC access token. Skips --from-az.
#   BOWRAIN_USER_EMAIL     Email used in device-auth (default: claude-agent@bowrain.cloud)
#   BOWRAIN_USER_NAME      Display name in device-auth (default: Claude Agent)
#   AZ_RESOURCE_GROUP      Default for --rg
#   AZ_INFRA_ENV           Default for --env (default: dev)

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
DO_FROM_AZ=false
AZ_RG="${AZ_RESOURCE_GROUP:-}"
AZ_ENV="${AZ_INFRA_ENV:-dev}"
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
    --from-az)       DO_FROM_AZ=true ;;
    --rg)            AZ_RG="$2"; shift ;;
    --env)           AZ_ENV="$2"; shift ;;
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

# ── (Optional) Mint admin token from Azure ───────────────────────────────
#
# Reads the Keycloak master admin secret from Key Vault, ensures an
# `agent-bot` service-account client exists in the bowrain-admin realm,
# and mints a client_credentials access token. AdminGuard
# (bowrain/billing/middleware.go) only checks issuer + audience — not
# roles — so any bowrain-admin-realm token works.

mint_admin_token_via_az() {
  local env="$1" rg="$2"
  need az
  local prefix="bowrain-${env}"
  local kv_name="kv-${prefix}"
  local kc_app="ca-${prefix}-keycloak"

  echo "==> Minting admin token via Azure"
  echo "    env:        $env (prefix: $prefix)"
  echo "    rg:         $rg"
  echo "    keycloak:   $kc_app"
  echo "    keyvault:   $kv_name"

  local kc_fqdn
  kc_fqdn="$(az containerapp show -g "$rg" -n "$kc_app" \
    --query 'properties.configuration.ingress.fqdn' -o tsv 2>/dev/null)"
  [ -n "$kc_fqdn" ] || { echo "    ✗ keycloak fqdn not found" >&2; return 1; }

  local kc_admin_pass
  kc_admin_pass="$(az keyvault secret show \
    --vault-name "$kv_name" --name keycloak-admin-password \
    --query value -o tsv 2>/dev/null)"
  [ -n "$kc_admin_pass" ] || { echo "    ✗ keycloak-admin-password not in Key Vault" >&2; return 1; }

  echo "    ✓ resolved keycloak: https://${kc_fqdn}"

  # Step 1: master-realm token via temp-admin client_credentials.
  local master_tok
  master_tok="$(curl -sf "https://${kc_fqdn}/realms/master/protocol/openid-connect/token" \
    -d "client_id=temp-admin" \
    -d "client_secret=${kc_admin_pass}" \
    -d "grant_type=client_credentials" | jq -r '.access_token // empty')"
  [ -n "$master_tok" ] || { echo "    ✗ master-realm token failed" >&2; return 1; }

  # Step 2: ensure agent-bot service-account client exists in bowrain-admin realm.
  local realm_url="https://${kc_fqdn}/admin/realms/bowrain-admin"
  local client_uuid
  client_uuid="$(curl -sf "$realm_url/clients?clientId=agent-bot" \
    -H "Authorization: Bearer $master_tok" | jq -r '.[0].id // empty')"
  if [ -z "$client_uuid" ]; then
    echo "    ↳ creating agent-bot service-account client in bowrain-admin realm"
    curl -sf -X POST "$realm_url/clients" \
      -H "Authorization: Bearer $master_tok" \
      -H "Content-Type: application/json" \
      -d '{
        "clientId": "agent-bot",
        "enabled": true,
        "protocol": "openid-connect",
        "publicClient": false,
        "serviceAccountsEnabled": true,
        "directAccessGrantsEnabled": false,
        "standardFlowEnabled": false,
        "implicitFlowEnabled": false,
        "clientAuthenticatorType": "client-secret"
      }' >/dev/null || { echo "    ✗ failed to create agent-bot client" >&2; return 1; }
    client_uuid="$(curl -sf "$realm_url/clients?clientId=agent-bot" \
      -H "Authorization: Bearer $master_tok" | jq -r '.[0].id // empty')"
    [ -n "$client_uuid" ] || { echo "    ✗ could not look up agent-bot UUID" >&2; return 1; }
  fi

  # Step 3: read agent-bot's client secret.
  local agent_secret
  agent_secret="$(curl -sf "$realm_url/clients/$client_uuid/client-secret" \
    -H "Authorization: Bearer $master_tok" | jq -r '.value // empty')"
  [ -n "$agent_secret" ] || { echo "    ✗ could not read agent-bot client secret" >&2; return 1; }

  # Step 4: mint the bowrain-admin-realm access token.
  BOWRAIN_ADMIN_TOKEN="$(curl -sf \
    "https://${kc_fqdn}/realms/bowrain-admin/protocol/openid-connect/token" \
    -d "grant_type=client_credentials" \
    -d "client_id=agent-bot" \
    -d "client_secret=${agent_secret}" | jq -r '.access_token // empty')"
  [ -n "$BOWRAIN_ADMIN_TOKEN" ] || { echo "    ✗ admin token mint failed" >&2; return 1; }
  export BOWRAIN_ADMIN_TOKEN

  echo "    ✓ admin token minted (bowrain-admin realm via agent-bot)"
}

if $DO_FROM_AZ; then
  if [ -z "${BOWRAIN_ADMIN_TOKEN:-}" ]; then
    [ -n "$AZ_RG" ] || { echo "--from-az requires --rg <resource-group>" >&2; exit 1; }
    mint_admin_token_via_az "$AZ_ENV" "$AZ_RG"
  else
    echo "==> Skipping --from-az (BOWRAIN_ADMIN_TOKEN already set)"
  fi
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
