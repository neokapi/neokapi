# Go builder — compiles bowrain-server and bowrain-worker.
# Built once per commit and used by bowrain-server and bowrain-worker
# images via COPY --from=bowrain-base.
#
# Build: docker build -f docker/bowrain-base/Dockerfile -t bowrain-base .

FROM golang:1.26-alpine AS go-builder
RUN apk add --no-cache git
WORKDIR /src

# Cache module downloads.
COPY go.mod go.sum go.work ./
COPY platform/go.mod platform/go.sum ./platform/
COPY kapi/go.mod kapi/go.sum ./kapi/
COPY bowrain/go.mod bowrain/go.sum ./bowrain/
RUN go mod download && cd platform && go mod download && cd ../bowrain && go mod download

# Copy source.
COPY . .
RUN mkdir -p bowrain/apps/kapi-web/dist && echo placeholder > bowrain/apps/kapi-web/dist/index.html

# Build both binaries. The Go build cache means the second binary
# reuses all compiled packages — only the main package differs.
RUN CGO_ENABLED=0 cd bowrain && go build -ldflags="-s -w" -o /out/bowrain-server ./cmd/bowrain-server
RUN CGO_ENABLED=0 cd bowrain && go build -ldflags="-s -w" -o /out/bowrain-worker ./cmd/bowrain-worker

# Output stage: just the binaries.
FROM scratch
COPY --from=go-builder /out/ /
