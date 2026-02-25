# ── Stage 1: Build web UI ────────────────────────────────────────────────────
FROM node:22-alpine AS web-builder
WORKDIR /src

# Build the shared @gokapi/ui package first (TypeScript project reference).
COPY packages/ui/package.json packages/ui/package-lock.json* packages/ui/
RUN cd packages/ui && npm ci
COPY packages/ui/ packages/ui/
RUN cd packages/ui && npx tsc

# Build the web UI.
COPY bowrain/apps/web/package.json bowrain/apps/web/package-lock.json bowrain/apps/web/
RUN cd bowrain/apps/web && npm ci
COPY bowrain/apps/web/ bowrain/apps/web/
RUN cd bowrain/apps/web && npm run build

# ── Stage 2: Build Go binary ────────────────────────────────────────────────
FROM golang:1.26-alpine AS go-builder
RUN apk add --no-cache git
WORKDIR /src

# Cache module downloads (all four modules + workspace).
COPY go.mod go.sum go.work ./
COPY platform/go.mod platform/go.sum ./platform/
COPY kapi/go.mod kapi/go.sum ./kapi/
COPY bowrain/go.mod bowrain/go.sum ./bowrain/
RUN go mod download && cd platform && go mod download && cd ../bowrain && go mod download

# Copy source (web dist is needed for //go:embed).
COPY . .
COPY --from=web-builder /src/bowrain/apps/web/dist bowrain/apps/web/dist
# Create placeholder for kapi-web embed (not used by bowrain-server but needed for compilation).
RUN mkdir -p bowrain/apps/kapi-web/dist && echo placeholder > bowrain/apps/kapi-web/dist/index.html

# Build bowrain-server from bowrain module. Pure Go (modernc.org/sqlite), no CGO needed.
RUN CGO_ENABLED=0 cd bowrain && go build -ldflags="-s -w" -o /bowrain-server ./cmd/bowrain-server

# ── Stage 3: Runtime ────────────────────────────────────────────────────────
FROM alpine:3.21
RUN apk add --no-cache ca-certificates tzdata

COPY --from=go-builder /bowrain-server /usr/local/bin/bowrain-server

# Default data directory (SQLite databases when not using PostgreSQL).
VOLUME /data
ENV BOWRAIN_STORE=/data/bowrain.db

# Default mode: api server. Set BOWRAIN_MODE=worker for async job processing.
ENV BOWRAIN_MODE=api

EXPOSE 8080
ENTRYPOINT ["bowrain-server"]
