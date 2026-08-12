#!/bin/bash
# Run the full Bowrain server e2e test lifecycle: setup → test → teardown.
set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/../../.." && pwd)"

cleanup() {
  echo ""
  echo "Tearing down e2e stack..."
  bash "$SCRIPT_DIR/teardown.sh" || true
}
trap cleanup EXIT

echo "============================================"
echo "Bowrain Server E2E Tests"
echo "============================================"
echo ""

# Check for Docker
if ! command -v docker &> /dev/null; then
  echo "Docker not found. Skipping e2e tests."
  exit 0
fi

if ! docker compose version &> /dev/null; then
  echo "Docker Compose not found. Skipping e2e tests."
  exit 0
fi

# Setup
bash "$SCRIPT_DIR/setup.sh"

echo ""
echo "Running e2e tests..."
# The e2e tests live in the bowrain module; run from there so the relative
# package path resolves (the old ./platform/e2e/server path predates the
# platform/ → bowrain/ rename).
#
# Both tags ride one comma-joined flag: `go` does not union repeated -tags, the
# last occurrence wins. The suite's test binary links go-sqlite3, so a build
# that loses fts5 still compiles and fails only when a query reaches FTS5.
cd "$ROOT_DIR/bowrain"
go test -tags "fts5,e2e" -count=1 -v ./e2e/server/

echo ""
echo "All Bowrain server e2e tests passed!"
