FROM node:22-alpine AS build
WORKDIR /app
COPY package.json package-lock.json ./
COPY packages/ui/package.json packages/ui/
COPY apps/ctrl/package.json apps/ctrl/
RUN npm ci --workspace=apps/ctrl
COPY packages/ui/ packages/ui/
COPY apps/ctrl/ apps/ctrl/
RUN npm run build --workspace=apps/ctrl

FROM nginx:alpine
COPY --from=build /app/apps/ctrl/dist /usr/share/nginx/html
COPY docker/bowrain-ctrl/nginx.conf /etc/nginx/conf.d/default.conf
