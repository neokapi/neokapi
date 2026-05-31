# Sandbox image for @bravo Node.js code execution.
# Minimal Node runtime with common localization libraries pre-installed.
# The container runs with a read-only root filesystem; /workspace is the
# only writable mount (tmpfs, noexec).
#
# Build: docker build -t ghcr.io/neokapi/bravo-sandbox-node:latest -f bowrain/docker/bravo-sandbox-node/Dockerfile .

FROM node:24-slim
RUN corepack enable

# Localization libraries the sandbox can require(). Installed with pnpm into a
# flat (hoisted) node_modules exposed via NODE_PATH — equivalent to a global
# install, without npm.
WORKDIR /opt/sandbox-libs
RUN printf 'node-linker=hoisted\n' > .npmrc \
    && printf '%s\n' '{"name":"bravo-sandbox-libs","private":true,"dependencies":{"i18next":"24.2.2","@formatjs/icu-messageformat-parser":"2.9.8","yaml":"2.7.0","csv-parse":"5.6.0","csv-stringify":"6.5.2"}}' > package.json \
    && corepack pnpm install --prod \
    && corepack pnpm store prune

# Non-root user for additional isolation.
RUN adduser --disabled-password --gecos "" --home /workspace sandbox
USER sandbox
WORKDIR /workspace

# Ensure the installed modules are resolvable by require().
ENV NODE_PATH=/opt/sandbox-libs/node_modules
