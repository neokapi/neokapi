# Bowrain Web UI — pure static file server.
# Serves the SPA with fallback to index.html. No reverse proxying —
# use Traefik (see deploy/docker/) or Azure Front Door for routing.
#
# Build:  docker build -f docker/bowrain-web/Dockerfile \
#           --build-context bowrain-base=docker-image://bowrain-base:latest .
# Run:    docker run -p 3000:80 bowrain-web

FROM nginx:1-alpine
RUN rm /etc/nginx/conf.d/default.conf
COPY docker/bowrain-web/nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=bowrain-base /dist /usr/share/nginx/html
EXPOSE 80
