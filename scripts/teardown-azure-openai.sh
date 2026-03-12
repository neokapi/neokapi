#!/usr/bin/env bash
#
# Tear down Azure OpenAI resources created by setup-azure-openai.sh.
#
# This deletes the entire resource group (neokapi-ai-rg) and all resources
# within it, including the OpenAI resource and its deployments.
#
# Prerequisites:
#   - Azure CLI (az) installed and logged in: az login
#
# Usage:
#   bash scripts/teardown-azure-openai.sh

set -euo pipefail

RESOURCE_GROUP="neokapi-ai-rg"

echo "==> Checking if resource group '${RESOURCE_GROUP}' exists..."
if ! az group show --name "${RESOURCE_GROUP}" --output none 2>/dev/null; then
  echo "Resource group '${RESOURCE_GROUP}' does not exist. Nothing to tear down."
  exit 0
fi

echo "==> Deleting resource group: ${RESOURCE_GROUP}"
echo "    This will remove all resources within the group."
az group delete \
  --name "${RESOURCE_GROUP}" \
  --yes \
  --no-wait \
  --output none

echo ""
echo "Resource group '${RESOURCE_GROUP}' deletion initiated."
echo "Azure will complete the deletion in the background."
