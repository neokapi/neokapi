#!/usr/bin/env bash
#
# fetch-bowrain-admin-token.sh — print a Bowrain admin OIDC access token on stdout.
#
# Composable building block: mints a short-lived (Keycloak default ~5 min)
# access token issued by the bowrain-admin Keycloak realm. The bowrain-server
# AdminGuard (bowrain/billing/middleware.go:122-160) verifies issuer + audience
# only — not roles — so any bowrain-admin-realm token grants admin access.
#
# Usage
# -----
#   BOWRAIN_ADMIN_TOKEN=$(scripts/fetch-bowrain-admin-token.sh --rg bowrain-dev-rg --env dev)
#   curl -H "Authorization: Bearer $BOWRAIN_ADMIN_TOKEN" https://dev.bowrain.cloud/api/admin/...
#
# Flags
# -----
#   --rg <rg>        Azure resource group (required; or set AZ_RESOURCE_GROUP)
#   --env dev|prod   Infra environment (default: dev; or set AZ_INFRA_ENV)
#
# How it works
# ------------
#   1. az containerapp show ca-bowrain-{env}-keycloak     → keycloak FQDN
#   2. az keyvault secret show kv-bowrain-{env}/keycloak-admin-password
#   3. POST master/.../token (temp-admin client_credentials)         → master token
#   4. Ensure agent-bot service-account client exists in bowrain-admin realm
#      (idempotent; created on first run)
#   5. GET .../clients/{uuid}/client-secret                          → agent-bot secret
#   6. POST realms/bowrain-admin/.../token (client_credentials)      → admin token
#
# Errors
# ------
# Anything that fails goes to stderr; only the token goes to stdout, so the
# `BOWRAIN_ADMIN_TOKEN=$(...)` capture is safe.

set -euo pipefail

AZ_RG="${AZ_RESOURCE_GROUP:-}"
AZ_ENV="${AZ_INFRA_ENV:-dev}"

while [ $# -gt 0 ]; do
  case "$1" in
    --rg)        AZ_RG="$2"; shift ;;
    --env)       AZ_ENV="$2"; shift ;;
    -h|--help)   sed -n '1,/^set -euo/p' "$0" | sed 's/^# \?//' | head -n -1; exit 0 ;;
    *)           echo "unknown flag: $1" >&2; exit 2 ;;
  esac
  shift
done

[ -n "$AZ_RG" ] || { echo "missing --rg <resource-group>" >&2; exit 2; }

need() { command -v "$1" >/dev/null 2>&1 || { echo "missing tool: $1" >&2; exit 1; }; }
need az
need curl
need jq

PREFIX="bowrain-${AZ_ENV}"
KV_NAME="kv-${PREFIX}"
KC_APP="ca-${PREFIX}-keycloak"

log() { printf '==> %s\n' "$*" >&2; }

log "resolving keycloak FQDN ($KC_APP in $AZ_RG)"
KC_FQDN="$(az containerapp show -g "$AZ_RG" -n "$KC_APP" \
  --query 'properties.configuration.ingress.fqdn' -o tsv)"
[ -n "$KC_FQDN" ] || { echo "could not resolve keycloak fqdn" >&2; exit 1; }

log "reading keycloak-admin-password from Key Vault ($KV_NAME)"
KC_ADMIN_PASS="$(az keyvault secret show \
  --vault-name "$KV_NAME" --name keycloak-admin-password \
  --query value -o tsv)"
[ -n "$KC_ADMIN_PASS" ] || { echo "could not read keycloak-admin-password" >&2; exit 1; }

log "minting master-realm token via temp-admin client_credentials"
MASTER_TOK="$(curl -sf "https://${KC_FQDN}/realms/master/protocol/openid-connect/token" \
  -d "client_id=temp-admin" \
  -d "client_secret=${KC_ADMIN_PASS}" \
  -d "grant_type=client_credentials" \
  | jq -r '.access_token // empty')"
[ -n "$MASTER_TOK" ] || { echo "master-realm token mint failed" >&2; exit 1; }

REALM_URL="https://${KC_FQDN}/admin/realms/bowrain-admin"

log "ensuring agent-bot service-account client exists in bowrain-admin realm"
CLIENT_UUID="$(curl -sf "$REALM_URL/clients?clientId=agent-bot" \
  -H "Authorization: Bearer $MASTER_TOK" | jq -r '.[0].id // empty')"
if [ -z "$CLIENT_UUID" ]; then
  log "  ↳ creating agent-bot client"
  curl -sfS -X POST "$REALM_URL/clients" \
    -H "Authorization: Bearer $MASTER_TOK" \
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
    }' >/dev/null
  CLIENT_UUID="$(curl -sf "$REALM_URL/clients?clientId=agent-bot" \
    -H "Authorization: Bearer $MASTER_TOK" | jq -r '.[0].id // empty')"
  [ -n "$CLIENT_UUID" ] || { echo "agent-bot client UUID lookup failed" >&2; exit 1; }
fi

log "reading agent-bot client secret"
AGENT_SECRET="$(curl -sf "$REALM_URL/clients/$CLIENT_UUID/client-secret" \
  -H "Authorization: Bearer $MASTER_TOK" | jq -r '.value // empty')"
[ -n "$AGENT_SECRET" ] || { echo "agent-bot client secret read failed" >&2; exit 1; }

log "minting bowrain-admin-realm access token"
ADMIN_TOKEN="$(curl -sfS \
  "https://${KC_FQDN}/realms/bowrain-admin/protocol/openid-connect/token" \
  -d "grant_type=client_credentials" \
  -d "client_id=agent-bot" \
  -d "client_secret=${AGENT_SECRET}" \
  | jq -r '.access_token // empty')"
[ -n "$ADMIN_TOKEN" ] || { echo "bowrain-admin token mint failed" >&2; exit 1; }

# Stdout: the token, nothing else.
printf '%s' "$ADMIN_TOKEN"
