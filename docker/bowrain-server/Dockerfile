# Bowrain API server.
# Requires bowrain-base to be built first (provides compiled binaries).
#
# Build: docker build -f docker/bowrain-server/Dockerfile --build-context bowrain-base=docker-image://bowrain-base .

FROM alpine:3.21
RUN apk add --no-cache ca-certificates tzdata

COPY --from=bowrain-base /bowrain-server /usr/local/bin/bowrain-server

VOLUME /data
ENV BOWRAIN_STORE=/data/bowrain.db

EXPOSE 8080
ENTRYPOINT ["bowrain-server"]
