#!/usr/bin/env bash
#
# Provision Azure OpenAI resources for gokapi AI translation.
#
# Prerequisites:
#   - Azure CLI (az) installed and logged in: az login
#   - Subscription with Azure OpenAI access enabled
#
# Usage:
#   bash scripts/setup-azure-openai.sh [resource-name] [location] [deployment-name]
#
# Defaults:
#   resource-name:   gokapi-openai
#   location:        eastus
#   deployment-name: gpt-4o

set -euo pipefail

RESOURCE_GROUP="gokapi-ai-rg"
RESOURCE_NAME="${1:-gokapi-openai}"
LOCATION="${2:-eastus}"
DEPLOYMENT_NAME="${3:-gpt-4o}"
MODEL_NAME="gpt-4o"
MODEL_VERSION="2024-11-20"

echo "==> Creating resource group: ${RESOURCE_GROUP}"
az group create \
  --name "${RESOURCE_GROUP}" \
  --location "${LOCATION}" \
  --output none

echo "==> Creating Azure OpenAI resource: ${RESOURCE_NAME}"
az cognitiveservices account create \
  --name "${RESOURCE_NAME}" \
  --resource-group "${RESOURCE_GROUP}" \
  --kind "OpenAI" \
  --sku "S0" \
  --location "${LOCATION}" \
  --output none

echo "==> Deploying model: ${MODEL_NAME} as '${DEPLOYMENT_NAME}'"
az cognitiveservices account deployment create \
  --name "${RESOURCE_NAME}" \
  --resource-group "${RESOURCE_GROUP}" \
  --deployment-name "${DEPLOYMENT_NAME}" \
  --model-name "${MODEL_NAME}" \
  --model-version "${MODEL_VERSION}" \
  --model-format "OpenAI" \
  --sku-capacity 10 \
  --sku-name "Standard" \
  --output none

echo "==> Retrieving credentials..."
ENDPOINT=$(az cognitiveservices account show \
  --name "${RESOURCE_NAME}" \
  --resource-group "${RESOURCE_GROUP}" \
  --query "properties.endpoint" \
  --output tsv)

API_KEY=$(az cognitiveservices account keys list \
  --name "${RESOURCE_NAME}" \
  --resource-group "${RESOURCE_GROUP}" \
  --query "key1" \
  --output tsv)

echo ""
echo "Azure OpenAI resource provisioned successfully."
echo ""
echo "  Endpoint:    ${ENDPOINT}"
echo "  API Key:     ${API_KEY}"
echo "  Deployment:  ${DEPLOYMENT_NAME}"
echo ""
echo "Use these values when adding an Azure OpenAI provider in gokapi settings:"
echo "  - Base URL:  ${ENDPOINT}"
echo "  - Model:     ${DEPLOYMENT_NAME}"
echo "  - API Key:   ${API_KEY}"
