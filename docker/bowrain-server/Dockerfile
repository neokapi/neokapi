# Bowrain API server — self-contained multi-stage build.
# Shares Go module cache layers with bowrain-worker via GHA cache.
#
# Build: docker build -f docker/bowrain-server/Dockerfile .

# ── Stage 1: Build server binary ─────────────────────────────────────────
FROM golang:1.26-alpine AS builder
RUN apk add --no-cache git
WORKDIR /src

# Cache module downloads (identical to worker — shared cache layer).
COPY go.mod go.sum go.work ./
COPY cli/go.mod cli/go.sum ./cli/
COPY platform/go.mod platform/go.sum ./platform/
COPY kapi/go.mod kapi/go.sum ./kapi/
COPY brain/go.mod brain/go.sum ./brain/
COPY bowrain/go.mod bowrain/go.sum ./bowrain/
RUN go mod download \
    && cd cli && go mod download \
    && cd ../platform && go mod download \
    && cd ../kapi && go mod download \
    && cd ../brain && go mod download \
    && cd ../bowrain && go mod download

# Copy source.
COPY . .
RUN mkdir -p bowrain/apps/kapi-web/dist && echo placeholder > bowrain/apps/kapi-web/dist/index.html

# Build only the server binary.
RUN CGO_ENABLED=0 cd bowrain && go build -ldflags="-s -w" -o /out/bowrain-server ./cmd/bowrain-server

# ── Stage 2: Minimal runtime ─────────────────────────────────────────────
FROM alpine:3.21
RUN apk add --no-cache ca-certificates tzdata

COPY --from=builder /out/bowrain-server /usr/local/bin/bowrain-server

VOLUME /data
ENV BOWRAIN_STORE=/data/bowrain.db

EXPOSE 8080
ENTRYPOINT ["bowrain-server"]
