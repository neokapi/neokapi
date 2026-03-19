# @bravo agent container — ZeroClaw in gateway mode.
# Tiny image, sub-10ms cold start.
#
# Build: docker build -t ghcr.io/neokapi/bravo-agent:latest -f docker/bravo/Dockerfile .
# Run:   docker run -p 42617:42617 ghcr.io/neokapi/bravo-agent:latest

# Prep stage: get envsubst binary and set up entrypoint.
FROM alpine:3.21 AS prep

RUN apk add --no-cache gettext
COPY docker/bravo/config.toml.template /bravo/config.toml.template
COPY docker/bravo/entrypoint.sh /bravo/entrypoint.sh
RUN chmod +x /bravo/entrypoint.sh

FROM ghcr.io/zeroclaw-labs/zeroclaw:latest

WORKDIR /bravo
# Copy envsubst and its dependencies from prep stage.
COPY --from=prep /usr/bin/envsubst /usr/bin/envsubst
COPY --from=prep /usr/lib/libintl.so.8 /usr/lib/libintl.so.8
# Copy shell for entrypoint script.
COPY --from=prep /bin/sh /bin/sh
COPY --from=prep /bravo/ /bravo/

EXPOSE 42617

ENTRYPOINT ["/bravo/entrypoint.sh"]
CMD ["zeroclaw", "gateway"]
