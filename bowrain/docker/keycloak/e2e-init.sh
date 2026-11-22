#!/usr/bin/env bash
# Prepare the local "bowrain" realm for end-to-end runs (compose.e2e.yaml).
#
# The realm signs in with passkeys only, which an automated UI test cannot
# complete. For an e2e run this script points the realm's browser flow at
# Keycloak's built-in "browser" flow (username and password) and seeds a test
# user with a password. Run it only against a throwaway local stack: the
# default realm import stays passkey-only, and `make -C bowrain stack-down`
# followed by `stack-up` restores it.
#
# Environment: KEYCLOAK_URL (default http://keycloak:8080), KEYCLOAK_ADMIN,
# KEYCLOAK_ADMIN_PASSWORD, E2E_REALM (default bowrain), E2E_USERNAME,
# E2E_PASSWORD.
set -euo pipefail

kc=/opt/keycloak/bin/kcadm.sh
url="${KEYCLOAK_URL:-http://keycloak:8080}"
realm="${E2E_REALM:-bowrain}"
user="${E2E_USERNAME:?E2E_USERNAME is required}"
password="${E2E_PASSWORD:?E2E_PASSWORD is required}"

# Keycloak can report healthy a moment before the admin API answers.
for _ in $(seq 1 30); do
  if "$kc" config credentials --server "$url" --realm master \
    --user "${KEYCLOAK_ADMIN:-admin}" --password "${KEYCLOAK_ADMIN_PASSWORD:-admin}" >/dev/null 2>&1; then
    break
  fi
  sleep 2
done
"$kc" config credentials --server "$url" --realm master \
  --user "${KEYCLOAK_ADMIN:-admin}" --password "${KEYCLOAK_ADMIN_PASSWORD:-admin}"

"$kc" update "realms/$realm" -s browserFlow=browser
echo "keycloak-e2e: realm $realm signs in with a password (browser flow: browser)"

id="$("$kc" get users -r "$realm" -q "username=$user" -q exact=true --fields id --format csv --noquotes | head -1)"
if [ -z "$id" ]; then
  "$kc" create users -r "$realm" -s "username=$user" -s "email=$user" \
    -s emailVerified=true -s enabled=true -s firstName=E2E -s lastName=Reviewer
fi
"$kc" set-password -r "$realm" --username "$user" --new-password "$password"
# The realm gives every new user the passkey registration as a default required
# action, which would stop the sign-in at a passkey prompt. The test user has
# none.
id="$("$kc" get users -r "$realm" -q "username=$user" -q exact=true --fields id --format csv --noquotes | head -1)"
"$kc" update "users/$id" -r "$realm" -s 'requiredActions=[]'
echo "keycloak-e2e: test user $user is ready"
