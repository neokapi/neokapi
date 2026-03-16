#!/usr/bin/env bash
# Run cloud e2e tests against the dev environment.
# Fetches the Keycloak admin password from Azure Key Vault automatically.
#
# Prerequisites:
#   - az CLI (logged in: az login)
#
# Usage:
#   ./run-dev.sh                    # Run all tests
#   ./run-dev.sh --headed           # Run with visible browser
#   ./run-dev.sh --grep "Health"    # Run specific tests

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"

# Check Azure CLI.
if ! command -v az &>/dev/null; then
  echo "Error: az CLI not found." >&2
  exit 1
fi

if ! az account show &>/dev/null; then
  echo "Error: Not logged in to Azure. Run: az login" >&2
  exit 1
fi

# Fetch Keycloak admin password from Key Vault.
echo "Fetching Keycloak admin password from Key Vault..."
export KEYCLOAK_ADMIN_PASSWORD
KEYCLOAK_ADMIN_PASSWORD=$(az keyvault secret show \
  --vault-name kv-bowrain-dev \
  --name keycloak-admin-password \
  --query value -o tsv)

export BOWRAIN_URL="https://dev.bowrain.cloud"

# Admin API requires the internal Container App FQDN (not the public Front Door URL).
echo "Fetching Keycloak internal FQDN..."
export KEYCLOAK_ADMIN_URL
KEYCLOAK_ADMIN_URL="https://$(az containerapp show \
  --name ca-bowrain-dev-keycloak \
  --resource-group rg-bowrain-d-sdc \
  --query 'properties.configuration.ingress.fqdn' -o tsv)"

exec bash "$SCRIPT_DIR/run.sh" "$@"
