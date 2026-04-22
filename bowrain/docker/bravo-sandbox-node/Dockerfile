# Sandbox image for @bravo Node.js code execution.
# Minimal Node runtime with common localization libraries pre-installed.
# The container runs with a read-only root filesystem; /workspace is the
# only writable mount (tmpfs, noexec).
#
# Build: docker build -t ghcr.io/neokapi/bravo-sandbox-node:latest -f bowrain/docker/bravo-sandbox-node/Dockerfile .

FROM node:24-slim

# Install localization-relevant libraries globally.
RUN npm install -g --no-fund --no-audit \
    i18next@24.2.2 \
    @formatjs/icu-messageformat-parser@2.9.8 \
    yaml@2.7.0 \
    csv-parse@5.6.0 \
    csv-stringify@6.5.2 \
    && npm cache clean --force

# Non-root user for additional isolation.
RUN adduser --disabled-password --gecos "" --home /workspace sandbox
USER sandbox
WORKDIR /workspace

# Ensure globally installed modules are resolvable.
ENV NODE_PATH=/usr/local/lib/node_modules
