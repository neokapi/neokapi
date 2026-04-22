FROM node:24-slim AS build
WORKDIR /src

# Install vite-plus (vp + vpx) — matches the setup-vp GitHub Action
# that CI uses. Package build scripts call `vpx tsc` + `vp build`.
RUN apt-get update && apt-get install -y --no-install-recommends curl ca-certificates \
    && curl -fsSL https://viteplus.dev/install.sh | bash \
    && rm -rf /var/lib/apt/lists/*
ENV PATH="/root/.vite-plus/bin:${PATH}"
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
