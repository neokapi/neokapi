# gokapi Makefile
# ================

# Variables
GO          := go
GOTEST      := $(GO) test
GOBUILD     := $(GO) build
GOVET       := $(GO) vet
GOFMT       := gofmt
MODULE      := github.com/gokapi/gokapi
CLI_PKG     := $(MODULE)/cmd/kapi
SERVER_PKG  := $(MODULE)/cmd/bowrain-server
VERSION     ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT      := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_DATE  := $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
VERSION_PKG := $(MODULE)/core/version
LDFLAGS     := -ldflags "-X $(VERSION_PKG).Version=$(VERSION) -X $(VERSION_PKG).Commit=$(COMMIT) -X $(VERSION_PKG).BuildDate=$(BUILD_DATE)"
BIN_DIR     := bin
COVER_DIR   := coverage
PROTO_DIR   := plugin/proto/v1
PROTO_FILES := $(wildcard $(PROTO_DIR)/*.proto)
FRONTEND_DIR := apps/bowrain/frontend
KAPI_WEB_DIR := apps/kapi-web
WEB_DIR      := apps/web
WEBSITE_DIR  := website
NPM         := npm

# Tools
GOLANGCI_LINT := $(shell which golangci-lint 2>/dev/null || { test -x "$$(go env GOPATH)/bin/golangci-lint" && echo "$$(go env GOPATH)/bin/golangci-lint"; })
PROTOC        := $(shell which protoc 2>/dev/null)
PROTOC_GEN_GO := $(shell which protoc-gen-go 2>/dev/null)

.PHONY: all build build-server build-bowrain build-all build-frontend test test-unit test-integration \
        test-race test-e2e lint fmt vet proto clean install cover tools help \
        ui-deps frontend-deps frontend-dev frontend-build \
        kapi-web-deps kapi-web-build web-deps web-build \
        docker-build docker-push \
        screenshots recordings cli-recordings docs-assets \
        docs-deps docs-dev docs-build docs-serve

# Default target
all: frontend-build kapi-web-build web-build fmt vet lint test build ## Build and validate everything

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'

# ── Build ────────────────────────────────────────────────────────────────────

build: kapi-web-build ## Build the kapi CLI
	@mkdir -p $(BIN_DIR)
	$(GOBUILD) $(LDFLAGS) -o $(BIN_DIR)/kapi $(CLI_PKG)

build-server: web-build ## Build the Bowrain REST server
	@mkdir -p $(BIN_DIR)
	$(GOBUILD) $(LDFLAGS) -o $(BIN_DIR)/bowrain-server $(SERVER_PKG)

build-bowrain: frontend-build ## Build the Bowrain desktop app
	cd apps/bowrain && wails3 build -ldflags "-X $(VERSION_PKG).Version=$(VERSION) -X $(VERSION_PKG).Commit=$(COMMIT) -X $(VERSION_PKG).BuildDate=$(BUILD_DATE)"

build-all: build build-server ## Build all Go binaries

install: ## Install kapi CLI to GOPATH/bin
	$(GO) install $(LDFLAGS) $(CLI_PKG)

# ── Frontend (Bowrain UI) ───────────────────────────────────────────────────

frontend-deps: ## Install frontend dependencies
	cd $(FRONTEND_DIR) && $(NPM) install

frontend-dev: ## Start frontend dev server
	cd $(FRONTEND_DIR) && $(NPM) run dev

frontend-build: ui-deps frontend-deps ## Build frontend for production
	cd $(FRONTEND_DIR) && $(NPM) run build

build-ui: build-server frontend-build ## Build server + frontend

# ── Shared UI Package ──────────────────────────────────────────────────────

ui-deps: ## Install shared UI package dependencies
	cd packages/ui && $(NPM) install

# ── Kapi Web UI (kapi serve) ───────────────────────────────────────────────

kapi-web-deps: ## Install kapi web UI dependencies
	cd $(KAPI_WEB_DIR) && $(NPM) install

kapi-web-build: ui-deps kapi-web-deps ## Build kapi web UI for production
	cd packages/ui && npx tsc
	cd $(KAPI_WEB_DIR) && $(NPM) run build

# ── SaaS Web UI (bowrain-server) ───────────────────────────────────────────

web-deps: ## Install SaaS web UI dependencies
	cd $(WEB_DIR) && $(NPM) install

web-build: ui-deps web-deps ## Build SaaS web UI for production
	cd packages/ui && npx tsc
	cd $(WEB_DIR) && $(NPM) run build

# ── Docker ──────────────────────────────────────────────────────────────────

DOCKER_IMAGE := ghcr.io/gokapi/bowrain-server

docker-build: ## Build Docker image for bowrain-server
	docker build -t $(DOCKER_IMAGE):$(VERSION) -t $(DOCKER_IMAGE):latest .

docker-push: ## Push Docker image to GHCR
	docker push $(DOCKER_IMAGE):$(VERSION)
	docker push $(DOCKER_IMAGE):latest

# ── Documentation Assets (Screenshots & Recordings) ─────────────────────────

screenshots: frontend-deps ## Generate documentation screenshots
	cd $(FRONTEND_DIR) && $(NPM) run screenshots

recordings: frontend-deps ## Generate Bowrain (GUI) video recordings
	cd $(FRONTEND_DIR) && $(NPM) run recordings:all

cli-recordings: build ## Generate CLI demo videos using VHS
	./website/tapes/generate.sh

docs-assets: screenshots recordings cli-recordings ## Generate all documentation assets

# ── Test ─────────────────────────────────────────────────────────────────────

test: ## Run all tests
	$(GOTEST) ./... -count=1

test-unit: ## Run unit tests only (exclude integration)
	$(GOTEST) ./... -count=1 -short

test-race: ## Run tests with race detector
	$(GOTEST) ./... -count=1 -race

test-integration: ## Run integration tests
	$(GOTEST) ./... -count=1 -tags=integration -run Integration

test-e2e: ## Run end-to-end tests against Docker stack
	bash e2e/run.sh

test-verbose: ## Run tests with verbose output
	$(GOTEST) ./... -count=1 -v

cover: ## Run tests with coverage
	@mkdir -p $(COVER_DIR)
	$(GOTEST) ./... -count=1 -coverprofile=$(COVER_DIR)/coverage.out -covermode=atomic
	$(GO) tool cover -html=$(COVER_DIR)/coverage.out -o $(COVER_DIR)/coverage.html
	@echo "Coverage report: $(COVER_DIR)/coverage.html"

# ── Code Quality ─────────────────────────────────────────────────────────────

fmt: ## Format Go source files
	$(GOFMT) -w -s .

vet: ## Run go vet
	$(GOVET) ./...

lint: ## Run golangci-lint
ifdef GOLANGCI_LINT
	$(GOLANGCI_LINT) run ./...
else
	@echo "golangci-lint not installed. Run 'make tools' to install."
endif

check: fmt vet lint ## Run all code quality checks

# ── Protobuf ─────────────────────────────────────────────────────────────────

proto: ## Generate Go code from protobuf definitions
ifndef PROTOC
	$(error "protoc not found. Install Protocol Buffers compiler.")
endif
ifndef PROTOC_GEN_GO
	$(error "protoc-gen-go not found. Run 'make tools' to install.")
endif
	protoc --go_out=. --go_opt=paths=source_relative \
		--go-grpc_out=. --go-grpc_opt=paths=source_relative \
		$(PROTO_DIR)/*.proto

# ── Tools ────────────────────────────────────────────────────────────────────

tools: ## Install development tools
	$(GO) install google.golang.org/protobuf/cmd/protoc-gen-go@latest
	$(GO) install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
	$(GO) install github.com/golangci/golangci-lint/cmd/golangci-lint@latest

# ── Clean ────────────────────────────────────────────────────────────────────

clean: ## Remove build artifacts
	rm -rf $(BIN_DIR)
	rm -rf $(COVER_DIR)
	rm -rf packages/ui/node_modules
	rm -rf $(FRONTEND_DIR)/dist
	rm -rf $(FRONTEND_DIR)/node_modules
	rm -rf $(KAPI_WEB_DIR)/dist
	rm -rf $(KAPI_WEB_DIR)/node_modules
	rm -rf $(WEB_DIR)/dist
	rm -rf $(WEB_DIR)/node_modules
	$(GO) clean -cache -testcache

# ── Dependencies ─────────────────────────────────────────────────────────────

deps: ## Download and tidy dependencies
	$(GO) mod download
	$(GO) mod tidy

deps-update: ## Update all dependencies
	$(GO) get -u ./...
	$(GO) mod tidy

# ── Documentation Site ──────────────────────────────────────────────────────

docs-deps: ## Install docs site dependencies
	cd $(WEBSITE_DIR) && $(NPM) ci

docs-dev: ## Start docs dev server
	cd $(WEBSITE_DIR) && $(NPM) start

docs-build: ## Build docs for production
	cd $(WEBSITE_DIR) && $(NPM) run build

docs-serve: ## Serve built docs locally
	cd $(WEBSITE_DIR) && $(NPM) run serve
