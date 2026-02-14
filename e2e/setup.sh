#!/bin/bash
# Start the e2e Docker Compose stack and wait for services to be healthy.
set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$SCRIPT_DIR"

echo "Starting e2e stack..."
docker compose up -d --build --wait --wait-timeout 120

echo "Verifying health..."
curl -sf http://localhost:8080/api/v1/health > /dev/null
echo "bowrain-server is healthy."
