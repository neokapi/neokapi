#!/bravo/busybox sh
# Render config.toml from template — only MCP vars need substitution.
# Model/provider config uses ZEROCLAW_* env vars read directly by ZeroClaw.
set -e

/bravo/busybox sed \
  -e "s|\${BRAVO_MCP_ENDPOINT}|${BRAVO_MCP_ENDPOINT}|g" \
  -e "s|\${BRAVO_AGENT_TOKEN}|${BRAVO_AGENT_TOKEN}|g" \
  /bravo/config.toml.template > /zeroclaw-data/.zeroclaw/config.toml

exec "$@"
