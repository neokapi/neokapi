FROM node:24-slim AS build
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

# Populate the pnpm store from the lockfile alone (cached until pnpm-lock.yaml
# changes). The packageManager field in package.json pins the pnpm version.
COPY package.json pnpm-lock.yaml pnpm-workspace.yaml ./
# patchedDependencies in pnpm-workspace.yaml reference patches/*.patch — needed
# by `pnpm fetch` to resolve the patched packages.
COPY patches/ patches/
RUN corepack pnpm fetch

# Workspace manifests for ctrl's dependency closure; glob workspace members
# resolve only from the dirs present here.
COPY packages/kapi-format/package.json packages/kapi-format/
COPY packages/ui/package.json packages/ui/
COPY bowrain/packages/ui/package.json bowrain/packages/ui/
COPY bowrain/apps/ctrl/package.json bowrain/apps/ctrl/
# Install every present workspace project's deps from the store (ctrl bundles
# its @neokapi/* workspace deps from source, so those need their deps too).
RUN corepack pnpm install --offline

# Source for ctrl + its workspace deps (consumed from source via package exports).
COPY packages/kapi-format/ packages/kapi-format/
COPY packages/ui/ packages/ui/
COPY bowrain/packages/ui/ bowrain/packages/ui/
COPY bowrain/apps/ctrl/ bowrain/apps/ctrl/

# Build. Type safety is enforced by `vp check` in CI, not the build.
RUN corepack pnpm --filter bowrain-ctrl run build

FROM nginx:1-alpine
RUN rm /etc/nginx/conf.d/default.conf
COPY bowrain/docker/bowrain-ctrl/nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=build /src/bowrain/apps/ctrl/dist /usr/share/nginx/html
EXPOSE 80
