#!/bin/bash
set -e

IMPORT_DIR=/opt/keycloak/data/import

if [ -f "$IMPORT_DIR/realm.json" ] && [ -n "$OIDC_CLIENT_SECRET" ]; then
  sed -i "s|bowrain-secret|$OIDC_CLIENT_SECRET|g" "$IMPORT_DIR/realm.json"
fi

exec /opt/keycloak/bin/kc.sh "$@"
