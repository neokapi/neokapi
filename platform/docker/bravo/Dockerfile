# @bravo agent container — ZeroClaw in gateway mode.
# Tiny image (~16MB), sub-10ms cold start.
#
# Build: docker build -t ghcr.io/neokapi/bravo-agent:latest -f docker/bravo/Dockerfile .
# Run:   docker run -p 42617:42617 -v /path/to/config.toml:/bravo/config.toml ghcr.io/neokapi/bravo-agent:latest

FROM ghcr.io/zeroclaw-labs/zeroclaw:latest AS runtime

WORKDIR /bravo
COPY docker/bravo/config.toml.template /bravo/config.toml.template

# Config is injected at runtime via volume mount or environment variable substitution.
# The entrypoint script renders config.toml from the template using envsubst.
COPY docker/bravo/entrypoint.sh /bravo/entrypoint.sh
RUN chmod +x /bravo/entrypoint.sh

EXPOSE 42617

ENTRYPOINT ["/bravo/entrypoint.sh"]
CMD ["zeroclaw", "gateway"]
