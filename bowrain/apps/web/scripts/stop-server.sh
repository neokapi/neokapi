#!/usr/bin/env bash
# Stop the local bowrain-server and Docker dependencies.
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/../../../.." && pwd)"

echo "Stopping bowrain-server..."
if [ -f "$REPO_ROOT/.bowrain-server.pid" ]; then
  kill "$(cat "$REPO_ROOT/.bowrain-server.pid")" 2>/dev/null || true
  rm -f "$REPO_ROOT/.bowrain-server.pid"
fi

echo "Stopping dependencies..."
docker compose -f "$REPO_ROOT/compose.yaml" down -v

echo "Stack stopped and volumes cleaned."
