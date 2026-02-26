# Bowrain async worker.
# Requires bowrain-base to be built first (provides compiled binaries).
#
# Build: docker build -f docker/bowrain-worker/Dockerfile --build-context bowrain-base=docker-image://bowrain-base .

FROM alpine:3.21
RUN apk add --no-cache ca-certificates tzdata

COPY --from=bowrain-base /bowrain-worker /usr/local/bin/bowrain-worker

ENTRYPOINT ["bowrain-worker"]
