# Bowrain Web UI — pure static file server.
# Self-contained build: includes Node stages to compile the SPA from source.
# No base image dependency — can be built independently of Go/server builds.
#
# Build:  docker build -f platform/docker/bowrain-web/Dockerfile .
# Run:    docker run -p 3000:80 bowrain-web

# ── Stage 1: Build web UI ──────────────────────────────────────────────────
FROM node:22-alpine AS web-builder
WORKDIR /src

# Copy workspace root (lock file + workspace config) and package.json files
# for all workspaces to leverage Docker layer caching for npm ci.
COPY platform/package.json platform/package-lock.json platform/
COPY platform/packages/ui/package.json platform/packages/ui/
COPY platform/apps/web/package.json platform/apps/web/
COPY platform/apps/bowrain/frontend/package.json platform/apps/bowrain/frontend/
RUN cd platform && npm ci

# Build shared UI package
COPY platform/packages/ui/ platform/packages/ui/
RUN cd platform/packages/ui && npx tsc

# Build web app
COPY platform/apps/web/ platform/apps/web/
RUN cd platform/apps/web && npm run build

# ── Stage 2: Serve with nginx ──────────────────────────────────────────────
FROM nginx:1-alpine
RUN rm /etc/nginx/conf.d/default.conf
COPY platform/docker/bowrain-web/nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=web-builder /src/platform/apps/web/dist /usr/share/nginx/html

# Inject build info as a static JSON file served alongside the SPA.
ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_DATE=unknown
RUN printf '{"version":"%s","commit":"%s","build_date":"%s","component":"web"}\n' \
    "$VERSION" "$COMMIT" "$BUILD_DATE" > /usr/share/nginx/html/version.json

EXPOSE 80
