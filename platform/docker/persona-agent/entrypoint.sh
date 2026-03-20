#!/bin/sh
# Agentic testing agent entrypoint.
#
# Environment variables:
#   AGENT_NAME          — agent identifier (e.g., "coordinator", "sophie-translator")
#   AGENT_ROLE          — persona role (e.g., "coordinator", "translator")
#   AGENT_TASK_MESSAGE  — task message for this session
#   WORKSPACE_SLUG      — target workspace (empty for coordinator)
#   FLEET_REPO          — git clone URL for the fleet state repo
#   FLEET_REPO_TOKEN    — PAT for fleet repo access (optional if SSH)
#   BRAVO_MCP_ENDPOINT  — Bowrain MCP server URL
#   BRAVO_AGENT_TOKEN   — JWT for MCP authentication
#   BOWRAIN_API_TOKEN   — long-lived API token (exchanged for JWT)
#   AGENTIC_MCP_ENDPOINT — Agentic Testing MCP URL (coordinator only)
#   REDIS_URL           — Redis URL for execution event publishing
#
# Flow:
# 1. Exchange API token for JWT (if needed)
# 2. Clone/pull fleet repo
# 3. Assemble SOUL.md from base persona + workspace override
# 4. Render config.toml from template
# 5. Run zeroclaw agent -m "<task>"
# 6. Push memory changes back to fleet repo

set -eu

FLEET_DIR="/tmp/fleet"
AGENT_DIR="/root/.zeroclaw"
ROLE="${AGENT_ROLE:-${AGENT_NAME}}"

echo "=== Agent Session Start ==="
echo "Agent: ${AGENT_NAME}"
echo "Role:  ${ROLE}"
echo "Task:  ${AGENT_TASK_MESSAGE:-<none>}"
echo "Workspace: ${WORKSPACE_SLUG:-<fleet-wide>}"
echo ""

# ── Token exchange ─────────────────────────────────────────────────────
# If a long-lived API token is provided, exchange it for a short-lived JWT.
if [ -n "${BOWRAIN_API_TOKEN:-}" ]; then
  EXCHANGE_URL="${BRAVO_MCP_ENDPOINT%/mcp/}/api/v1/auth/token/exchange"
  BRAVO_AGENT_TOKEN=$(wget -qO- \
    --header="Authorization: Bearer ${BOWRAIN_API_TOKEN}" \
    --post-data="" \
    "${EXCHANGE_URL}" \
    | sed 's/.*"access_token":"\([^"]*\)".*/\1/')
  export BRAVO_AGENT_TOKEN
  echo "Exchanged API token for session JWT"
fi

# ── Clone or pull fleet repo ───────────────────────────────────────────
if [ -n "${FLEET_REPO:-}" ]; then
  if [ -d "$FLEET_DIR/.git" ]; then
    echo "Pulling fleet repo..."
    git -C "$FLEET_DIR" pull --ff-only 2>/dev/null || git -C "$FLEET_DIR" fetch --all
  else
    echo "Cloning fleet repo..."
    REPO_URL="$FLEET_REPO"
    if [ -n "${FLEET_REPO_TOKEN:-}" ]; then
      REPO_URL=$(echo "$FLEET_REPO" | sed "s|https://|https://x-access-token:${FLEET_REPO_TOKEN}@|")
    fi
    git clone --depth 1 "$REPO_URL" "$FLEET_DIR"
  fi
else
  echo "WARNING: FLEET_REPO not set, skipping fleet sync"
fi

# ── Assemble SOUL.md ──────────────────────────────────────────────────
mkdir -p "$AGENT_DIR"

BASE_SOUL="$FLEET_DIR/personas/${ROLE}/SOUL.md"
WORKSPACE_SOUL="$FLEET_DIR/workspaces/${WORKSPACE_SLUG:-_}/agents/${AGENT_NAME}/SOUL.md"

if [ -f "$BASE_SOUL" ]; then
  cp "$BASE_SOUL" "$AGENT_DIR/SOUL.md"
  echo "Loaded base persona: $ROLE"
else
  echo "WARNING: No base persona at $BASE_SOUL"
  echo "# ${AGENT_NAME}" > "$AGENT_DIR/SOUL.md"
fi

# Append workspace-specific override if it exists.
if [ -n "${WORKSPACE_SLUG:-}" ] && [ -f "$WORKSPACE_SOUL" ]; then
  echo "" >> "$AGENT_DIR/SOUL.md"
  echo "---" >> "$AGENT_DIR/SOUL.md"
  echo "" >> "$AGENT_DIR/SOUL.md"
  cat "$WORKSPACE_SOUL" >> "$AGENT_DIR/SOUL.md"
  echo "Loaded workspace override: $WORKSPACE_SLUG/$AGENT_NAME"
fi

# ── Load agent memory ─────────────────────────────────────────────────
MEMORY_DIR="$FLEET_DIR/workspaces/${WORKSPACE_SLUG:-_}/agents/${AGENT_NAME}/memory"
if [ "$ROLE" = "coordinator" ]; then
  MEMORY_DIR="$FLEET_DIR/coordinator/memory"
fi

if [ -d "$MEMORY_DIR" ]; then
  mkdir -p "$AGENT_DIR/memory"
  cp -r "$MEMORY_DIR"/* "$AGENT_DIR/memory/" 2>/dev/null || true
  echo "Loaded memory from $MEMORY_DIR"
fi

# ── Render config.toml ────────────────────────────────────────────────
# Build the optional agentic MCP block (coordinator only).
if [ -n "${AGENTIC_MCP_ENDPOINT:-}" ]; then
  AGENTIC_MCP_BLOCK=$(cat <<MCPEOF
[[mcp.servers]]
name = "agentic"
transport = "http"
url = "${AGENTIC_MCP_ENDPOINT}"
headers = { Authorization = "Bearer ${BRAVO_AGENT_TOKEN}" }
tool_timeout_secs = 60
MCPEOF
)
else
  AGENTIC_MCP_BLOCK="# agentic MCP not configured (worker agent)"
fi
export AGENTIC_MCP_BLOCK

envsubst < /agentic/config.toml.template > "$AGENT_DIR/config.toml"
echo "Config rendered to $AGENT_DIR/config.toml"

# ── Execution event publishing ────────────────────────────────────────
EXEC_ID="exec_$(date +%s%3N)"
EXEC_STARTED_AT=$(date -u +%Y-%m-%dT%H:%M:%SZ)

publish_exec_event() {
  local event_type="$1"
  local extra_data="${2:-}"

  if [ -z "${REDIS_URL:-}" ]; then
    return
  fi

  local payload="{\"type\":\"${event_type}\",\"execution_id\":\"${EXEC_ID}\",\"workspace\":\"${WORKSPACE_SLUG:-}\",\"agent\":\"${AGENT_NAME}\",\"role\":\"${ROLE}\",\"timestamp\":\"$(date -u +%Y-%m-%dT%H:%M:%SZ)\"${extra_data}}"
  redis-cli -u "$REDIS_URL" PUBLISH "agentic:events" "$payload" >/dev/null 2>&1 || true
}

echo ""
echo "=== Starting ZeroClaw Agent ==="

# Publish exec.started event.
publish_exec_event "exec.started" ",\"data\":{\"task\":\"${AGENT_TASK_MESSAGE:-}\"}"

# ── Run agent ─────────────────────────────────────────────────────────
zeroclaw agent -m "${AGENT_TASK_MESSAGE:-Run your standard routine}" 2>&1
EXIT_CODE=$?

# Publish exec.completed or exec.failed event.
if [ "$EXIT_CODE" -eq 0 ]; then
  publish_exec_event "exec.completed" ",\"data\":{\"summary\":\"Agent session completed successfully\"}"
else
  publish_exec_event "exec.failed" ",\"data\":{\"error\":\"Agent exited with code ${EXIT_CODE}\"}"
fi

# ── Push memory changes back ──────────────────────────────────────────
echo ""
echo "=== Syncing Memory ==="

if [ -d "$AGENT_DIR/memory" ] && [ -d "$MEMORY_DIR" ]; then
  cp -r "$AGENT_DIR/memory"/* "$MEMORY_DIR/" 2>/dev/null || true

  cd "$FLEET_DIR"
  if ! git diff --quiet 2>/dev/null; then
    git config user.name "agent-${AGENT_NAME}"
    git config user.email "agent-${AGENT_NAME}@bowrain.cloud"
    git add -A
    git commit -m "memory: ${AGENT_NAME} session $(date -u +%Y-%m-%dT%H:%M:%SZ)"
    git push 2>/dev/null || echo "WARNING: Failed to push memory (will retry next session)"
  else
    echo "No memory changes to commit."
  fi
fi

echo "=== Agent Session End (exit: $EXIT_CODE) ==="
exit $EXIT_CODE
