#!/bin/bash
# Stop the Docker Compose stack and clean up volumes.
set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"

echo "Stopping e2e stack..."
docker compose -f "$ROOT_DIR/compose.yaml" down -v
echo "Done."
