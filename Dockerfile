# ── Stage 1: Build web UI ────────────────────────────────────────────────────
FROM node:22-alpine AS web-builder
WORKDIR /src/apps/web
COPY apps/web/package.json apps/web/package-lock.json ./
RUN npm ci
COPY apps/web/ ./

# The web UI imports from the shared @gokapi/ui package via workspace link.
COPY packages/ /src/packages/
RUN npm run build

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
