#!/bin/bash
# Stop the e2e Docker Compose stack and clean up volumes.
set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$SCRIPT_DIR"

echo "Stopping e2e stack..."
docker compose down -v
echo "Done."
