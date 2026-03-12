#!/usr/bin/env bash
# Start dependencies and bowrain-server locally for E2E tests.
# Usage: ./start-server.sh
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/../../../.." && pwd)"

echo "Starting dependencies (Keycloak + Mailpit)..."
docker compose -f "$REPO_ROOT/compose.yaml" up -d --wait

echo "Building bowrain-server..."
cd "$REPO_ROOT" && make build-server

echo "Starting bowrain-server..."
BOWRAIN_JWT_SECRET=dev-secret-change-in-production \
BOWRAIN_OIDC_ISSUER_URL=http://localhost:8180/realms/bowrain \
BOWRAIN_OIDC_CLIENT_ID=bowrain \
BOWRAIN_OIDC_CLIENT_SECRET=bowrain-secret \
BOWRAIN_SMTP_HOST=localhost:1025 \
BOWRAIN_SMTP_FROM=noreply@bowrain.cloud \
BOWRAIN_STORE="$REPO_ROOT/bowrain-e2e.db" \
BOWRAIN_GRPC_PORT=9080 \
"$REPO_ROOT/bin/bowrain-server" &
echo $! > "$REPO_ROOT/.bowrain-server.pid"

echo "Waiting for server to be healthy..."
for i in $(seq 1 60); do
  if curl -sf http://localhost:8080/api/v1/health > /dev/null 2>&1; then
    echo "Server is healthy!"
    exit 0
  fi
  sleep 1
done

echo "ERROR: Server did not become healthy within 60 seconds"
exit 1
