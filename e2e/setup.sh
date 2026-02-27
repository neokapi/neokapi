#!/bin/bash
# Start dependencies and the local bowrain-server for e2e tests.
set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"

echo "Starting dependencies (Keycloak + Mailpit)..."
docker compose -f "$ROOT_DIR/compose.yaml" up -d --wait

echo "Building bowrain-server..."
cd "$ROOT_DIR" && make build-server

echo "Starting bowrain-server..."
BOWRAIN_JWT_SECRET=dev-secret-change-in-production \
BOWRAIN_OIDC_ISSUER_URL=http://localhost:8180/realms/bowrain \
BOWRAIN_OIDC_CLIENT_ID=bowrain \
BOWRAIN_OIDC_CLIENT_SECRET=bowrain-secret \
BOWRAIN_SMTP_HOST=localhost:1025 \
BOWRAIN_SMTP_FROM=noreply@bowrain.cloud \
BOWRAIN_STORE="$ROOT_DIR/bowrain-e2e.db" \
BOWRAIN_GRPC_PORT=9080 \
BOWRAIN_WEB_UI_DIR="$ROOT_DIR/bowrain/apps/web/dist" \
"$ROOT_DIR/bin/bowrain-server" &
echo $! > "$ROOT_DIR/.bowrain-server.pid"

echo "Waiting for server health..."
for i in $(seq 1 30); do
  if curl -sf http://localhost:8080/api/v1/health > /dev/null 2>&1; then
    echo "bowrain-server is healthy."
    exit 0
  fi
  sleep 2
done

echo "ERROR: Server did not become healthy within 60 seconds"
exit 1
