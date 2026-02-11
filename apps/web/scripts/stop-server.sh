#!/usr/bin/env bash
# Stop the Docker Compose stack and clean up volumes.
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/../../.." && pwd)"
COMPOSE_FILE="$REPO_ROOT/e2e/docker-compose.yml"

echo "Stopping gokapi-server Docker stack..."
docker compose -f "$COMPOSE_FILE" down -v

echo "Stack stopped and volumes cleaned."
