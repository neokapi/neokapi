# Agentic Testing server — self-contained multi-stage build.
#
# Build: docker build -f bowrain/docker/agentic-testing/Dockerfile .

# ── Stage 1: Build server binary ─────────────────────────────────────────
FROM golang:1.26-alpine AS builder
RUN apk add --no-cache git
WORKDIR /src

# Cache module downloads (shared layer structure with bowrain-server).
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
    && cd ../apps/kapi-desktop && go mod download \
    && cd ../../bowrain && go mod download \
    && cd core && go mod download \
    && cd ../cli && go mod download

# Copy source.
COPY . .

# Build only the agentic-server binary.
ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_DATE=unknown
RUN CGO_ENABLED=0 cd bowrain && go build \
    -ldflags="-s -w -X github.com/neokapi/neokapi/core/version.Version=${VERSION} -X github.com/neokapi/neokapi/core/version.Commit=${COMMIT} -X github.com/neokapi/neokapi/core/version.BuildDate=${BUILD_DATE}" \
    -o /out/agentic-server ./agentic-testing/cmd/agentic-server

# ── Stage 2: Minimal runtime ─────────────────────────────────────────────
FROM alpine:3.21
RUN apk add --no-cache ca-certificates tzdata git

COPY --from=builder /out/agentic-server /usr/local/bin/agentic-server

EXPOSE 8080
ENTRYPOINT ["agentic-server"]
