FROM node:24-slim AS build
WORKDIR /src

# Install vite-plus (vp) which the package build scripts call into.
# Shim `vpx` → `npx` since the install script only ships `vp`; both
# just run a tool from the workspace node_modules.
RUN apt-get update && apt-get install -y --no-install-recommends curl ca-certificates \
    && curl -fsSL https://viteplus.dev/install.sh | bash \
    && cp /root/.vite-plus/bin/vp /usr/local/bin/vp \
    && chmod +x /usr/local/bin/vp \
    && printf '#!/bin/sh\nexec npx "$@"\n' > /usr/local/bin/vpx \
    && chmod +x /usr/local/bin/vpx \
    && rm -rf /var/lib/apt/lists/*
COPY package.json package-lock.json ./
COPY packages/kapi-format/package.json packages/kapi-format/
COPY packages/ui/package.json packages/ui/
COPY bowrain/packages/ui/package.json bowrain/packages/ui/
COPY bowrain/apps/ctrl/package.json bowrain/apps/ctrl/
RUN npm ci
COPY packages/kapi-format/ packages/kapi-format/
COPY packages/ui/ packages/ui/
COPY bowrain/packages/ui/ bowrain/packages/ui/
RUN cd bowrain/packages/ui && npx tsc
COPY bowrain/apps/ctrl/ bowrain/apps/ctrl/
RUN cd bowrain/apps/ctrl && npm run build

FROM nginx:1-alpine
RUN rm /etc/nginx/conf.d/default.conf
COPY bowrain/docker/bowrain-ctrl/nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=build /src/bowrain/apps/ctrl/dist /usr/share/nginx/html
EXPOSE 80
