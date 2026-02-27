# gokapi Makefile
# ================

# Suppress macOS linker warnings for CGO (Wails WebView)
ifeq ($(shell uname -s),Darwin)
export MACOSX_DEPLOYMENT_TARGET := 10.15
export CGO_CFLAGS := -mmacosx-version-min=10.15
export CGO_LDFLAGS := -mmacosx-version-min=10.15
endif

# Variables
GO          := go
GOTEST      := $(GO) test
GOBUILD     := $(GO) build
GOVET       := $(GO) vet
GOFMT       := gofmt
MODULE      := github.com/gokapi/gokapi
PLATFORM    := github.com/gokapi/gokapi/platform
KAPI_MOD    := github.com/gokapi/gokapi/kapi
BOWRAIN     := github.com/gokapi/gokapi/bowrain
CLI_PKG     := $(KAPI_MOD)/cmd/kapi
SERVER_PKG  := $(BOWRAIN)/cmd/bowrain-server
VERSION     ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT      := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_DATE  := $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
VERSION_PKG := $(MODULE)/core/version
LDFLAGS     := -ldflags "-X $(VERSION_PKG).Version=$(VERSION) -X $(VERSION_PKG).Commit=$(COMMIT) -X $(VERSION_PKG).BuildDate=$(BUILD_DATE)"
BIN_DIR     := bin
COVER_DIR   := coverage
BRIDGE_PROTO_DIR := core/plugin/proto/v2
SERVER_PROTO_DIR := bowrain/proto/v1
CERT_DIR     := docker/traefik/certs
FRONTEND_DIR := bowrain/apps/bowrain/frontend
KAPI_WEB_DIR := bowrain/apps/kapi-web
WEB_DIR      := bowrain/apps/web
KC_THEME_DIR := bowrain/apps/keycloak-theme
WEBSITE_DIR  := website
NPM         := npm

# Tools
GOLANGCI_LINT := $(shell which golangci-lint 2>/dev/null || { test -x "$$(go env GOPATH)/bin/golangci-lint" && echo "$$(go env GOPATH)/bin/golangci-lint"; })
PROTOC        := $(shell which protoc 2>/dev/null)
PROTOC_GEN_GO := $(shell which protoc-gen-go 2>/dev/null)

.PHONY: all build build-brain build-server build-bowrain build-all build-frontend test test-fast test-parallel test-unit test-integration \
        test-bridge-filters fetch-bridge-jar fetch-bridge-testdata test-race test-e2e test-framework test-platform test-kapi lint fmt vet proto clean install cover tools help \
        ui-deps frontend-deps frontend-dev frontend-build \
        kapi-web-deps kapi-web-build web-deps web-build \
        keycloak-theme \
        docker-build docker-push certs \
        storybook storybook-dev storybook-build \
        screenshots recordings cli-recordings docs-assets fetch-docs-assets \
        docs-deps docs-dev docs-build docs-serve

# ── General ──────────────────────────────────────────────────────────────────

all: frontend-build kapi-web-build web-build fmt vet lint test build ## Build and validate everything

help: ## Show this help
	@awk '/^# ── / { \
		gsub(/# ── /, ""); gsub(/ ─+$$/, ""); category = $$0; next \
	} \
	/^[a-zA-Z_-]+:.*## / { \
		match($$0, /## (.*)/); desc = substr($$0, RSTART+3); \
		match($$0, /^[a-zA-Z_-]+/); target = substr($$0, RSTART, RLENGTH); \
		targets[++n] = target; descs[n] = desc; cats[n] = category \
	} \
	END { \
		cur = ""; \
		for (i = 1; i <= n; i++) { \
			if (cats[i] != cur) { cur = cats[i]; printf "\n\033[1m%s\033[0m\n", cur } \
			printf "  \033[36m%-20s\033[0m %s\n", targets[i], descs[i] \
		} \
		printf "\n" \
	}' $(MAKEFILE_LIST)

# ── Build ────────────────────────────────────────────────────────────────────

build: ## Build the kapi CLI
	@mkdir -p $(BIN_DIR)
	cd kapi && $(GOBUILD) $(LDFLAGS) -o ../$(BIN_DIR)/kapi ./cmd/kapi

build-server: web-build ## Build the Bowrain REST server
	@mkdir -p $(BIN_DIR)
	cd bowrain && $(GOBUILD) $(LDFLAGS) -o ../$(BIN_DIR)/bowrain-server ./cmd/bowrain-server

build-bowrain: frontend-build ## Build the Bowrain desktop app
	cd bowrain/apps/bowrain && wails3 build -ldflags "-X $(VERSION_PKG).Version=$(VERSION) -X $(VERSION_PKG).Commit=$(COMMIT) -X $(VERSION_PKG).BuildDate=$(BUILD_DATE)"

build-brain: ## Build brain CLI
	@mkdir -p $(BIN_DIR)
	cd bowrain && $(GOBUILD) $(LDFLAGS) -o ../$(BIN_DIR)/brain ./cmd/brain

build-all: build build-brain build-server ## Build all Go binaries

install: ## Install kapi CLI to GOPATH/bin
	cd kapi && $(GO) install $(LDFLAGS) ./cmd/kapi

install-brain: ## Install brain CLI to GOPATH/bin
	cd bowrain && $(GO) install $(LDFLAGS) ./cmd/brain

# ── Frontend (Bowrain UI) ───────────────────────────────────────────────────

frontend-deps: ## Install frontend dependencies
	cd $(FRONTEND_DIR) && $(NPM) install

frontend-dev: ## Start frontend dev server
	@printf '\033]0;🍦 Bowrain Frontend\007'
	cd $(FRONTEND_DIR) && $(NPM) run dev

frontend-build: ui-build frontend-deps ## Build frontend for production
	cd $(FRONTEND_DIR) && $(NPM) run build

build-ui: build-server frontend-build ## Build server + frontend

# ── Shared UI Package ──────────────────────────────────────────────────────

ui-deps: ## Install shared UI package dependencies
	cd packages/ui && $(NPM) install

ui-build: ui-deps ## Build shared UI declarations
	cd packages/ui && npx tsc

storybook: ui-deps ## Launch Storybook dev server (port 6006)
	@printf '\033]0;🍦 Storybook\007'
	cd packages/ui && $(NPM) run storybook

storybook-dev: storybook ## Alias for storybook

storybook-build: ui-deps ## Build Storybook static site → packages/ui/storybook-static/
	cd packages/ui && $(NPM) run storybook:build

# ── Kapi Web UI (kapi serve) ───────────────────────────────────────────────

kapi-web-deps: ## Install kapi web UI dependencies
	cd $(KAPI_WEB_DIR) && $(NPM) install

kapi-web-build: ui-build kapi-web-deps ## Build kapi web UI for production
	cd $(KAPI_WEB_DIR) && $(NPM) run build

# ── SaaS Web UI (bowrain-server) ───────────────────────────────────────────

web-deps: ## Install SaaS web UI dependencies
	cd $(WEB_DIR) && $(NPM) install

web-build: ui-build web-deps ## Build SaaS web UI for production
	cd $(WEB_DIR) && $(NPM) run build

# ── Keycloak Theme ─────────────────────────────────────────────────────────

keycloak-theme: ui-deps ## Build Keycloak login theme JAR
	cd $(KC_THEME_DIR) && $(NPM) ci && $(NPM) run build-keycloak-theme

# ── Docker ──────────────────────────────────────────────────────────────────

DOCKER_IMAGE := ghcr.io/gokapi/bowrain-server

docker-build: ## Build Docker image for bowrain-server
	docker build -t $(DOCKER_IMAGE):$(VERSION) -t $(DOCKER_IMAGE):latest .

docker-push: ## Push Docker image to GHCR
	docker push $(DOCKER_IMAGE):$(VERSION)
	docker push $(DOCKER_IMAGE):latest

dev-deps: ## Start dev dependencies (Traefik + Keycloak + Mailpit) in Docker
	@printf '\033]0;🍦 Dev Dependencies\007'
	docker compose up -d --wait

dev-deps-down: ## Stop dev dependencies
	docker compose down -v

bowrain-dev: ## Launch Bowrain desktop app in dev mode (hot reload)
	@printf '\033]0;🍦 Bowrain Desktop\007'
	cd bowrain/apps/bowrain && wails3 dev

certs: ## Generate mkcert TLS certificates for *.bowrain.mymac
	@mkdir -p $(CERT_DIR)
	mkcert -cert-file $(CERT_DIR)/wildcard.pem -key-file $(CERT_DIR)/wildcard-key.pem \
		"*.bowrain.mymac" "bowrain.mymac"

dev-server: ## Run bowrain-server locally (no UI build; use dev-web for HMR)
	@printf '\033]0;🍦 Bowrain Server\007'
	@mkdir -p $(BIN_DIR)
	cd bowrain && $(GOBUILD) $(LDFLAGS) -o ../$(BIN_DIR)/bowrain-server ./cmd/bowrain-server
	BOWRAIN_JWT_SECRET=dev-secret-change-in-production \
	BOWRAIN_OIDC_ISSUER_URL=https://auth.bowrain.mymac/realms/bowrain \
	BOWRAIN_OIDC_CLIENT_ID=bowrain \
	BOWRAIN_OIDC_CLIENT_SECRET=bowrain-secret \
	BOWRAIN_SMTP_HOST=localhost:1025 \
	BOWRAIN_SMTP_FROM=noreply@bowrain.cloud \
	BOWRAIN_STORE=bowrain-dev.db \
	bin/bowrain-server

dev-web: ## Run web UI dev server with HMR (proxy → localhost:8080)
	@printf '\033]0;🍦 Web UI\007'
	cd bowrain/apps/web && $(NPM) run dev

# ── Documentation Assets (Screenshots & Recordings) ─────────────────────────

screenshots: frontend-deps ## Generate documentation screenshots
	cd $(FRONTEND_DIR) && $(NPM) run screenshots

recordings: frontend-deps ## Generate Bowrain (GUI) video recordings
	cd $(FRONTEND_DIR) && $(NPM) run recordings:all

cli-recordings: build ## Generate CLI demo videos using VHS
	./website/tapes/generate.sh

docs-assets: screenshots recordings cli-recordings ## Generate all documentation assets

fetch-docs-assets: ## Download pre-built docs assets from the docs-assets GitHub release
	@echo "Fetching docs assets from GitHub release..."
	@gh release download docs-assets --pattern 'docs-assets.tar.gz' --dir /tmp --clobber
	@mkdir -p website/static
	@tar xzf /tmp/docs-assets.tar.gz -C website/static
	@rm -f /tmp/docs-assets.tar.gz
	@echo "Done. Assets extracted to website/static/"
	@du -sh website/static/img website/static/video 2>/dev/null || true

# ── Test ─────────────────────────────────────────────────────────────────────

test: ## Run all tests (all four modules)
	$(GOTEST) ./... -count=1
	cd platform && $(GOTEST) ./... -count=1
	cd kapi && $(GOTEST) ./... -count=1
	cd bowrain && $(GOTEST) ./... -count=1

test-fast: ## Run all tests with caching (fast local iteration)
	$(GOTEST) ./...
	cd platform && $(GOTEST) ./...
	cd kapi && $(GOTEST) ./...
	cd bowrain && $(GOTEST) ./...

test-parallel: ## Run all tests in parallel (all four modules concurrently)
	@$(GOTEST) ./... -count=1 & \
	(cd platform && $(GOTEST) ./... -count=1) & \
	(cd kapi && $(GOTEST) ./... -count=1) & \
	(cd bowrain && $(GOTEST) ./... -count=1) & \
	wait

test-unit: ## Run unit tests only (exclude integration)
	$(GOTEST) ./... -count=1 -short
	cd platform && $(GOTEST) ./... -count=1 -short
	cd kapi && $(GOTEST) ./... -count=1 -short
	cd bowrain && $(GOTEST) ./... -count=1 -short

test-race: ## Run tests with race detector
	$(GOTEST) ./... -count=1 -race
	cd platform && $(GOTEST) ./... -count=1 -race
	cd kapi && $(GOTEST) ./... -count=1 -race
	cd bowrain && $(GOTEST) ./... -count=1 -race

test-framework: ## Run framework tests only
	$(GOTEST) ./... -count=1

test-platform: ## Run platform module tests only
	cd platform && $(GOTEST) ./... -count=1

test-kapi: ## Run kapi CLI tests only
	cd kapi && $(GOTEST) ./... -count=1

test-bowrain: ## Run bowrain tests only
	cd bowrain && $(GOTEST) ./... -count=1

test-integration: ## Run integration tests
	$(GOTEST) ./... -count=1 -tags=integration -run Integration
	cd bowrain && $(GOTEST) ./... -count=1 -tags=integration -run Integration

GITHUB_TOKEN         ?= $(shell gh auth token 2>/dev/null)
OKAPI_BRIDGE_VERSION ?= v2.0.0
OKAPI_VERSION        ?= 1.48.0
BRIDGE_JAR           := $(HOME)/.cache/gokapi/bridge/$(OKAPI_BRIDGE_VERSION)-okapi$(OKAPI_VERSION)/okapi-bridge.jar

fetch-bridge-jar: ## Download okapi-bridge JAR from GitHub release
	@GITHUB_TOKEN=$(GITHUB_TOKEN) bash scripts/fetch-okapi-bridge.sh

fetch-bridge-testdata: ## Download okapi test data from GitHub release
	@GITHUB_TOKEN=$(GITHUB_TOKEN) bash scripts/fetch-okapi-testdata.sh

test-bridge-filters: fetch-bridge-jar fetch-bridge-testdata ## Run bridge filter integration tests (requires Java)
	GOKAPI_BRIDGE_JAR=$(BRIDGE_JAR) $(GOTEST) -tags=integration -count=1 -v ./core/plugin/bridge/filters/...

test-e2e: ## Run end-to-end tests against Docker stack
	bash e2e/run.sh

test-verbose: ## Run tests with verbose output
	$(GOTEST) ./... -count=1 -v
	cd platform && $(GOTEST) ./... -count=1 -v
	cd kapi && $(GOTEST) ./... -count=1 -v
	cd bowrain && $(GOTEST) ./... -count=1 -v

cover: ## Run tests with coverage
	@mkdir -p $(COVER_DIR)
	$(GOTEST) ./... -count=1 -coverprofile=$(COVER_DIR)/framework.out -covermode=atomic
	cd platform && $(GOTEST) ./... -count=1 -coverprofile=../$(COVER_DIR)/platform.out -covermode=atomic
	cd kapi && $(GOTEST) ./... -count=1 -coverprofile=../$(COVER_DIR)/kapi.out -covermode=atomic
	cd bowrain && $(GOTEST) ./... -count=1 -coverprofile=../$(COVER_DIR)/bowrain.out -covermode=atomic
	cat $(COVER_DIR)/framework.out > $(COVER_DIR)/coverage.out
	tail -n +2 $(COVER_DIR)/platform.out >> $(COVER_DIR)/coverage.out
	tail -n +2 $(COVER_DIR)/kapi.out >> $(COVER_DIR)/coverage.out
	tail -n +2 $(COVER_DIR)/bowrain.out >> $(COVER_DIR)/coverage.out
	$(GO) tool cover -html=$(COVER_DIR)/coverage.out -o $(COVER_DIR)/coverage.html
	@echo "Coverage report: $(COVER_DIR)/coverage.html"

# ── Code Quality ─────────────────────────────────────────────────────────────

fmt: ## Format Go source files
	$(GOFMT) -w -s .

vet: ## Run go vet
	$(GOVET) ./...
	cd platform && $(GOVET) ./...
	cd kapi && $(GOVET) ./...
	cd bowrain && $(GOVET) ./...

lint: ## Run golangci-lint
ifdef GOLANGCI_LINT
	$(GOLANGCI_LINT) run ./...
	cd platform && $(GOLANGCI_LINT) run ./...
	cd kapi && $(GOLANGCI_LINT) run ./...
	cd bowrain && $(GOLANGCI_LINT) run ./...
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
		$(BRIDGE_PROTO_DIR)/*.proto
	protoc --go_out=. --go_opt=paths=source_relative \
		--go-grpc_out=. --go-grpc_opt=paths=source_relative \
		$(SERVER_PROTO_DIR)/*.proto

# ── Tools ────────────────────────────────────────────────────────────────────

setup-remote: ## Install dependencies for Claude Code cloud environments
	CLAUDE_CODE_REMOTE=true bash scripts/setup-remote.sh

tools: ## Install development tools
	$(GO) install google.golang.org/protobuf/cmd/protoc-gen-go@latest
	$(GO) install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
	$(GO) install github.com/golangci/golangci-lint/cmd/golangci-lint@latest

# ── Clean ────────────────────────────────────────────────────────────────────

clean: ## Remove build artifacts
	rm -rf $(BIN_DIR)
	rm -rf $(COVER_DIR)
	rm -rf packages/ui/storybook-static
	rm -rf packages/ui/node_modules
	rm -rf $(FRONTEND_DIR)/dist
	rm -rf $(FRONTEND_DIR)/node_modules
	rm -rf $(KAPI_WEB_DIR)/dist
	rm -rf $(KAPI_WEB_DIR)/node_modules
	rm -rf $(WEB_DIR)/dist
	rm -rf $(WEB_DIR)/node_modules
	rm -rf $(KC_THEME_DIR)/dist_keycloak
	rm -rf $(KC_THEME_DIR)/node_modules
	$(GO) clean -cache -testcache

# ── Dependencies ─────────────────────────────────────────────────────────────

deps: ## Download and tidy dependencies
	$(GO) mod download && $(GO) mod tidy
	cd platform && $(GO) mod download && $(GO) mod tidy
	cd kapi && $(GO) mod download && $(GO) mod tidy
	cd bowrain && $(GO) mod download && $(GO) mod tidy

deps-update: ## Update all dependencies
	$(GO) get -u ./...
	$(GO) mod tidy
	cd platform && $(GO) get -u ./...
	cd platform && $(GO) mod tidy
	cd kapi && $(GO) get -u ./...
	cd kapi && $(GO) mod tidy
	cd bowrain && $(GO) get -u ./...
	cd bowrain && $(GO) mod tidy

# ── Documentation Site ──────────────────────────────────────────────────────

docs-deps: ## Install docs site dependencies
	cd $(WEBSITE_DIR) && $(NPM) ci

docs-dev: ## Start docs dev server
	@printf '\033]0;🍦 Docs\007'
	cd $(WEBSITE_DIR) && $(NPM) start

docs-build: ## Build docs for production
	cd $(WEBSITE_DIR) && $(NPM) run build

docs-serve: ## Serve built docs locally
	cd $(WEBSITE_DIR) && $(NPM) run serve
