#!/bin/sh
# Agentic testing agent entrypoint.
#
# Environment variables:
#   AGENT_NAME          — agent identifier (e.g., "coordinator", "sophie-translator")
#   AGENT_ROLE          — persona role (e.g., "coordinator", "translator")
#   AGENT_TASK_MESSAGE  — task message for this session
#   AGENT_LOCALE        — target locale for this agent (e.g., "fr-FR")
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

set -u

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
  EXCHANGE_RESP=$(wget -qO- \
    --header="Authorization: Bearer ${BOWRAIN_API_TOKEN}" \
    --post-data="" \
    "${EXCHANGE_URL}" 2>/dev/null || true)
  if [ -n "$EXCHANGE_RESP" ]; then
    BRAVO_AGENT_TOKEN=$(echo "$EXCHANGE_RESP" | sed 's/.*"access_token":"\([^"]*\)".*/\1/')
    export BRAVO_AGENT_TOKEN
    echo "Exchanged API token for session JWT"
  else
    echo "WARNING: Token exchange failed (401?), using raw API token"
    BRAVO_AGENT_TOKEN="${BOWRAIN_API_TOKEN}"
    export BRAVO_AGENT_TOKEN
  fi
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
    if ! git clone --depth 1 "$REPO_URL" "$FLEET_DIR"; then
      echo "WARNING: Failed to clone fleet repo, continuing without fleet state"
    fi
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

  # Build redis-cli args. Azure Redis requires TLS (--tls) and auth (-a).
  REDIS_CLI_ARGS=""
  case "$REDIS_URL" in
    rediss://*) REDIS_CLI_ARGS="--tls" ;;
  esac
  # Extract host and port from URL (rediss://host:port or redis://host:port).
  REDIS_HOST=$(echo "$REDIS_URL" | sed 's|rediss\?://||' | cut -d: -f1)
  REDIS_PORT=$(echo "$REDIS_URL" | sed 's|rediss\?://||' | cut -d: -f2)

  redis-cli -h "$REDIS_HOST" -p "${REDIS_PORT:-6380}" ${REDIS_CLI_ARGS} \
    ${REDIS_PASSWORD:+-a "$REDIS_PASSWORD"} \
    PUBLISH "agentic:events" "$payload" >/dev/null 2>&1 || echo "WARNING: Failed to publish event ${event_type}"
}

echo ""
echo "=== Starting ZeroClaw Agent ==="

# Publish exec.started event.
LOCALE="${AGENT_LOCALE:-}"
publish_exec_event "exec.started" ",\"data\":{\"task\":\"${AGENT_TASK_MESSAGE:-}\",\"locale\":\"${LOCALE}\"}"

# ── Run agent ─────────────────────────────────────────────────────────
START_EPOCH=$(date +%s)
AGENT_LOG="/tmp/agent-output.log"

# Run agent, capturing output to a log file. We avoid piping through tee
# because $? in plain sh reflects the last pipe stage, not the agent.
set +e
# Feed 'A' (Always) to stdin so ZeroClaw's interactive permission prompts
# are auto-approved. Repeat enough times to cover all tool calls.
yes A | zeroclaw agent -m "${AGENT_TASK_MESSAGE:-Run your standard routine}" >"$AGENT_LOG" 2>&1
EXIT_CODE=$?
set -e
cat "$AGENT_LOG"

END_EPOCH=$(date +%s)
DURATION_SEC=$((END_EPOCH - START_EPOCH))

# ── Extract token usage from agent output ─────────────────────────────
# ZeroClaw prints a usage summary line like: "Tokens used: 12345" or
# "Total tokens: 12345". We extract the last number after the keyword.
TOKENS_USED=0
if [ -f "$AGENT_LOG" ]; then
  TOKENS_LINE=$(grep -i 'tokens.*used\|total.*tokens' "$AGENT_LOG" | tail -1 || true)
  if [ -n "$TOKENS_LINE" ]; then
    TOKENS_USED=$(echo "$TOKENS_LINE" | grep -oE '[0-9]+' | tail -1 || echo 0)
  fi
fi

# Publish exec.completed or exec.failed event with duration and tokens.
if [ "$EXIT_CODE" -eq 0 ]; then
  publish_exec_event "exec.completed" ",\"data\":{\"summary\":\"Agent session completed successfully\",\"duration_sec\":${DURATION_SEC},\"tokens_used\":${TOKENS_USED}}"
else
  publish_exec_event "exec.failed" ",\"data\":{\"error\":\"Agent exited with code ${EXIT_CODE}\",\"duration_sec\":${DURATION_SEC},\"tokens_used\":${TOKENS_USED}}"
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
