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
RUN go mod download \
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

VOLUME /data
ENV BOWRAIN_STORE=/data/bowrain.db

EXPOSE 8080
ENTRYPOINT ["bowrain-server"]
