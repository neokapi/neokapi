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
# libatomic1: the Node runtime the vite-plus installer downloads links it, and
# node:24-slim does not ship it.
RUN apt-get update && apt-get install -y --no-install-recommends curl ca-certificates libatomic1 \
    && curl -fsSL https://viteplus.dev/install.sh | bash \
    # Copy through the installer's own symlink rather than a path inside it.
    # ~/.local/share/vite-plus/bin/vp points at current/bin/vp, and current/
    # points at the installed version — so a version bump moves the binary but
    # not this path. Naming a path inside the tree is what broke the build when
    # the installer moved from ~/.vite-plus to ~/.local/share/vite-plus.
    && install -m 0755 /root/.local/share/vite-plus/bin/vp /usr/local/bin/vp \
    && ln -sf vp /usr/local/bin/vpx \
    && corepack enable \
    && rm -rf /var/lib/apt/lists/*

# The lockfile, the workspace definition and the patches it names. The
# dependencies are fetched in the build step below, not in a layer of their
# own: that layer held the pnpm store and node_modules (about 1 GiB), and the
# gha layer cache the image builds export kept it for a fetch that takes
# about 20 seconds. The lockfile records no tarball URLs, so pnpm fetches from
# the registry its configuration names: .npmrc names the one the lockfile was
# resolved against (scripts/check-lockfiles.py holds it to that).
COPY package.json pnpm-lock.yaml pnpm-workspace.yaml .npmrc ./
COPY patches/ patches/

# Source for the web app + its workspace deps. Most @neokapi/* packages are
# consumed from source (their exports point at ./src), so the Vite build follows
# the import graph into them. The exception is @neokapi/i18n-react, whose
# ./runtime export is dist-only, so it is built before the web app.
COPY packages/kapi-format/ packages/kapi-format/
COPY packages/i18n-react/ packages/i18n-react/
COPY packages/ui/ packages/ui/
COPY packages/concept-ui/ packages/concept-ui/
COPY packages/context-explorer/ packages/context-explorer/
COPY packages/contract-types/ packages/contract-types/
# The generated reference datasets. packages/ui reads format-families.json to
# decide which formats preview as a keyed table, so the web bundle needs it.
COPY packages/reference-data/ packages/reference-data/
COPY packages/editor-grid/ packages/editor-grid/
COPY packages/status-views/ packages/status-views/
COPY packages/project-screens/ packages/project-screens/
COPY packages/flow-editor/ packages/flow-editor/
COPY packages/bowrain-client/ packages/bowrain-client/
COPY bowrain/packages/ui/ bowrain/packages/ui/
# bowrain/apps/web's vite.config imports ../../packages/app/neokapi-i18n.config.json
# (the extract config shared with the desktop app), so the app package must be
# in the build context.
COPY bowrain/packages/app/ bowrain/packages/app/
COPY bowrain/apps/web/ bowrain/apps/web/
# The vite config imports the qps plugin from scripts/. It is skipped here
# (CI=true): an image carries a qps dictionary only when the build context
# already holds one (make bowrain-app-qps, bowrain-ctrl-qps, bowrain-pulse-qps).
COPY scripts/vite-qps-catalog.mjs scripts/

# One step fetches the dependencies, installs every present workspace
# project's deps (the web app bundles its @neokapi/* workspace deps from
# source, so their third-party deps are needed too), builds, and removes
# node_modules, so the layer keeps the sources and the built dist only. The
# pnpm store sits on a cache mount, which the layer cache does not export;
# sharing=locked keeps two builds on one builder from writing it at once.
# @neokapi/i18n-react/runtime is a built ./dist subpath export consumed by
# @neokapi/ui, so it builds before the web app. Type safety is enforced by
# `vp check` in CI, not the build.
RUN --mount=type=cache,id=bowrain-web-pnpm-store,target=/pnpm-store,sharing=locked \
    corepack pnpm fetch --store-dir /pnpm-store \
    && corepack pnpm install --offline --store-dir /pnpm-store \
    && corepack pnpm --filter @neokapi/i18n-react run build \
    && corepack pnpm --filter neokapi-web run build \
    && find /src -name node_modules -type d -prune -exec rm -rf {} +

# ── Stage 2: Serve with nginx ──────────────────────────────────────────────
FROM nginx:1-alpine
# The base tag can lag Alpine's security fixes by weeks (libxml2 2.13.9-r3 was
# out while nginx:1-alpine still carried r2), so each build takes them.
RUN apk upgrade --no-cache && rm /etc/nginx/conf.d/default.conf
COPY bowrain/docker/bowrain-web/nginx.conf /etc/nginx/conf.d/default.conf
# Security response headers, included from every location in nginx.conf. Outside
# conf.d on purpose: these are bare add_header directives, not a server block.
COPY bowrain/docker/nginx/security-headers.conf /etc/nginx/security-headers.conf
COPY bowrain/docker/bowrain-web/csp.conf /etc/nginx/csp.conf
COPY --from=web-builder /src/bowrain/apps/web/dist /usr/share/nginx/html

# Inject build info as a static JSON file served alongside the SPA.
ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_DATE=unknown
RUN printf '{"version":"%s","commit":"%s","build_date":"%s","component":"web"}\n' \
    "$VERSION" "$COMMIT" "$BUILD_DATE" > /usr/share/nginx/html/version.json

EXPOSE 80
