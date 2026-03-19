# @bravo agent container — ZeroClaw gateway for the Bowrain platform.
#
# Uses ZeroClaw's native env var config for model/provider settings.
# Environment variables (injected by the ACA runtime):
#   ZEROCLAW_PROVIDER        — AI provider (e.g. "azure_openai", "anthropic")
#   ZEROCLAW_API_KEY         — Provider API key
#   ZEROCLAW_MODEL           — Model name (e.g. "gpt-4o")
#   ZEROCLAW_API_BASE        — Provider API base URL
#   ZEROCLAW_API_VERSION     — API version (for Azure OpenAI)
#
# Build: docker build -t ghcr.io/neokapi/bravo-agent:latest -f docker/bravo/Dockerfile .
# Run:   docker run -p 42617:42617 -e ZEROCLAW_PROVIDER=anthropic -e ZEROCLAW_API_KEY=sk-... ghcr.io/neokapi/bravo-agent:latest

FROM ghcr.io/zeroclaw-labs/zeroclaw:latest

# Overwrite default config with bravo-specific settings:
# - Pairing disabled (containers are internal-only, accessed by the worker)
# - Public bind enabled (required for container networking)
# - SQLite memory enabled
COPY docker/bravo/config.toml /zeroclaw-data/.zeroclaw/config.toml

ENV ZEROCLAW_GATEWAY_PORT=42617

EXPOSE 42617

ENTRYPOINT ["zeroclaw"]
CMD ["gateway", "start", "--host", "0.0.0.0"]
