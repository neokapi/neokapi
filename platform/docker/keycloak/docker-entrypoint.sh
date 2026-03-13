#!/bin/bash
set -e

IMPORT_DIR=/opt/keycloak/data/import

if [ -f "$IMPORT_DIR/realm.json" ]; then
  if [ -n "$OIDC_CLIENT_SECRET" ]; then
    sed -i "s|bowrain-secret|$OIDC_CLIENT_SECRET|g" "$IMPORT_DIR/realm.json"
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
fi

exec /opt/keycloak/bin/kc.sh "$@"
