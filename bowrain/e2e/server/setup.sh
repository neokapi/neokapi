#!/bin/bash
# Start the E2E stack using pre-built Docker images.
set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"

echo "Starting E2E stack (pulling pre-built images)..."
docker compose -f "$SCRIPT_DIR/compose.yaml" up -d --wait

echo "E2E stack is ready."
echo "  bowrain-server: http://localhost:8080"
echo "  keycloak:       http://localhost:8180"
echo "  mailpit:        http://localhost:8025"
