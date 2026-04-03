# Persona agent container — ZeroClaw in single-shot agent mode.
#
# Unlike bravo-agent (gateway mode for @bravo chat), this image runs
# `zeroclaw agent -m "<task>"` as a one-shot session, syncs the fleet
# repo for persona/memory, and exits.
#
# Build: docker build -t ghcr.io/neokapi/persona-agent:latest -f bowrain/docker/persona-agent/Dockerfile .

# ── Stage 1: grab zeroclaw binary from official image ─────────────────
FROM ghcr.io/zeroclaw-labs/zeroclaw:latest AS zeroclaw

# ── Stage 2: Debian slim with shell + tools + zeroclaw ──────────────────
FROM debian:bookworm-slim

# Install runtime dependencies: git (fleet repo), envsubst (config templating),
# redis-tools (execution event publishing via redis-cli).
RUN apt-get update && apt-get install -y --no-install-recommends \
    git gettext-base redis-tools ca-certificates curl \
    && rm -rf /var/lib/apt/lists/*

# Copy zeroclaw binary from official image.
COPY --from=zeroclaw /usr/local/bin/zeroclaw /usr/local/bin/zeroclaw

# Copy entrypoint and config template.
COPY bowrain/docker/persona-agent/entrypoint.sh /agentic/entrypoint.sh
COPY bowrain/docker/persona-agent/config.toml.template /agentic/config.toml.template
RUN chmod +x /agentic/entrypoint.sh

# No baked-in SOUL.md — assembled at runtime from fleet repo.
# No gateway port — agent mode is single-shot.

ENTRYPOINT ["/bin/sh", "/agentic/entrypoint.sh"]
