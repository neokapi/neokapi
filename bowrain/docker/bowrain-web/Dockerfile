# Bowrain Web UI — pure static file server.
# Self-contained build: includes Node stages to compile the SPA from source.
# No base image dependency — can be built independently of Go/server builds.
#
# Build:  docker build -f bowrain/docker/bowrain-web/Dockerfile .
# Run:    docker run -p 3000:80 bowrain-web

# ── Stage 1: Build web UI ──────────────────────────────────────────────────
FROM node:24-slim AS web-builder
WORKDIR /src

# Install vite-plus (vp) which the package build scripts call into.
# The install script only ships `vp`; `vpx` is a companion tool that
# normally comes from `vp env use`, but for deterministic builds we
# just shim `vpx` → `npx` (same effect: run a tool from node_modules).
RUN apt-get update && apt-get install -y --no-install-recommends curl ca-certificates \
    && curl -fsSL https://viteplus.dev/install.sh | bash \
    && cp /root/.vite-plus/bin/vp /usr/local/bin/vp \
    && chmod +x /usr/local/bin/vp \
    && printf '#!/bin/sh\nexec npx "$@"\n' > /usr/local/bin/vpx \
    && chmod +x /usr/local/bin/vpx \
    && rm -rf /var/lib/apt/lists/*

# Copy workspace root (lock file + workspace config) and package.json files
# for all workspaces to leverage Docker layer caching for npm ci.
COPY package.json package-lock.json ./
COPY packages/kapi-format/package.json packages/kapi-format/
COPY packages/ui/package.json packages/ui/
COPY packages/flow-editor/package.json packages/flow-editor/
COPY bowrain/packages/ui/package.json bowrain/packages/ui/
COPY bowrain/apps/web/package.json bowrain/apps/web/
COPY bowrain/apps/bowrain/frontend/package.json bowrain/apps/bowrain/frontend/
RUN npm ci

# Build shared UI package — needs source from kapi-format too since
# @neokapi/ui-primitives type-imports from `@neokapi/kapi-format` and
# tsc follows those into the package's TS source. @neokapi/flow-editor is
# consumed from source (its exports point at ./src), so the web app's tsc
# follows into it the same way — copy the source, no separate build.
COPY packages/kapi-format/ packages/kapi-format/
COPY packages/ui/ packages/ui/
COPY packages/flow-editor/ packages/flow-editor/
COPY bowrain/packages/ui/ bowrain/packages/ui/
RUN cd bowrain/packages/ui && npx tsc

# Build web app
COPY bowrain/apps/web/ bowrain/apps/web/
RUN cd bowrain/apps/web && npm run build

# ── Stage 2: Serve with nginx ──────────────────────────────────────────────
FROM nginx:1-alpine
RUN rm /etc/nginx/conf.d/default.conf
COPY bowrain/docker/bowrain-web/nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=web-builder /src/bowrain/apps/web/dist /usr/share/nginx/html

# Inject build info as a static JSON file served alongside the SPA.
ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_DATE=unknown
RUN printf '{"version":"%s","commit":"%s","build_date":"%s","component":"web"}\n' \
    "$VERSION" "$COMMIT" "$BUILD_DATE" > /usr/share/nginx/html/version.json

EXPOSE 80
