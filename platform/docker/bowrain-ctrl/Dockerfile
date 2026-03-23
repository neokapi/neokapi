FROM node:22-slim AS build
WORKDIR /src
COPY platform/package.json platform/package-lock.json platform/
COPY platform/packages/ui/package.json platform/packages/ui/
COPY platform/apps/ctrl/package.json platform/apps/ctrl/
RUN cd platform && npm ci
COPY platform/packages/ui/ platform/packages/ui/
RUN cd platform/packages/ui && npx tsc
COPY platform/apps/ctrl/ platform/apps/ctrl/
RUN cd platform/apps/ctrl && npm run build

FROM nginx:1-alpine
RUN rm /etc/nginx/conf.d/default.conf
COPY platform/docker/bowrain-ctrl/nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=build /src/platform/apps/ctrl/dist /usr/share/nginx/html
EXPOSE 80
