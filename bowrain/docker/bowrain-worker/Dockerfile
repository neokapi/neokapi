# Bowrain async worker — self-contained multi-stage build.
# Shares Go module cache layers with bowrain-server via GHA cache.
#
# Build: docker build -f bowrain/docker/bowrain-worker/Dockerfile .

# ── Stage 1: Build worker binary ─────────────────────────────────────────
FROM golang:1.27-alpine AS builder
RUN apk add --no-cache git
WORKDIR /src

# Cache module downloads (identical to server — shared cache layer).
COPY go.work go.mod go.sum ./
COPY host/go.mod host/go.sum ./host/
COPY cli/go.mod cli/go.sum ./cli/
COPY kapi/go.mod kapi/go.sum ./kapi/
COPY apps/kapi-desktop/go.mod apps/kapi-desktop/go.sum ./apps/kapi-desktop/
COPY bowrain/go.mod bowrain/go.sum ./bowrain/
COPY bowrain/core/go.mod bowrain/core/go.sum ./bowrain/core/
COPY bowrain/plugin/go.mod bowrain/plugin/go.sum ./bowrain/plugin/
# GOWORK=off: download per-module deps without the workspace. go.work lists
# scripts/* modules whose go.mod isn't copied into this layer, so a workspace
# `go mod download` would fail loading them (#610). The build step below keeps
# the workspace (all modules present after `COPY . .`).
RUN export GOWORK=off \
    && go mod download \
    && cd host && go mod download \
    && cd ../cli && go mod download \
    && cd ../kapi && go mod download \
    && cd ../bowrain && go mod download \
    && cd core && go mod download \
    && cd ../plugin && go mod download

# Copy source. The Go build cache below is a cache mount, not a layer: a
# context that differs from the last build (another worktree, one edited file)
# recompiles only what changed, and no layer holds the cache. The modules stay
# in the layer above, which the CI image builds restore from their layer cache.
COPY . .

# The gettext catalogs core/i18n and host/i18n embed are build output:
# version control carries the translated JSON, and this compiles it into
# the `catalogs/*.mo` the `//go:embed` pattern below resolves.
RUN --mount=type=cache,target=/root/.cache/go-build \
    go run ./core/i18n/gen/catalogs

# Build only the worker binary (version injected via build args).
ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_DATE=unknown
RUN --mount=type=cache,target=/root/.cache/go-build \
    cd bowrain && CGO_ENABLED=0 go build -trimpath \
    -ldflags="-s -w -X github.com/neokapi/neokapi/core/version.Version=${VERSION} -X github.com/neokapi/neokapi/core/version.Commit=${COMMIT} -X github.com/neokapi/neokapi/core/version.BuildDate=${BUILD_DATE}" \
    -o /out/bowrain-worker ./cmd/bowrain-worker

# ── Stage 2: Minimal runtime ─────────────────────────────────────────────
FROM alpine:3.24
# git: brand-scan repo sources shell out to the git binary (bowrain/connector).
RUN apk add --no-cache ca-certificates tzdata git
# The process runs as an unprivileged user with a fixed id, so a deployment
# can name it (and own its volumes) without reading /etc/passwd. The passwd
# entry exists for tools that look the user up, such as git.
RUN addgroup -S -g 10001 bowrain \
    && adduser -S -D -H -u 10001 -G bowrain -h /tmp -s /sbin/nologin bowrain

COPY --from=builder /out/bowrain-worker /usr/local/bin/bowrain-worker

# The worker writes only these directories, and checks at start-up that each is
# writable (bowrain/runtimedirs). Everything else can be a read-only root
# filesystem.
#   TMPDIR                  git clones and per-job scratch files. A tmpfs or a
#                           writable volume.
#   BLOB_STORAGE_LOCAL_DIR  blob storage, only when S3 is not configured
#                           (default $TMPDIR/bowrain-blobs).
# /data is owned by the worker's user, so a volume the worker and the server
# share there, whichever of them creates it, is too.
RUN mkdir -p /data && chown 10001:10001 /data
# /tmp is a VOLUME so that a platform mounting a writable volume there, as
# ECS does on Fargate for a read-only root filesystem, copies its mode (1777)
# instead of creating a directory only root can write.
RUN chmod 1777 /tmp
VOLUME /tmp
ENV TMPDIR=/tmp HOME=/tmp

USER 10001:10001
ENTRYPOINT ["bowrain-worker"]
