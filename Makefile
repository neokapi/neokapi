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
CLI_MOD     := github.com/gokapi/gokapi/cli
PLATFORM    := github.com/gokapi/gokapi/platform
KAPI_MOD    := github.com/gokapi/gokapi/kapi
BOWRAIN_CLI := github.com/gokapi/gokapi/bowrain-cli
BOWRAIN     := github.com/gokapi/gokapi/bowrain
CLI_PKG     := $(KAPI_MOD)/cmd/kapi
SERVER_PKG  := $(BOWRAIN)/cmd/bowrain-server
WORKER_PKG  := $(BOWRAIN)/cmd/bowrain-worker
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
KAPI_WEB_DIR := kapi/apps/kapi-web
WEB_DIR      := bowrain/apps/web
KC_THEME_DIR := bowrain/apps/keycloak-theme
WEBSITE_DIR  := website
NPM         := npm

# Tools
GOLANGCI_LINT := $(shell which golangci-lint 2>/dev/null || { test -x "$$(go env GOPATH)/bin/golangci-lint" && echo "$$(go env GOPATH)/bin/golangci-lint"; })
PROTOC        := $(shell which protoc 2>/dev/null)
PROTOC_GEN_GO := $(shell which protoc-gen-go 2>/dev/null)

.PHONY: all build build-bowrain-cli build-brain build-server build-headless build-bowrain build-all build-frontend test test-fast test-parallel test-unit test-integration \
        test-bridge-filters test-bridge-pool fetch-bridge-jar fetch-bridge-testdata test-race test-e2e test-framework test-platform test-cli test-kapi test-bowrain-cli test-brain test-bowrain lint fmt vet proto clean install cover tools help \
        ui-deps frontend-deps frontend-dev frontend-build \
        kapi-web-deps kapi-web-build web-deps web-build \
        keycloak-theme \
        docker-server docker-web docker-keycloak docker-all docker-push-server docker-push-web docker-push-keycloak docker-push certs \
        storybook storybook-dev storybook-build \
        screenshots recordings kapi-recordings bowrain-cli-recordings brain-recordings cli-recordings docs-assets fetch-docs-assets \
        videos-deps videos-setup videos-studio videos-render \
        docs-deps docs-dev docs-build docs-serve \
        test-bridge-json test-native-json generate-test-comparison generate-test-stubs

# ── General ──────────────────────────────────────────────────────────────────

all: help ## Default: show help

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

build-server: ## Build the Bowrain REST server
	@mkdir -p $(BIN_DIR)
	cd bowrain && $(GOBUILD) $(LDFLAGS) -o ../$(BIN_DIR)/bowrain-server ./cmd/bowrain-server

build-worker: ## Build the Bowrain worker
	@mkdir -p $(BIN_DIR)
	cd bowrain && $(GOBUILD) $(LDFLAGS) -o ../$(BIN_DIR)/bowrain-worker ./cmd/bowrain-worker

build-bowrain: frontend-build ## Build the Bowrain desktop app
	cd bowrain/apps/bowrain && wails3 build -ldflags "-X $(VERSION_PKG).Version=$(VERSION) -X $(VERSION_PKG).Commit=$(COMMIT) -X $(VERSION_PKG).BuildDate=$(BUILD_DATE)"

build-bowrain-cli: ## Build Bowrain CLI
	@mkdir -p $(BIN_DIR)
	cd bowrain-cli && $(GOBUILD) $(LDFLAGS) -o ../$(BIN_DIR)/bowrain ./cmd/bowrain
	@ln -sf bowrain $(BIN_DIR)/brain

build-brain: build-bowrain-cli ## Alias for build-bowrain-cli (backward compat)

build-headless: frontend-build ## Build headless desktop binary (server mode, no GUI deps)
	@mkdir -p $(BIN_DIR)
	cd bowrain/apps/bowrain && $(GOBUILD) -tags server $(LDFLAGS) -o ../../../$(BIN_DIR)/bowrain-headless .

build-all: build build-bowrain-cli build-server build-worker ## Build all Go binaries

install: ## Install kapi CLI to GOPATH/bin
	cd kapi && $(GO) install $(LDFLAGS) ./cmd/kapi

install-bowrain-cli: ## Install Bowrain CLI to GOPATH/bin
	cd bowrain-cli && $(GO) install $(LDFLAGS) ./cmd/bowrain

install-brain: install-bowrain-cli ## Alias for install-bowrain-cli (backward compat)

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
	@printf '{"version":"%s","commit":"%s","build_date":"%s","component":"kapi-web"}\n' "$(VERSION)" "$(COMMIT)" "$(BUILD_DATE)" > $(KAPI_WEB_DIR)/public/version.json
	cd $(KAPI_WEB_DIR) && $(NPM) run build

# ── SaaS Web UI (bowrain-server) ───────────────────────────────────────────

web-deps: ## Install SaaS web UI dependencies
	cd $(WEB_DIR) && $(NPM) install

web-build: ui-build web-deps ## Build SaaS web UI for production
	@printf '{"version":"%s","commit":"%s","build_date":"%s","component":"web"}\n' "$(VERSION)" "$(COMMIT)" "$(BUILD_DATE)" > $(WEB_DIR)/public/version.json
	cd $(WEB_DIR) && $(NPM) run build

# ── Keycloak Theme ─────────────────────────────────────────────────────────

keycloak-theme: ui-deps ## Build Keycloak login theme JAR
	cd $(KC_THEME_DIR) && $(NPM) ci && $(NPM) run build-keycloak-theme

# ── Docker ──────────────────────────────────────────────────────────────────

DOCKER_IMAGE          := ghcr.io/gokapi/bowrain-server
DOCKER_WORKER_IMAGE   := ghcr.io/gokapi/bowrain-worker
DOCKER_WEB_IMAGE      := ghcr.io/gokapi/bowrain-web
DOCKER_KEYCLOAK_IMAGE := ghcr.io/gokapi/bowrain-keycloak

docker-server: ## Build server and worker images
	docker build -f docker/bowrain-server/Dockerfile --build-arg VERSION=$(VERSION) --build-arg COMMIT=$(COMMIT) --build-arg BUILD_DATE=$(BUILD_DATE) -t $(DOCKER_IMAGE):$(VERSION) -t $(DOCKER_IMAGE):latest .
	docker build -f docker/bowrain-worker/Dockerfile --build-arg VERSION=$(VERSION) --build-arg COMMIT=$(COMMIT) --build-arg BUILD_DATE=$(BUILD_DATE) -t $(DOCKER_WORKER_IMAGE):$(VERSION) -t $(DOCKER_WORKER_IMAGE):latest .

docker-web: ## Build web UI image
	docker build -f docker/bowrain-web/Dockerfile --build-arg VERSION=$(VERSION) --build-arg COMMIT=$(COMMIT) --build-arg BUILD_DATE=$(BUILD_DATE) -t $(DOCKER_WEB_IMAGE):$(VERSION) -t $(DOCKER_WEB_IMAGE):latest .

docker-keycloak: ## Build keycloak image
	docker build -f docker/keycloak/Dockerfile -t $(DOCKER_KEYCLOAK_IMAGE):$(VERSION) -t $(DOCKER_KEYCLOAK_IMAGE):latest .

docker-all: docker-server docker-web docker-keycloak ## Build all Docker images

docker-push-server: ## Push server and worker images
	docker push $(DOCKER_IMAGE):$(VERSION)
	docker push $(DOCKER_IMAGE):latest
	docker push $(DOCKER_WORKER_IMAGE):$(VERSION)
	docker push $(DOCKER_WORKER_IMAGE):latest

docker-push-web: ## Push web UI image
	docker push $(DOCKER_WEB_IMAGE):$(VERSION)
	docker push $(DOCKER_WEB_IMAGE):latest

docker-push-keycloak: ## Push keycloak image
	docker push $(DOCKER_KEYCLOAK_IMAGE):$(VERSION)
	docker push $(DOCKER_KEYCLOAK_IMAGE):latest

docker-push: docker-push-server docker-push-web docker-push-keycloak ## Push all Docker images

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

kapi-recordings: build ## Generate kapi CLI demo videos (VHS)
	./website/tapes/generate.sh

bowrain-cli-recordings: build build-bowrain-cli ## Generate Bowrain CLI demo videos (VHS)
	./bowrain/e2e/tapes/generate.sh

brain-recordings: bowrain-cli-recordings ## Alias for bowrain-cli-recordings (backward compat)

cli-recordings: kapi-recordings bowrain-cli-recordings ## Generate all CLI demo videos

docs-assets: screenshots recordings cli-recordings ## Generate all documentation assets

fetch-docs-assets: ## Download pre-built docs assets from the docs-assets GitHub release
	@echo "Fetching docs assets from GitHub release..."
	@gh release download docs-assets --pattern 'docs-assets.tar.gz' --dir /tmp --clobber
	@mkdir -p website/static
	@tar xzf /tmp/docs-assets.tar.gz -C website/static
	@rm -f /tmp/docs-assets.tar.gz
	@echo "Done. Assets extracted to website/static/"
	@du -sh website/static/img website/static/video 2>/dev/null || true

# ── Polished Videos (Remotion) ──────────────────────────────────────────────

VIDEOS_DIR := website/videos

videos-deps: ## Install Remotion video dependencies
	cd $(VIDEOS_DIR) && $(NPM) ci

videos-setup: videos-deps ## Set up raw recording symlinks for Remotion
	cd $(VIDEOS_DIR) && $(NPM) run setup-raw

videos-studio: videos-setup ## Launch Remotion Studio for video preview
	cd $(VIDEOS_DIR) && $(NPM) run studio

videos-render: videos-setup ## Render all polished demo videos
	cd $(VIDEOS_DIR) && $(NPM) run build
	cd $(VIDEOS_DIR) && $(NPM) run publish

# ── Test ─────────────────────────────────────────────────────────────────────

test: ## Run all tests (all modules)
	$(GOTEST) ./... -count=1
	cd cli && $(GOTEST) ./... -count=1
	cd platform && $(GOTEST) ./... -count=1
	cd kapi && $(GOTEST) ./... -count=1
	cd bowrain-cli && $(GOTEST) ./... -count=1
	cd bowrain && $(GOTEST) ./... -count=1

test-fast: ## Run all tests with caching (fast local iteration)
	$(GOTEST) ./...
	cd cli && $(GOTEST) ./...
	cd platform && $(GOTEST) ./...
	cd kapi && $(GOTEST) ./...
	cd bowrain-cli && $(GOTEST) ./...
	cd bowrain && $(GOTEST) ./...

test-parallel: ## Run all tests in parallel (all modules concurrently)
	@$(GOTEST) ./... -count=1 & \
	(cd cli && $(GOTEST) ./... -count=1) & \
	(cd platform && $(GOTEST) ./... -count=1) & \
	(cd kapi && $(GOTEST) ./... -count=1) & \
	(cd bowrain-cli && $(GOTEST) ./... -count=1) & \
	(cd bowrain && $(GOTEST) ./... -count=1) & \
	wait

test-unit: ## Run unit tests only (exclude integration)
	$(GOTEST) ./... -count=1 -short
	cd cli && $(GOTEST) ./... -count=1 -short
	cd platform && $(GOTEST) ./... -count=1 -short
	cd kapi && $(GOTEST) ./... -count=1 -short
	cd bowrain-cli && $(GOTEST) ./... -count=1 -short
	cd bowrain && $(GOTEST) ./... -count=1 -short

test-race: ## Run tests with race detector
	$(GOTEST) ./... -count=1 -race
	cd cli && $(GOTEST) ./... -count=1 -race
	cd platform && $(GOTEST) ./... -count=1 -race
	cd kapi && $(GOTEST) ./... -count=1 -race
	cd bowrain-cli && $(GOTEST) ./... -count=1 -race
	cd bowrain && $(GOTEST) ./... -count=1 -race

test-framework: ## Run framework tests only
	$(GOTEST) ./... -count=1

test-platform: ## Run platform module tests only
	cd platform && $(GOTEST) ./... -count=1

test-cli: ## Run cli module tests only
	cd cli && $(GOTEST) ./... -count=1

test-kapi: ## Run kapi CLI tests only
	cd kapi && $(GOTEST) ./... -count=1

test-bowrain-cli: ## Run Bowrain CLI tests only
	cd bowrain-cli && $(GOTEST) ./... -count=1

test-brain: test-bowrain-cli ## Alias for test-bowrain-cli (backward compat)

test-bowrain: ## Run bowrain tests only
	cd bowrain && $(GOTEST) ./... -count=1

test-integration: ## Run integration tests
	$(GOTEST) ./... -count=1 -tags=integration -run Integration
	cd bowrain && $(GOTEST) ./... -count=1 -tags=integration -run Integration

GITHUB_TOKEN         ?= $(shell gh auth token 2>/dev/null)
OKAPI_BRIDGE_VERSION ?= v2.3.6
OKAPI_VERSION        ?= 1.48.0
BRIDGE_JAR           := $(HOME)/.cache/gokapi/bridge/$(OKAPI_BRIDGE_VERSION)-okapi$(OKAPI_VERSION)/okapi-bridge.jar
OKAPI_FILTERS_DIR    ?= $(HOME)/src/okapi/Okapi/okapi/filters
OKAPI_SUREFIRE_DIR   ?= okapi-surefire/$(OKAPI_VERSION)-v1
GOTEST_JSON_FILE     := $(COVER_DIR)/bridge-test-results.jsonl
NATIVE_JSON_FILE     := $(COVER_DIR)/native-test-results.jsonl
TEST_COMPARE_JSON    := website/static/data/test-comparison.json

fetch-bridge-jar: ## Download okapi-bridge JAR from GitHub release
	@GITHUB_TOKEN=$(GITHUB_TOKEN) bash scripts/fetch-okapi-bridge.sh

fetch-bridge-testdata: ## Download okapi test data from GitHub release
	@GITHUB_TOKEN=$(GITHUB_TOKEN) bash scripts/fetch-okapi-testdata.sh

fetch-okapi-surefire: ## Download Okapi Surefire XML reports from GitHub release
	@GITHUB_TOKEN=$(GITHUB_TOKEN) bash scripts/fetch-okapi-surefire.sh

test-bridge-filters: fetch-bridge-jar fetch-bridge-testdata ## Run bridge filter integration tests (requires Java)
	GOKAPI_BRIDGE_JAR=$(BRIDGE_JAR) $(GOTEST) -tags=integration -count=1 -v ./core/plugin/bridge/filters/...

test-bridge-pool: fetch-bridge-jar fetch-bridge-testdata ## Run bridge tests with shared JVM pool (faster, default 4 JVMs)
	@GOKAPI_BRIDGE_JAR=$(BRIDGE_JAR) bash scripts/run-bridge-tests.sh $(or $(BRIDGE_POOL_SIZE),4)

test-bridge-json: fetch-bridge-jar fetch-bridge-testdata ## Run bridge filter tests with JSON output
	@mkdir -p $(COVER_DIR)
	GOKAPI_BRIDGE_JAR=$(BRIDGE_JAR) $(GOTEST) -tags=integration -count=1 -json ./core/plugin/bridge/filters/... > $(GOTEST_JSON_FILE); true

test-native-json: ## Run native format tests with JSON output
	@mkdir -p $(COVER_DIR)
	$(GOTEST) -count=1 -json ./core/formats/... > $(NATIVE_JSON_FILE); true

generate-test-comparison: fetch-okapi-surefire test-bridge-json test-native-json ## Generate test comparison dashboard data
	@mkdir -p website/static/data
	$(GO) run ./scripts/testcompare \
		-okapi-dir $(OKAPI_SUREFIRE_DIR) \
		-gotest-bridge-json $(GOTEST_JSON_FILE) \
		-gotest-native-json $(NATIVE_JSON_FILE) \
		-bridge-src core/plugin/bridge/filters \
		-native-src core/formats \
		-out $(TEST_COMPARE_JSON)

generate-test-stubs: fetch-okapi-surefire ## Generate Go test stubs from Surefire XML
	$(GO) run ./scripts/gen-test-stubs \
		-surefire-dir $(OKAPI_SUREFIRE_DIR)

test-e2e: ## Run end-to-end tests against Docker stack
	bash e2e/run.sh

test-verbose: ## Run tests with verbose output
	$(GOTEST) ./... -count=1 -v
	cd cli && $(GOTEST) ./... -count=1 -v
	cd platform && $(GOTEST) ./... -count=1 -v
	cd kapi && $(GOTEST) ./... -count=1 -v
	cd bowrain-cli && $(GOTEST) ./... -count=1 -v
	cd bowrain && $(GOTEST) ./... -count=1 -v

cover: ## Run tests with coverage
	@mkdir -p $(COVER_DIR)
	$(GOTEST) ./... -count=1 -coverprofile=$(COVER_DIR)/framework.out -covermode=atomic
	cd cli && $(GOTEST) ./... -count=1 -coverprofile=../$(COVER_DIR)/cli.out -covermode=atomic
	cd platform && $(GOTEST) ./... -count=1 -coverprofile=../$(COVER_DIR)/platform.out -covermode=atomic
	cd kapi && $(GOTEST) ./... -count=1 -coverprofile=../$(COVER_DIR)/kapi.out -covermode=atomic
	cd bowrain-cli && $(GOTEST) ./... -count=1 -coverprofile=../$(COVER_DIR)/bowrain-cli.out -covermode=atomic
	cd bowrain && $(GOTEST) ./... -count=1 -coverprofile=../$(COVER_DIR)/bowrain.out -covermode=atomic
	cat $(COVER_DIR)/framework.out > $(COVER_DIR)/coverage.out
	tail -n +2 $(COVER_DIR)/cli.out >> $(COVER_DIR)/coverage.out
	tail -n +2 $(COVER_DIR)/platform.out >> $(COVER_DIR)/coverage.out
	tail -n +2 $(COVER_DIR)/kapi.out >> $(COVER_DIR)/coverage.out
	tail -n +2 $(COVER_DIR)/bowrain-cli.out >> $(COVER_DIR)/coverage.out
	tail -n +2 $(COVER_DIR)/bowrain.out >> $(COVER_DIR)/coverage.out
	$(GO) tool cover -html=$(COVER_DIR)/coverage.out -o $(COVER_DIR)/coverage.html
	@echo "Coverage report: $(COVER_DIR)/coverage.html"

# ── Code Quality ─────────────────────────────────────────────────────────────

fmt: ## Format Go source files
	$(GOFMT) -w -s .

vet: ## Run go vet
	$(GOVET) ./...
	cd cli && $(GOVET) ./...
	cd platform && $(GOVET) ./...
	cd kapi && $(GOVET) ./...
	cd bowrain-cli && $(GOVET) ./...
	cd bowrain && $(GOVET) ./...

lint: ## Run golangci-lint
ifdef GOLANGCI_LINT
	$(GOLANGCI_LINT) run ./...
	cd cli && $(GOLANGCI_LINT) run ./...
	cd platform && $(GOLANGCI_LINT) run ./...
	cd kapi && $(GOLANGCI_LINT) run ./...
	cd bowrain-cli && $(GOLANGCI_LINT) run ./...
	cd bowrain && $(GOLANGCI_LINT) run ./...
else
	@echo "golangci-lint not installed. Run 'make tools' to install."
endif

gha-lint: ## Lint GitHub Actions workflow files
	@command -v actionlint >/dev/null 2>&1 || { echo "actionlint not installed. Run 'brew install actionlint' or 'go install github.com/rhysd/actionlint/cmd/actionlint@latest'."; exit 1; }
	actionlint

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
	cd cli && $(GO) mod download && $(GO) mod tidy
	cd platform && $(GO) mod download && $(GO) mod tidy
	cd kapi && $(GO) mod download && $(GO) mod tidy
	cd bowrain-cli && $(GO) mod download && $(GO) mod tidy
	cd bowrain && $(GO) mod download && $(GO) mod tidy

deps-update: ## Update all dependencies
	$(GO) get -u ./...
	$(GO) mod tidy
	cd cli && $(GO) get -u ./...
	cd cli && $(GO) mod tidy
	cd platform && $(GO) get -u ./...
	cd platform && $(GO) mod tidy
	cd kapi && $(GO) get -u ./...
	cd kapi && $(GO) mod tidy
	cd bowrain-cli && $(GO) get -u ./...
	cd bowrain-cli && $(GO) mod tidy
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
