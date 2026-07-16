# syntax=docker/dockerfile:1
# Runtime image for the kapi CLI, assembled from the release's prebuilt
# binaries — the same cgo + static-ICU builds the archives ship. ICU is linked
# statically but glibc is not, and the binaries are built on ubuntu-24.04
# runners, so the base must carry glibc >= 24.04's; anything older (e.g.
# debian:bookworm) fails at dynamic link time.
#
# Build context layout (staged by the publish-cli-image job in
# .github/workflows/release.yml):
#   bins/<arch>/kapi                                  amd64 + arm64
#   plugins/<arch>/bowrain/{kapi-bowrain,manifest.json}
ARG BASE=ubuntu:24.04

FROM ${BASE} AS kapi
# git: CI jobs commit produced translations back to the repo.
# ca-certificates: TLS to AI/MT providers and the Bowrain server.
# jq + curl: CI components parse `kapi --json` output and call the forge API
# (MR creation, threaded notes) — table stakes for a CI-job image.
RUN apt-get update \
  && apt-get install -y --no-install-recommends ca-certificates git jq curl \
  && rm -rf /var/lib/apt/lists/*
ARG TARGETARCH
COPY bins/${TARGETARCH}/kapi /usr/local/bin/kapi
ENTRYPOINT ["kapi"]
CMD ["--help"]

# Variant with the kapi-bowrain plugin preinstalled in the system plugin root
# (host/pluginhost/discover.go scans /usr/share/kapi/plugins), so
# server-connected projects need zero install steps: the container runs
# `kapi up` / `kapi push` / `kapi pull` out of the box.
FROM kapi AS kapi-bowrain
ARG TARGETARCH
COPY plugins/${TARGETARCH}/bowrain/ /usr/share/kapi/plugins/bowrain/
