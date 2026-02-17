#!/usr/bin/env bash
# Start the Docker Compose stack for E2E tests.
# Usage: ./start-server.sh [--build]
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/../../.." && pwd)"
COMPOSE_FILE="$REPO_ROOT/e2e/docker-compose.yml"

echo "Starting bowrain-server Docker stack..."

BUILD_FLAG=""
if [[ "${1:-}" == "--build" ]]; then
  BUILD_FLAG="--build"
fi

docker compose -f "$COMPOSE_FILE" up -d $BUILD_FLAG

echo "Waiting for server to be healthy..."
for i in $(seq 1 60); do
  if curl -sf http://localhost:8080/api/v1/health > /dev/null 2>&1; then
    echo "Server is healthy!"
    exit 0
  fi
  sleep 1
done

echo "ERROR: Server did not become healthy within 60 seconds"
docker compose -f "$COMPOSE_FILE" logs
exit 1
