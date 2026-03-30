#!/usr/bin/env bash
# Run cloud e2e tests against a live Bowrain server.
#
# Usage:
#   ./run.sh                                              # Local Docker stack
#   BOWRAIN_URL=https://dev.bowrain.cloud ./run.sh        # Dev environment
#   BOWRAIN_URL=https://dev.bowrain.cloud \
#     KEYCLOAK_ADMIN_PASSWORD=$(az keyvault secret show \
#       --vault-name kv-bowrain-dev \
#       --name keycloak-admin-password \
#       --query value -o tsv) ./run.sh                    # Dev with Key Vault
#
# Environment variables:
#   BOWRAIN_URL             — Server URL (default: http://localhost:8080)
#   KEYCLOAK_URL            — Keycloak URL (default: derived from BOWRAIN_URL)
#   KEYCLOAK_ADMIN_PASSWORD — Keycloak admin password (default: admin)
#   E2E_USER_EMAIL          — Override test user email

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$SCRIPT_DIR"

# Install dependencies if needed.
if [ ! -d node_modules ]; then
  echo "Installing dependencies..."
  vp install
fi

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
