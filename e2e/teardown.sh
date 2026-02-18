#!/bin/bash
# Stop the local bowrain-server and Docker dependencies.
set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"

echo "Stopping bowrain-server..."
if [ -f "$ROOT_DIR/.bowrain-server.pid" ]; then
  kill "$(cat "$ROOT_DIR/.bowrain-server.pid")" 2>/dev/null || true
  rm -f "$ROOT_DIR/.bowrain-server.pid"
fi

echo "Stopping dependencies..."
docker compose -f "$ROOT_DIR/compose.yaml" down -v
echo "Done."
