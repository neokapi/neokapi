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
# `make test-e2e-kapi` owns the tag set and the i18n-catalogs prerequisite, so
# this wrapper defers to it rather than keeping a second spelling that can drift
# out of it. Both tags must ride one comma-joined flag: `go` does not union
# repeated -tags, the last occurrence wins, and a build that quietly loses fts5
# fails only when a query reaches FTS5.
make -C "$ROOT_DIR" test-e2e-kapi

echo ""
echo "All kapi CLI e2e tests passed!"
