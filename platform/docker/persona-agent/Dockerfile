# Persona agent container — ZeroClaw in single-shot agent mode.
#
# Unlike bravo-agent (gateway mode for @bravo chat), this image runs
# `zeroclaw agent -m "<task>"` as a one-shot session, syncs the fleet
# repo for persona/memory, and exits.
#
# Build: docker build -t ghcr.io/neokapi/persona-agent:latest -f platform/docker/persona-agent/Dockerfile .

# ── Stage 1: grab zeroclaw binary from official image ─────────────────
FROM ghcr.io/zeroclaw-labs/zeroclaw:latest AS zeroclaw

# ── Stage 2: Alpine with shell + tools + zeroclaw ─────────────────────
FROM alpine:3.21

# Install runtime dependencies: git (fleet repo), envsubst (config templating),
# redis (execution event publishing via redis-cli).
RUN apk add --no-cache git gettext redis gcompat libgcc

# Copy zeroclaw binary from official image.
COPY --from=zeroclaw /usr/local/bin/zeroclaw /usr/local/bin/zeroclaw

# Copy entrypoint and config template.
COPY platform/docker/persona-agent/entrypoint.sh /agentic/entrypoint.sh
COPY platform/docker/persona-agent/config.toml.template /agentic/config.toml.template
RUN chmod +x /agentic/entrypoint.sh

# No baked-in SOUL.md — assembled at runtime from fleet repo.
# No gateway port — agent mode is single-shot.

ENTRYPOINT ["/bin/sh", "/agentic/entrypoint.sh"]
