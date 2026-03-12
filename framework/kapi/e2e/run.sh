#!/bin/bash
# Run kapi CLI end-to-end tests.
# No Docker or external services required — just builds kapi and runs tests.
#
# Usage:
#   bash kapi/e2e/run.sh          # from repo root
#   bash run.sh                    # from kapi/e2e/
set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/../.." && pwd)"

echo "============================================"
echo "Kapi CLI E2E Tests"
echo "============================================"
echo ""

cd "$ROOT_DIR"

echo "Running kapi CLI e2e tests..."
go test -tags=e2e -count=1 -v ./kapi/e2e/

echo ""
echo "All kapi CLI e2e tests passed!"
