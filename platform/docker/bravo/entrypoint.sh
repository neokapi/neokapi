#!/bravo/busybox sh
# Render config.toml from template by substituting env vars with sed.
set -e

# If a long-lived API token is provided, exchange it for a short-lived JWT.
# This keeps the MCP server OAuth 2.1 compliant — agents authenticate with
# a bwt_* token from Key Vault and get a 1-hour JWT for the session.
if [ -n "${BOWRAIN_API_TOKEN}" ]; then
  EXCHANGE_URL="${BRAVO_MCP_ENDPOINT%/mcp/}/api/v1/auth/token/exchange"
  BRAVO_AGENT_TOKEN=$(/bravo/busybox wget -qO- \
    --header="Authorization: Bearer ${BOWRAIN_API_TOKEN}" \
    --post-data="" \
    "${EXCHANGE_URL}" \
    | /bravo/busybox sed 's/.*"access_token":"\([^"]*\)".*/\1/')
  export BRAVO_AGENT_TOKEN
fi

/bravo/busybox sed \
  -e "s|\${BRAVO_MODEL_PROVIDER}|${BRAVO_MODEL_PROVIDER}|g" \
  -e "s|\${BRAVO_MODEL_NAME}|${BRAVO_MODEL_NAME}|g" \
  -e "s|\${BRAVO_MODEL_API_BASE}|${BRAVO_MODEL_API_BASE}|g" \
  -e "s|\${BRAVO_MODEL_API_KEY}|${BRAVO_MODEL_API_KEY}|g" \
  -e "s|\${BRAVO_MCP_ENDPOINT}|${BRAVO_MCP_ENDPOINT}|g" \
  -e "s|\${BRAVO_AGENT_TOKEN}|${BRAVO_AGENT_TOKEN}|g" \
  /bravo/config.toml.template > /zeroclaw-data/.zeroclaw/config.toml

exec "$@"
