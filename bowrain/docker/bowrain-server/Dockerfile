# Bowrain API server — self-contained multi-stage build.
# Shares Go module cache layers with bowrain-worker via GHA cache.
#
# Build: docker build -f bowrain/docker/bowrain-server/Dockerfile .

# ── Stage 1: Build server binary ─────────────────────────────────────────
FROM golang:1.26-alpine AS builder
RUN apk add --no-cache git
WORKDIR /src

# Cache module downloads (identical to worker — shared cache layer).
COPY go.work go.mod go.sum ./
COPY cli/go.mod cli/go.sum ./cli/
COPY kapi/go.mod kapi/go.sum ./kapi/
COPY apps/kapi-desktop/go.mod apps/kapi-desktop/go.sum ./apps/kapi-desktop/
COPY bowrain/go.mod bowrain/go.sum ./bowrain/
COPY bowrain/core/go.mod bowrain/core/go.sum ./bowrain/core/
COPY bowrain/cli/go.mod bowrain/cli/go.sum ./bowrain/cli/
COPY bowrain/plugin/go.mod bowrain/plugin/go.sum ./bowrain/plugin/
COPY bowrain/plugin/schema/go.mod bowrain/plugin/schema/go.sum ./bowrain/plugin/schema/
# GOWORK=off: download per-module deps without the workspace. go.work lists
# scripts/* modules whose go.mod isn't copied into this layer, so a workspace
# `go mod download` would fail loading them (#610). The build step below keeps
# the workspace (all modules present after `COPY . .`).
RUN export GOWORK=off \
    && go mod download \
    && cd cli && go mod download \
    && cd ../kapi && go mod download \
    && cd ../bowrain && go mod download \
    && cd core && go mod download \
    && cd ../cli && go mod download

# Copy source.
COPY . .

# Build only the server binary (version injected via build args).
ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_DATE=unknown
RUN CGO_ENABLED=0 cd bowrain && go build \
    -ldflags="-s -w -X github.com/neokapi/neokapi/core/version.Version=${VERSION} -X github.com/neokapi/neokapi/core/version.Commit=${COMMIT} -X github.com/neokapi/neokapi/core/version.BuildDate=${BUILD_DATE}" \
    -o /out/bowrain-server ./cmd/bowrain-server

# ── Stage 2: Minimal runtime ─────────────────────────────────────────────
FROM alpine:3.23
RUN apk add --no-cache ca-certificates tzdata git

COPY --from=builder /out/bowrain-server /usr/local/bin/bowrain-server

# /data holds local blob storage (sync push payloads). The server requires a
# PostgreSQL store via BOWRAIN_DATABASE_URL — there is no SQLite/file backend.
VOLUME /data
ENV BLOB_STORAGE_LOCAL_DIR=/data/blobs

EXPOSE 8080
ENTRYPOINT ["bowrain-server"]
