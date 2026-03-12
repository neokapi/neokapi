#!/bin/bash
# Start the full bowrain stack for Desktop recordings using pre-built Docker images.
# No need to build anything — all images come from ghcr.io.
#
# Usage:
#   bash scripts/start-recording-server.sh
#   # ... run recordings ...
#   docker compose -f e2e/compose.yaml down -v

set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$SCRIPT_DIR/../../../../.."
E2E_COMPOSE="$REPO_ROOT/e2e/compose.yaml"

echo "Starting full stack from pre-built Docker images..."
docker compose -f "$E2E_COMPOSE" up -d --wait

echo ""
echo "Waiting for server health..."
for i in $(seq 1 30); do
  if curl -sf http://localhost:8080/api/v1/health > /dev/null 2>&1; then
    echo "Server is healthy."
    break
  fi
  if [ "$i" -eq 30 ]; then
    echo "Server failed to become healthy after 60s."
    exit 1
  fi
  sleep 2
done

echo ""
echo "Recording server ready at http://localhost:8080"
echo "Stop with: docker compose -f $E2E_COMPOSE down -v"
