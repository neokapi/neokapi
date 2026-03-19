# @bravo agent container — ZeroClaw gateway with MCP integration.
#
# Build: docker build -t ghcr.io/neokapi/bravo-agent:latest -f docker/bravo/Dockerfile .

# Prep stage: busybox + config template.
FROM busybox:musl AS prep
COPY docker/bravo/config.toml.template /bravo/config.toml.template
COPY docker/bravo/entrypoint.sh /bravo/entrypoint.sh
RUN chmod +x /bravo/entrypoint.sh

# Final: ZeroClaw + busybox + config template.
FROM ghcr.io/zeroclaw-labs/zeroclaw:latest
COPY --from=prep /bin/busybox /bravo/busybox
COPY --from=prep /bravo/ /bravo/
# Bake workspace identity + scoping files into the image.
COPY docker/bravo/workspace/SOUL.md /zeroclaw-data/workspace/SOUL.md
COPY docker/bravo/workspace/IDENTITY.md /zeroclaw-data/workspace/IDENTITY.md
COPY docker/bravo/workspace/MEMORY.md /zeroclaw-data/workspace/MEMORY.md

ENV ZEROCLAW_GATEWAY_PORT=42617
EXPOSE 42617

ENTRYPOINT ["/bravo/busybox", "sh", "/bravo/entrypoint.sh"]
CMD ["zeroclaw", "gateway", "start", "--host", "0.0.0.0"]
