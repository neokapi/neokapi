# ── Stage 1: Build web UI ────────────────────────────────────────────────────
FROM node:22-alpine AS web-builder
WORKDIR /src

# Build the shared @gokapi/ui package first (TypeScript project reference).
COPY packages/ui/package.json packages/ui/package-lock.json* packages/ui/
RUN cd packages/ui && npm ci
COPY packages/ui/ packages/ui/
RUN cd packages/ui && npx tsc

# Build the web UI.
COPY apps/web/package.json apps/web/package-lock.json apps/web/
RUN cd apps/web && npm ci
COPY apps/web/ apps/web/
RUN cd apps/web && npm run build

# ── Stage 2: Build Go binary ────────────────────────────────────────────────
FROM golang:1.25-alpine AS go-builder
RUN apk add --no-cache git
WORKDIR /src

# Cache module downloads.
COPY go.mod go.sum ./
RUN go mod download

# Copy source (web dist is needed for //go:embed).
COPY . .
COPY --from=web-builder /src/apps/web/dist apps/web/dist
# Create placeholder for kapi-web embed (not used by gokapi-server but needed for compilation).
RUN mkdir -p apps/kapi-web/dist && echo placeholder > apps/kapi-web/dist/index.html

# Build gokapi-server. Pure Go (modernc.org/sqlite), no CGO needed.
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /gokapi-server ./cmd/gokapi-server

# ── Stage 3: Runtime ────────────────────────────────────────────────────────
FROM alpine:3.21
RUN apk add --no-cache ca-certificates tzdata

COPY --from=go-builder /gokapi-server /usr/local/bin/gokapi-server

# Default data directory for SQLite databases.
VOLUME /data
ENV GOKAPI_STORE=/data/gokapi.db

EXPOSE 8080
ENTRYPOINT ["gokapi-server"]
