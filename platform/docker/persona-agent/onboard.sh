#!/bin/sh
# Onboard persona agents for a Bowrain workspace.
#
# Per AD-032, persona agents are full users with workspace membership and
# API tokens (bwt_*). They authenticate identically to human users and
# receive project-level permissions via role templates and language scopes.
#
# This script:
#   1. Creates Keycloak users for each agent (no password — token-only auth)
#   2. Ensures the Bowrain workspace exists
#   3. Creates API tokens for each agent (stored in Key Vault)
#
# Project membership with role templates and language scopes should be
# configured via the Bowrain UI after onboarding (or via API once PR #117
# adds the project members endpoint).
#
# Usage:
#   BOWRAIN_URL=https://ca-bowrain-dev-api.example.com \
#   KC_URL=https://auth.dev.bowrain.cloud \
#   KC_ADMIN_PASS=<password> \
#   ADMIN_TOKEN=<bowrain-admin-jwt> \
#   KEY_VAULT=kv-bowrain-dev \
#   ./onboard.sh <workspace-slug>
#
# Agents are defined in the AGENTS variable below. Edit to add/remove.

set -eu

SLUG="${1:?Usage: onboard.sh <workspace-slug>}"

BOWRAIN_URL="${BOWRAIN_URL:?Set BOWRAIN_URL}"
KC_URL="${KC_URL:?Set KC_URL}"
KC_ADMIN_PASS="${KC_ADMIN_PASS:?Set KC_ADMIN_PASS}"
ADMIN_TOKEN="${ADMIN_TOKEN:?Set ADMIN_TOKEN (Bowrain admin JWT)}"
KEY_VAULT="${KEY_VAULT:-}"

KC_REALM="bowrain"

# ── Agent roster ─────────────────────────────────────────────────────
# Format: name:First:Last:role
# Role is informational here — actual permissions come from project membership.
AGENTS="
coordinator:Fleet:Coordinator:coordinator
maria:Maria:Dubois:translator
katrin:Katrin:Weber:translator
yuki:Yuki:Tanaka:translator
alex:Alex:Chen:reviewer
"

# ── Keycloak admin token ──────────────────────────────────────────────
kc_token() {
  # Try service account first (Keycloak 26+), fall back to password grant.
  TOKEN=$(curl -sf "${KC_URL}/realms/master/protocol/openid-connect/token" \
    -d "client_id=temp-admin" \
    -d "client_secret=${KC_ADMIN_PASS}" \
    -d "grant_type=client_credentials" 2>/dev/null \
    | sed 's/.*"access_token":"\([^"]*\)".*/\1/' || true)

  if [ -z "$TOKEN" ]; then
    TOKEN=$(curl -sf "${KC_URL}/realms/master/protocol/openid-connect/token" \
      -d "client_id=admin-cli" \
      -d "username=admin" \
      -d "password=${KC_ADMIN_PASS}" \
      -d "grant_type=password" \
      | sed 's/.*"access_token":"\([^"]*\)".*/\1/')
  fi

  echo "$TOKEN"
}

# ── Create Keycloak user (idempotent) ─────────────────────────────────
create_kc_user() {
  local email="$1"
  local first="$2"
  local last="$3"
  local kc_tok="$4"

  # Check if user exists.
  existing=$(curl -sf "${KC_URL}/admin/realms/${KC_REALM}/users?email=${email}&exact=true" \
    -H "Authorization: Bearer ${kc_tok}")

  if echo "$existing" | grep -q '"id"'; then
    echo "  User ${email} already exists" >&2
    echo "$existing" | sed 's/.*"id":"\([^"]*\)".*/\1/'
    return
  fi

  # Create user (no password — agents use API tokens, not password login).
  resp=$(curl -sf -w "\n%{http_code}" "${KC_URL}/admin/realms/${KC_REALM}/users" \
    -H "Authorization: Bearer ${kc_tok}" \
    -H "Content-Type: application/json" \
    -d "{
      \"username\": \"${email}\",
      \"email\": \"${email}\",
      \"firstName\": \"${first}\",
      \"lastName\": \"${last}\",
      \"enabled\": true,
      \"emailVerified\": true,
      \"requiredActions\": []
    }")

  status=$(echo "$resp" | tail -1)
  if [ "$status" = "201" ] || [ "$status" = "409" ]; then
    echo "  Created user ${email}" >&2
  else
    echo "  ERROR creating user ${email}: status=${status}" >&2
    echo "$resp" >&2
  fi

  # Re-fetch to get the ID.
  curl -sf "${KC_URL}/admin/realms/${KC_REALM}/users?email=${email}&exact=true" \
    -H "Authorization: Bearer ${kc_tok}" \
    | sed 's/.*"id":"\([^"]*\)".*/\1/'
}

# ── Create Bowrain workspace (idempotent) ─────────────────────────────
ensure_workspace() {
  local name="$1"
  local slug="$2"

  resp=$(curl -sf -w "\n%{http_code}" "${BOWRAIN_URL}/api/v1/workspaces" \
    -H "Authorization: Bearer ${ADMIN_TOKEN}" \
    -H "Content-Type: application/json" \
    -d "{\"name\": \"${name}\", \"slug\": \"${slug}\"}")

  status=$(echo "$resp" | tail -1)
  body=$(echo "$resp" | head -1)

  if [ "$status" = "201" ] || [ "$status" = "200" ]; then
    echo "  Created workspace: ${slug}" >&2
  elif [ "$status" = "409" ]; then
    echo "  Workspace ${slug} already exists" >&2
  else
    echo "  ERROR creating workspace: status=${status}" >&2
    echo "$body" >&2
    return 1
  fi
}

# ── Create API token for agent ────────────────────────────────────────
create_agent_token() {
  local ws_slug="$1"
  local agent_name="$2"

  resp=$(curl -sf "${BOWRAIN_URL}/api/v1/ws/${ws_slug}/tokens" \
    -H "Authorization: Bearer ${ADMIN_TOKEN}" \
    -H "Content-Type: application/json" \
    -d "{\"name\": \"agent-${agent_name}\"}")

  echo "$resp" | sed 's/.*"token":"\([^"]*\)".*/\1/'
}

# ── Main ──────────────────────────────────────────────────────────────

echo "=== Onboarding persona agents for: ${SLUG} ===" >&2

# Get Keycloak admin token.
echo "Authenticating with Keycloak..." >&2
KC_TOK=$(kc_token)
if [ -z "$KC_TOK" ]; then
  echo "ERROR: Failed to get Keycloak admin token" >&2
  exit 1
fi

# Create Keycloak users.
echo "" >&2
echo "Creating Keycloak users..." >&2

for agent_line in $AGENTS; do
  name=$(echo "$agent_line" | cut -d: -f1)
  first=$(echo "$agent_line" | cut -d: -f2)
  last=$(echo "$agent_line" | cut -d: -f3)
  email="agent-${name}@bowrain.cloud"
  create_kc_user "$email" "$first" "$last" "$KC_TOK"
done

# Ensure workspace exists.
echo "" >&2
echo "Ensuring workspace..." >&2
WS_NAME=$(echo "$SLUG" | sed 's/-l10n$//' | sed 's/-/ /g')
ensure_workspace "$WS_NAME" "$SLUG"

# Generate API tokens.
echo "" >&2
echo "Generating API tokens..." >&2
echo "# Agent tokens for workspace: ${SLUG}"
echo "# Generated: $(date -u +%Y-%m-%dT%H:%M:%SZ)"

for agent_line in $AGENTS; do
  name=$(echo "$agent_line" | cut -d: -f1)
  token=$(create_agent_token "$SLUG" "$name")
  echo "  Token for ${name}: ${token:0:12}..." >&2

  # Store in Key Vault if configured.
  if [ -n "$KEY_VAULT" ]; then
    az keyvault secret set \
      --vault-name "$KEY_VAULT" \
      --name "agent-token-${name}" \
      --value "$token" \
      --output none 2>&1
    echo "  Stored in Key Vault: agent-token-${name}" >&2
  else
    varname=$(echo "$name" | tr '-' '_' | tr '[:lower:]' '[:upper:]')
    echo "${varname}_TOKEN=${token}"
  fi
done

echo "" >&2
echo "=== Onboarding complete ===" >&2
echo "" >&2
echo "Next steps:" >&2
echo "  1. Add agents as project members in the Bowrain UI:" >&2
echo "     - Translators: 'translator' role template + language scope" >&2
echo "     - Reviewer: 'reviewer' role template" >&2
echo "     - Coordinator: 'project-admin' role template" >&2
echo "  2. Restart agent containers to pick up the new tokens" >&2
