# Bowrain async worker — self-contained multi-stage build.
# Shares Go module cache layers with bowrain-server via GHA cache.
#
# Build: docker build -f docker/bowrain-worker/Dockerfile .

# ── Stage 1: Build worker binary ─────────────────────────────────────────
FROM golang:1.26-alpine AS builder
RUN apk add --no-cache git
WORKDIR /src

# Cache module downloads (identical to server — shared cache layer).
COPY go.mod go.sum go.work ./
COPY platform/go.mod platform/go.sum ./platform/
COPY kapi/go.mod kapi/go.sum ./kapi/
COPY bowrain/go.mod bowrain/go.sum ./bowrain/
RUN go mod download && cd platform && go mod download && cd ../bowrain && go mod download

# Copy source.
COPY . .
RUN mkdir -p bowrain/apps/kapi-web/dist && echo placeholder > bowrain/apps/kapi-web/dist/index.html

# Build only the worker binary.
RUN CGO_ENABLED=0 cd bowrain && go build -ldflags="-s -w" -o /out/bowrain-worker ./cmd/bowrain-worker

# ── Stage 2: Minimal runtime ─────────────────────────────────────────────
FROM alpine:3.21
RUN apk add --no-cache ca-certificates tzdata

COPY --from=builder /out/bowrain-worker /usr/local/bin/bowrain-worker

ENTRYPOINT ["bowrain-worker"]
