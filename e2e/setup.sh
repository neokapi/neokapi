#!/bin/bash
# Start the Docker Compose stack and wait for services to be healthy.
set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"

echo "Starting e2e stack..."
docker compose -f "$ROOT_DIR/compose.yaml" up -d --build --wait --wait-timeout 120

echo "Verifying health..."
curl -sf http://localhost:8080/api/v1/health > /dev/null
echo "bowrain-server is healthy."
