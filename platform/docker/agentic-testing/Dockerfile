# Agentic testing agent container — ZeroClaw in single-shot agent mode.
#
# Unlike bravo-agent (gateway mode for @bravo chat), this image runs
# `zeroclaw agent -m "<task>"` as a one-shot session, syncs the fleet
# repo for persona/memory, and exits.
#
# Build: docker build -t ghcr.io/neokapi/agentic-agent:latest -f platform/docker/agentic-testing/Dockerfile .

# ── Prep stage: entrypoint + tools ──────────────────────────────────────
FROM alpine:3.21 AS prep
RUN apk add --no-cache git gettext
COPY platform/docker/agentic-testing/entrypoint.sh /agentic/entrypoint.sh
RUN chmod +x /agentic/entrypoint.sh

# ── Final: ZeroClaw + git + entrypoint ──────────────────────────────────
FROM ghcr.io/zeroclaw-labs/zeroclaw:latest

# Install git and envsubst (gettext) for fleet repo sync + config templating.
# ZeroClaw base image is Alpine-based.
USER root
RUN apk add --no-cache git gettext

COPY --from=prep /agentic/entrypoint.sh /agentic/entrypoint.sh

# Config template for agent mode (rendered at runtime by entrypoint).
COPY platform/docker/agentic-testing/config.toml.template /agentic/config.toml.template

# No baked-in SOUL.md — assembled at runtime from fleet repo.
# No gateway port — agent mode is single-shot.

ENTRYPOINT ["/bin/sh", "/agentic/entrypoint.sh"]
