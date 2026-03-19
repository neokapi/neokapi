#!/usr/bin/env bash
#
# setup-workspace.sh — Create a Bowrain workspace and invite agents.
#
# Usage: ./scripts/setup-workspace.sh <workspace-dir>
# Example: ./scripts/setup-workspace.sh workspaces/excalidraw
#
# This script:
# 1. Reads workspace.yaml to get project details
# 2. Creates the Bowrain workspace via bowrain CLI
# 3. Invites each agent listed in workspace.yaml
#
# Prerequisites:
# - bowrain CLI installed and authenticated
# - Keycloak users created (run setup-keycloak-users.sh first)

set -euo pipefail

if [ $# -lt 1 ]; then
  echo "Usage: $0 <workspace-dir>"
  echo "Example: $0 workspaces/excalidraw"
  exit 1
fi

WS_DIR="$1"
WS_YAML="${WS_DIR}/workspace.yaml"

if [ ! -f "$WS_YAML" ]; then
  echo "ERROR: ${WS_YAML} not found" >&2
  exit 1
fi

# Parse workspace.yaml (simple grep-based parsing)
WS_NAME=$(grep '^name:' "$WS_YAML" | head -1 | sed 's/^name: *//')
WS_SLUG=$(grep '^slug:' "$WS_YAML" | head -1 | sed 's/^slug: *//')
WS_DESC=$(grep '^description:' "$WS_YAML" | head -1 | sed 's/^description: *"//' | sed 's/"$//')
SOURCE_LANG=$(grep '^source_language:' "$WS_YAML" | head -1 | sed 's/^source_language: *//')

echo "Setting up workspace: ${WS_NAME} (${WS_SLUG})"
echo "  Description: ${WS_DESC}"
echo "  Source language: ${SOURCE_LANG}"
echo

# Step 1: Create the workspace
echo "Creating Bowrain workspace..."
bowrain workspace create \
  --name "$WS_NAME" \
  --slug "$WS_SLUG" \
  --source-language "$SOURCE_LANG" \
  2>/dev/null || echo "  (workspace may already exist)"

# Step 2: Invite agents
echo
echo "Inviting agents..."

AGENTS_DIR="${WS_DIR}/agents"
if [ ! -d "$AGENTS_DIR" ]; then
  echo "  No agents/ directory found."
  exit 0
fi

for agent_dir in "${AGENTS_DIR}"/*/; do
  agent_name=$(basename "$agent_dir")
  if [ "$agent_name" = "_template" ]; then
    continue
  fi

  email="${agent_name}@${WS_SLUG}.bowrain.test"
  echo "  Inviting ${agent_name} (${email})..."
  bowrain workspace invite \
    --workspace "$WS_SLUG" \
    --email "$email" \
    2>/dev/null || echo "    (may already be invited)"
done

echo
echo "Done. Workspace '${WS_SLUG}' is ready."
