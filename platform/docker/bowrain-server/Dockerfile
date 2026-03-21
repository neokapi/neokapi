# Bowrain API server — self-contained multi-stage build.
# Shares Go module cache layers with bowrain-worker via GHA cache.
#
# Build: docker build -f platform/docker/bowrain-server/Dockerfile .

# ── Stage 1: Build server binary ─────────────────────────────────────────
FROM golang:1.26-alpine AS builder
RUN apk add --no-cache git
WORKDIR /src

# Cache module downloads (identical to worker — shared cache layer).
COPY go.work ./
COPY framework/go.mod framework/go.sum ./framework/
COPY framework/cli/go.mod framework/cli/go.sum ./framework/cli/
COPY framework/kapi/go.mod framework/kapi/go.sum ./framework/kapi/
COPY platform/go.mod platform/go.sum ./platform/
COPY platform/core/go.mod platform/core/go.sum ./platform/core/
COPY platform/cli/go.mod platform/cli/go.sum ./platform/cli/
RUN cd framework && go mod download \
    && cd cli && go mod download \
    && cd ../kapi && go mod download \
    && cd ../../platform && go mod download \
    && cd core && go mod download \
    && cd ../cli && go mod download

# Copy source.
COPY . .

# Build only the server binary (version injected via build args).
ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_DATE=unknown
RUN CGO_ENABLED=0 cd platform && go build \
    -ldflags="-s -w -X github.com/neokapi/neokapi/core/version.Version=${VERSION} -X github.com/neokapi/neokapi/core/version.Commit=${COMMIT} -X github.com/neokapi/neokapi/core/version.BuildDate=${BUILD_DATE}" \
    -o /out/bowrain-server ./cmd/bowrain-server

# ── Stage 2: Minimal runtime ─────────────────────────────────────────────
FROM alpine:3.21
RUN apk add --no-cache ca-certificates tzdata git

COPY --from=builder /out/bowrain-server /usr/local/bin/bowrain-server

VOLUME /data
ENV BOWRAIN_STORE=/data/bowrain.db

EXPOSE 8080
ENTRYPOINT ["bowrain-server"]
