FROM node:24-slim AS build
WORKDIR /src
ENV CI=true

# Install vite-plus (vp) which the package build scripts call into, and enable
# corepack so the pinned pnpm version is available.
RUN apt-get update && apt-get install -y --no-install-recommends curl ca-certificates \
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

# Populate the pnpm store from the lockfile alone (cached until pnpm-lock.yaml
# changes). The packageManager field in package.json pins the pnpm version.
COPY package.json pnpm-lock.yaml pnpm-workspace.yaml ./
# patchedDependencies in pnpm-workspace.yaml reference patches/*.patch — needed
# by `pnpm fetch` to resolve the patched packages.
COPY patches/ patches/
RUN corepack pnpm fetch

# Workspace manifests for ctrl's dependency closure (ctrl → @neokapi/ui →
# concept-ui, contract-types, editor-grid, kapi-format, status-views,
# ui-primitives → neokapi-i18n); glob workspace members resolve only from the
# dirs present here.
COPY packages/concept-ui/package.json packages/concept-ui/
COPY packages/context-explorer/package.json packages/context-explorer/
COPY packages/contract-types/package.json packages/contract-types/
COPY packages/editor-grid/package.json packages/editor-grid/
COPY packages/kapi-format/package.json packages/kapi-format/
COPY packages/i18n-react/package.json packages/i18n-react/
COPY packages/status-views/package.json packages/status-views/
COPY packages/ui/package.json packages/ui/
COPY bowrain/packages/ui/package.json bowrain/packages/ui/
COPY bowrain/apps/ctrl/package.json bowrain/apps/ctrl/
# Install every present workspace project's deps from the store (ctrl bundles
# its @neokapi/* workspace deps from source, so those need their deps too).
RUN corepack pnpm install --offline

# Source for ctrl + its workspace deps (consumed from source via package exports).
COPY packages/concept-ui/ packages/concept-ui/
COPY packages/context-explorer/ packages/context-explorer/
COPY packages/contract-types/ packages/contract-types/
COPY packages/editor-grid/ packages/editor-grid/
COPY packages/kapi-format/ packages/kapi-format/
COPY packages/i18n-react/ packages/i18n-react/
COPY packages/status-views/ packages/status-views/
COPY packages/ui/ packages/ui/
COPY bowrain/packages/ui/ bowrain/packages/ui/
COPY bowrain/apps/ctrl/ bowrain/apps/ctrl/

# @neokapi/i18n-react/runtime is a built ./dist subpath export consumed by
# @neokapi/ui-primitives and @neokapi/status-views; build it before the ctrl
# build so the import resolves.
RUN corepack pnpm --filter @neokapi/i18n-react run build

# Build. Type safety is enforced by `vp check` in CI, not the build.
RUN corepack pnpm --filter bowrain-ctrl run build

FROM nginx:1-alpine
RUN rm /etc/nginx/conf.d/default.conf
COPY bowrain/docker/bowrain-ctrl/nginx.conf /etc/nginx/conf.d/default.conf
# Security response headers, included from every location in nginx.conf. Outside
# conf.d on purpose: these are bare add_header directives, not a server block.
COPY bowrain/docker/nginx/security-headers.conf /etc/nginx/security-headers.conf
COPY bowrain/docker/bowrain-ctrl/csp.conf /etc/nginx/csp.conf
COPY --from=build /src/bowrain/apps/ctrl/dist /usr/share/nginx/html
EXPOSE 80
