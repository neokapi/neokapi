#!/usr/bin/env bash
#
# setup-keycloak-users.sh — Create Keycloak users for the 7 Bowrain agent personas.
#
# Idempotent: skips users that already exist.
#
# Environment variables (all optional, sensible defaults for local dev):
#   KEYCLOAK_URL              Base URL (default: http://localhost:8180)
#   KEYCLOAK_ADMIN            Admin username (default: admin)
#   KEYCLOAK_ADMIN_PASSWORD   Admin password (default: admin)
#   KEYCLOAK_REALM            Target realm (default: master)
#   DEFAULT_PASSWORD           Password set for every new user (default: changeme)

set -euo pipefail

KEYCLOAK_URL="${KEYCLOAK_URL:-http://localhost:8180}"
KEYCLOAK_ADMIN="${KEYCLOAK_ADMIN:-admin}"
KEYCLOAK_ADMIN_PASSWORD="${KEYCLOAK_ADMIN_PASSWORD:-admin}"
REALM="${KEYCLOAK_REALM:-master}"
DEFAULT_PASSWORD="${DEFAULT_PASSWORD:-changeme}"

# --- Obtain admin access token ---
get_token() {
  local resp
  resp=$(curl -sf -X POST "${KEYCLOAK_URL}/realms/master/protocol/openid-connect/token" \
    -H "Content-Type: application/x-www-form-urlencoded" \
    -d "username=${KEYCLOAK_ADMIN}" \
    -d "password=${KEYCLOAK_ADMIN_PASSWORD}" \
    -d "grant_type=password" \
    -d "client_id=admin-cli")
  echo "$resp" | jq -r '.access_token'
}

echo "Authenticating with Keycloak at ${KEYCLOAK_URL} ..."
TOKEN=$(get_token)
if [ -z "$TOKEN" ] || [ "$TOKEN" = "null" ]; then
  echo "ERROR: Failed to obtain admin token. Is Keycloak running?" >&2
  exit 1
fi
echo "Authenticated."
echo

# --- User definitions ---
# Format: username|email|firstName|lastName
USERS=(
  "alex.chen|alex.chen@bowrain.test|Alex|Chen"
  "jeanpierre.dubois|jeanpierre.dubois@bowrain.test|Jean-Pierre|Dubois"
  "katrin.weber|katrin.weber@bowrain.test|Katrin|Weber"
  "yuki.tanaka|yuki.tanaka@bowrain.test|Yuki|Tanaka"
  "maria.santos|maria.santos@bowrain.test|Maria|Santos"
  "lisa.chen|lisa.chen@bowrain.test|Lisa|Chen"
  "taylor.kim|taylor.kim@bowrain.test|Taylor|Kim"
)

BASE="${KEYCLOAK_URL}/admin/realms/${REALM}/users"

create_user() {
  local username="$1" email="$2" first="$3" last="$4"

  # Check if user already exists
  local existing
  existing=$(curl -sf -H "Authorization: Bearer ${TOKEN}" \
    "${BASE}?username=${username}&exact=true")

  local count
  count=$(echo "$existing" | jq 'length')

  if [ "$count" -gt 0 ]; then
    local uid
    uid=$(echo "$existing" | jq -r '.[0].id')
    echo "SKIP  ${username} (already exists, id=${uid})"
    return
  fi

  # Create user
  local payload
  payload=$(jq -n \
    --arg u "$username" \
    --arg e "$email" \
    --arg f "$first" \
    --arg l "$last" \
    --arg p "$DEFAULT_PASSWORD" \
    '{
      username: $u,
      email: $e,
      firstName: $f,
      lastName: $l,
      enabled: true,
      emailVerified: true,
      credentials: [{
        type: "password",
        value: $p,
        temporary: false
      }]
    }')

  local http_code
  http_code=$(curl -s -o /dev/null -w "%{http_code}" \
    -X POST "${BASE}" \
    -H "Authorization: Bearer ${TOKEN}" \
    -H "Content-Type: application/json" \
    -d "$payload")

  if [ "$http_code" = "201" ]; then
    # Fetch the created user to get the ID
    local created
    created=$(curl -sf -H "Authorization: Bearer ${TOKEN}" \
      "${BASE}?username=${username}&exact=true")
    local uid
    uid=$(echo "$created" | jq -r '.[0].id')
    echo "OK    ${username} (created, id=${uid})"
  else
    echo "ERROR ${username} (HTTP ${http_code})" >&2
  fi
}

echo "Creating users in realm '${REALM}' ..."
echo

for entry in "${USERS[@]}"; do
  IFS='|' read -r username email first last <<< "$entry"
  create_user "$username" "$email" "$first" "$last"
done

echo
echo "Done."
