#!/usr/bin/env bash
# Run the cloud e2e suite against a disposable Bowrain stack.
#
# The suite deletes every project in the workspace it uses, so it runs only
# against a server that reports itself disposable on /api/v1/health
# (bowrain/e2e/shared/disposable-stack.ts). CI's stack,
# bowrain/e2e/server/compose.yaml, is one; so is a local stack started with
# `make -C bowrain stack-up-disposable`.
#
# Usage:
#   ./run.sh                                  # the stack at http://localhost:8080
#   BOWRAIN_URL=http://host:8080 ./run.sh     # another disposable stack
#
# Environment variables:
#   BOWRAIN_URL             Server URL (default: http://localhost:8080)
#   KEYCLOAK_URL            Keycloak URL (default: derived from BOWRAIN_URL)
#   KEYCLOAK_ADMIN_PASSWORD Keycloak admin password (default: admin)
#   E2E_USER_EMAIL          Override test user email

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$SCRIPT_DIR"

# Install dependencies if needed.
if [ ! -d node_modules ]; then
  echo "Installing dependencies..."
  vp install
fi

# The specs read the shared API client's types; a field one of them reads and
# the client does not declare fails here, before a browser starts.
vp run typecheck

# Ensure Playwright browsers are installed.
vpx playwright install --with-deps chromium 2>/dev/null || vpx playwright install chromium

echo ""
echo "════════════════════════════════════════════════════════════════"
echo "Bowrain Cloud E2E Tests"
echo "════════════════════════════════════════════════════════════════"
echo "  Server:   ${BOWRAIN_URL:-http://localhost:8080}"
echo "  Keycloak: ${KEYCLOAK_URL:-<derived from BOWRAIN_URL>}"
echo "════════════════════════════════════════════════════════════════"
echo ""

vpx playwright test "$@"
