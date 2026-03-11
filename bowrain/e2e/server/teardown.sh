#!/bin/bash
# Stop the E2E Docker stack and remove volumes.
set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"

echo "Stopping E2E stack..."
docker compose -f "$SCRIPT_DIR/compose.yaml" down -v
echo "Done."
