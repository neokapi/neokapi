#!/usr/bin/env bash
# Mirror a third-party container image into this organisation's GHCR namespace.
#
#   scripts/mirror-image.sh <source-image> <tag> [<target-name>]
#
# Example:
#   scripts/mirror-image.sh versity/versitygw v1.8.0
#   -> ghcr.io/neokapi/versitygw:v1.8.0
#
# The copy is a one-layer build on top of the source image so it carries the
# org.opencontainers.image.source label that links the package to this
# repository. Pushing needs a login with write:packages
# (`gh auth token | docker login ghcr.io -u <user> --password-stdin`); the
# mirror-images workflow runs this with the workflow token.
#
# Why a mirror at all: a test that runs `docker run docker.io/...` pulls from
# Docker Hub unauthenticated, and a busy CI day exhausts that pull limit, so
# the job fails on the pull before the test runs (#3130). Jobs pull the mirror
# with the workflow token instead.
set -euo pipefail

source_image="${1:?source image, e.g. versity/versitygw}"
tag="${2:?tag, e.g. v1.8.0}"
target_name="${3:-${source_image##*/}}"
org="${GHCR_ORG:-neokapi}"
target="ghcr.io/${org}/${target_name}:${tag}"

ctx="$(mktemp -d)"
trap 'rm -rf "$ctx"' EXIT
cat > "$ctx/Dockerfile" <<EOF
FROM docker.io/${source_image}:${tag}
LABEL org.opencontainers.image.source="https://github.com/${org}/neokapi" \\
      org.opencontainers.image.description="Mirror of docker.io/${source_image}:${tag} for the test suite"
EOF

echo "mirror-image: docker.io/${source_image}:${tag} -> ${target}"
docker build --pull -t "$target" "$ctx"
docker push "$target"
echo "mirror-image: pushed ${target}"
