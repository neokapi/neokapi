#!/bin/bash
# Stop the e2e Docker Compose stack after VHS tape recordings.
set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
E2E_DIR="$SCRIPT_DIR/../../e2e"

# Clean up token file.
rm -f "$SCRIPT_DIR/.server-token"

echo "Stopping e2e stack..."
bash "$E2E_DIR/teardown.sh"
