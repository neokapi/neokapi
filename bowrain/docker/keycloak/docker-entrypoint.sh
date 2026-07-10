#!/bin/bash
set -e

IMPORT_DIR=/opt/keycloak/data/import

# Substitute the realm smtpServer block from the KC_SMTP_* envs. Keycloak has
# no native KC_SMTP_* runtime options — realm SMTP only exists in the realm
# representation, and the seed files ship the local-dev Mailpit relay
# (host mailpit, port 1025, no auth). Applied to every realm file present.
#
# NOTE: `--import-realm` only imports realms that do not exist yet, so this
# takes effect on FIRST boot. Changing SMTP on an already-imported realm needs
#   kcadm.sh update realms/<realm> -s 'smtpServer={...}'
# instead (the import is skipped entirely for existing realms).
substitute_smtp() {
  local f="$1"
  if [ -n "$KC_SMTP_HOST" ]; then
    sed -i "s|\"host\": \"mailpit\"|\"host\": \"$KC_SMTP_HOST\"|g" "$f"
  fi
  if [ -n "$KC_SMTP_PORT" ]; then
    sed -i "s|\"port\": \"1025\"|\"port\": \"$KC_SMTP_PORT\"|g" "$f"
  fi
  if [ -n "$KC_SMTP_FROM" ]; then
    sed -i "s|\"from\": \"noreply@bowrain.cloud\"|\"from\": \"$KC_SMTP_FROM\"|g" "$f"
  fi
  if [ -n "$KC_SMTP_SSL" ]; then
    sed -i "s|\"ssl\": \"false\"|\"ssl\": \"$KC_SMTP_SSL\"|g" "$f"
  fi
  if [ -n "$KC_SMTP_STARTTLS" ]; then
    sed -i "s|\"starttls\": \"false\"|\"starttls\": \"$KC_SMTP_STARTTLS\"|g" "$f"
  fi
  # Authenticated relay (SES): flip auth on and inject the user/password keys
  # (the seed block carries none — auth is "false" for the Mailpit default).
  if [ -n "$KC_SMTP_USERNAME" ]; then
    sed -i "s|\"auth\": \"false\"|\"auth\": \"true\", \"user\": \"$KC_SMTP_USERNAME\", \"password\": \"$KC_SMTP_PASSWORD\"|g" "$f"
  fi
}

if [ -f "$IMPORT_DIR/realm.json" ]; then
  if [ -n "$OIDC_CLIENT_SECRET" ]; then
    sed -i "s|bowrain-secret|$OIDC_CLIENT_SECRET|g" "$IMPORT_DIR/realm.json"
  fi

  # bowrain-svc: the confidential service-account client (realm-management
  # manage-users, INSIDE the bowrain realm) that bowrain-server uses for
  # Keycloak Admin API write-through (email change). The bootstrap-admin pair
  # cannot serve this purpose — it lives in the master realm.
  if [ -n "$BOWRAIN_SVC_CLIENT_SECRET" ]; then
    sed -i "s|placeholder-bowrain-svc-secret|$BOWRAIN_SVC_CLIENT_SECRET|g" "$IMPORT_DIR/realm.json"
  fi

  # Apple Sign-In identity provider credentials.
  if [ -n "$APPLE_CLIENT_ID" ]; then
    sed -i "s|placeholder-apple-client-id|$APPLE_CLIENT_ID|g" "$IMPORT_DIR/realm.json"
  fi
  if [ -n "$APPLE_CLIENT_SECRET" ]; then
    sed -i "s|placeholder-apple-client-secret|$APPLE_CLIENT_SECRET|g" "$IMPORT_DIR/realm.json"
  fi
  if [ -n "$APPLE_TEAM_ID" ]; then
    sed -i "s|placeholder-apple-team-id|$APPLE_TEAM_ID|g" "$IMPORT_DIR/realm.json"
  fi
  if [ -n "$APPLE_KEY_ID" ]; then
    sed -i "s|placeholder-apple-key-id|$APPLE_KEY_ID|g" "$IMPORT_DIR/realm.json"
  fi

  substitute_smtp "$IMPORT_DIR/realm.json"
fi

# bowrain-admin realm (ctrl control-plane operators). The seed file ships a
# placeholder password for the admin@bowrain.cloud user: substitute it from
# BOWRAIN_ADMIN_INITIAL_PASSWORD when provided, otherwise replace it with a
# random throwaway so production never imports a guessable credential (set the
# real password afterwards via the bootstrap admin / kcadm or the realm's
# forgot-password flow once SMTP works).
if [ -f "$IMPORT_DIR/bowrain-admin-realm.json" ]; then
  admin_password="$BOWRAIN_ADMIN_INITIAL_PASSWORD"
  if [ -z "$admin_password" ]; then
    # cut (not a trailing head) so no pipe stage is killed early — a SIGPIPE
    # exit status would abort the script under `set -e`.
    admin_password="$(head -c 512 /dev/urandom | tr -dc 'A-Za-z0-9' | cut -c1-48)"
  fi
  sed -i "s|placeholder-admin-initial-password|$admin_password|g" "$IMPORT_DIR/bowrain-admin-realm.json"

  substitute_smtp "$IMPORT_DIR/bowrain-admin-realm.json"
fi

# Keycloak 26+ bootstrap admin service account for CI/CD configuration.
# Uses client credentials grant instead of the deprecated password grant.
EXTRA_ARGS=()
if [ -n "$KC_BOOTSTRAP_ADMIN_CLIENT_ID" ] && [ -n "$KC_BOOTSTRAP_ADMIN_CLIENT_SECRET" ]; then
  EXTRA_ARGS+=("--bootstrap-admin-client-id=$KC_BOOTSTRAP_ADMIN_CLIENT_ID" "--bootstrap-admin-client-secret=$KC_BOOTSTRAP_ADMIN_CLIENT_SECRET")
fi

exec /opt/keycloak/bin/kc.sh "$@" "${EXTRA_ARGS[@]}"
