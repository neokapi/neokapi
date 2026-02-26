# ── Stage 1: Build Go binary ────────────────────────────────────────────────
FROM golang:1.26-alpine AS go-builder
RUN apk add --no-cache git
WORKDIR /src

# Cache module downloads (all four modules + workspace).
COPY go.mod go.sum go.work ./
COPY platform/go.mod platform/go.sum ./platform/
COPY kapi/go.mod kapi/go.sum ./kapi/
COPY bowrain/go.mod bowrain/go.sum ./bowrain/
RUN go mod download && cd platform && go mod download && cd ../bowrain && go mod download

# Copy source. Worker doesn't need the web UI, but the bowrain module's
# embed directives require the dist directories to exist for compilation.
COPY . .
RUN mkdir -p bowrain/apps/web/dist && echo placeholder > bowrain/apps/web/dist/index.html
RUN mkdir -p bowrain/apps/kapi-web/dist && echo placeholder > bowrain/apps/kapi-web/dist/index.html

# Build bowrain-worker from bowrain module. Pure Go (modernc.org/sqlite), no CGO needed.
RUN CGO_ENABLED=0 cd bowrain && go build -ldflags="-s -w" -o /bowrain-worker ./cmd/bowrain-worker

# ── Stage 2: Runtime ────────────────────────────────────────────────────────
FROM alpine:3.21
RUN apk add --no-cache ca-certificates tzdata

COPY --from=go-builder /bowrain-worker /usr/local/bin/bowrain-worker

ENTRYPOINT ["bowrain-worker"]
