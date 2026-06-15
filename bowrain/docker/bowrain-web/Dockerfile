# Bowrain Web UI — pure static file server.
# Self-contained build: includes Node stages to compile the SPA from source.
# No base image dependency — can be built independently of Go/server builds.
#
# Build:  docker build -f bowrain/docker/bowrain-web/Dockerfile .
# Run:    docker run -p 3000:80 bowrain-web

# ── Stage 1: Build web UI ──────────────────────────────────────────────────
FROM node:24-slim AS web-builder
WORKDIR /src
ENV CI=true

# Install vite-plus (vp) which the package build scripts call into, and enable
# corepack so the pinned pnpm version is available.
RUN apt-get update && apt-get install -y --no-install-recommends curl ca-certificates \
    && curl -fsSL https://viteplus.dev/install.sh | bash \
    && cp /root/.vite-plus/bin/vp /usr/local/bin/vp \
    && chmod +x /usr/local/bin/vp \
    && ln -sf vp /usr/local/bin/vpx \
    && corepack enable \
    && rm -rf /var/lib/apt/lists/*

# Populate the pnpm store from the lockfile alone — this layer is cached until
# pnpm-lock.yaml changes (the network-heavy step). The packageManager field in
# package.json pins the exact pnpm version corepack runs.
COPY package.json pnpm-lock.yaml pnpm-workspace.yaml ./
# pnpm-workspace.yaml references patchedDependencies (patches/*.patch); they must
# be present for `pnpm fetch` to resolve the patched packages.
COPY patches/ patches/
RUN corepack pnpm fetch

# Workspace manifests for the web app's dependency closure. The glob workspace
# members in pnpm-workspace.yaml resolve only from the dirs present here, so a
# partial checkout is a valid (smaller) workspace.
COPY packages/kapi-format/package.json packages/kapi-format/
COPY packages/kapi-react/package.json packages/kapi-react/
COPY packages/ui/package.json packages/ui/
COPY packages/flow-editor/package.json packages/flow-editor/
COPY bowrain/packages/ui/package.json bowrain/packages/ui/
COPY bowrain/apps/web/package.json bowrain/apps/web/
# Install every present workspace project's deps from the store (the web app
# bundles its @neokapi/* workspace deps from source, so those packages need
# their own third-party deps, e.g. @codemirror/*, materialized too).
RUN corepack pnpm install --offline

# Source for the web app + its workspace deps. Most @neokapi/* packages are
# consumed from source (their exports point at ./src), so the Vite build follows
# the import graph into them. The exception is @neokapi/kapi-react, whose
# ./runtime export is dist-only — it must be built before the web build.
COPY packages/kapi-format/ packages/kapi-format/
COPY packages/kapi-react/ packages/kapi-react/
COPY packages/ui/ packages/ui/
COPY packages/flow-editor/ packages/flow-editor/
COPY bowrain/packages/ui/ bowrain/packages/ui/
COPY bowrain/apps/web/ bowrain/apps/web/

# @neokapi/kapi-react/runtime is a built ./dist subpath export consumed by
# @neokapi/ui; build it before the web app so the import resolves.
RUN corepack pnpm --filter @neokapi/kapi-react run build

# Build the web app. Type safety is enforced by `vp check` in CI, not the build.
RUN corepack pnpm --filter neokapi-web run build

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
