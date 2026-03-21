#!/bin/sh
# Onboard a workspace for agentic testing.
#
# This script provisions Keycloak users, creates the Bowrain workspace,
# and generates API tokens for each agent. Run from a machine with
# access to both the Keycloak admin API and the Bowrain API.
#
# Usage:
#   BOWRAIN_URL=https://api.dev.bowrain.cloud \
#   KC_URL=https://auth.dev.bowrain.cloud \
#   KC_ADMIN_PASS=<password> \
#   ADMIN_TOKEN=<bowrain-admin-jwt> \
#   ./onboard.sh <workspace-slug> <plan.yaml>
#
# Outputs a .env file with agent tokens to stdout.

set -eu

SLUG="${1:?Usage: onboard.sh <workspace-slug> <plan.yaml>}"
PLAN="${2:?Usage: onboard.sh <workspace-slug> <plan.yaml>}"

BOWRAIN_URL="${BOWRAIN_URL:?Set BOWRAIN_URL}"
KC_URL="${KC_URL:?Set KC_URL}"
KC_ADMIN_PASS="${KC_ADMIN_PASS:?Set KC_ADMIN_PASS}"
ADMIN_TOKEN="${ADMIN_TOKEN:?Set ADMIN_TOKEN (Bowrain admin JWT)}"

KC_REALM="bowrain"

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

# ── Create Bowrain workspace ──────────────────────────────────────────
create_workspace() {
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
    echo "$body" | sed 's/.*"id":"\([^"]*\)".*/\1/'
  elif [ "$status" = "409" ]; then
    echo "  Workspace ${slug} already exists" >&2
    # Fetch existing.
    curl -sf "${BOWRAIN_URL}/api/v1/workspaces/${slug}" \
      -H "Authorization: Bearer ${ADMIN_TOKEN}" \
      | sed 's/.*"id":"\([^"]*\)".*/\1/'
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

  resp=$(curl -sf "${BOWRAIN_URL}/api/v1/workspaces/${ws_slug}/tokens" \
    -H "Authorization: Bearer ${ADMIN_TOKEN}" \
    -H "Content-Type: application/json" \
    -d "{\"name\": \"agent-${agent_name}\"}")

  echo "$resp" | sed 's/.*"token":"\([^"]*\)".*/\1/'
}

# ── Main ──────────────────────────────────────────────────────────────

echo "=== Onboarding workspace: ${SLUG} ===" >&2

# Get Keycloak admin token.
echo "Authenticating with Keycloak..." >&2
KC_TOK=$(kc_token)
if [ -z "$KC_TOK" ]; then
  echo "ERROR: Failed to get Keycloak admin token" >&2
  exit 1
fi

# Read agent team from plan.yaml (requires yq or simple grep).
# Expected format in plan.yaml:
#   agent_team:
#     developer: alex-developer
#     translator_fr-FR: sophie-translator
#     qa: thomas-qa
#     pm: mei-pm

echo "" >&2
echo "Creating Keycloak users for agents..." >&2

# Define agent personas and their display names.
# Format: persona_name:First:Last
AGENTS="
coordinator:Fleet:Coordinator
alex-developer:Alex:Chen
sophie-translator:Sophie:Martin
thomas-qa:Thomas:Mueller
mei-pm:Mei:Tanaka
maria-brand:Maria:Santos
"

for agent_line in $AGENTS; do
  name=$(echo "$agent_line" | cut -d: -f1)
  first=$(echo "$agent_line" | cut -d: -f2)
  last=$(echo "$agent_line" | cut -d: -f3)
  email="agent-${name}@bowrain.cloud"
  create_kc_user "$email" "$first" "$last" "$KC_TOK"
done

# Create workspace.
echo "" >&2
echo "Creating Bowrain workspace..." >&2
WS_NAME=$(echo "$SLUG" | sed 's/-l10n$//' | sed 's/-/ /g')
WS_ID=$(create_workspace "$WS_NAME" "$SLUG")

# Generate API tokens for each agent.
echo "" >&2
echo "Generating API tokens..." >&2
echo "# Agent tokens for workspace: ${SLUG}"
echo "# Generated: $(date -u +%Y-%m-%dT%H:%M:%SZ)"

for agent_line in $AGENTS; do
  name=$(echo "$agent_line" | cut -d: -f1)
  token=$(create_agent_token "$SLUG" "$name")
  varname=$(echo "$name" | tr '-' '_' | tr '[:lower:]' '[:upper:]')
  echo "${varname}_TOKEN=${token}"
  echo "  Token for ${name}: ${token:0:12}..." >&2
done

echo "" >&2
echo "=== Onboarding complete ===" >&2
echo "Workspace ID: ${WS_ID}" >&2
echo "Save the token output to a .env file for agent deployment." >&2
