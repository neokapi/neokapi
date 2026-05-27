# bowrain/Makefile
# =================
# Build, test, and lint targets for the Bowrain platform, CLI, and core modules.
# Modules: neokapi/bowrain/core (interface), neokapi/bowrain/cli (CLI), neokapi/bowrain (server+app).
# Can be used standalone (cd bowrain && make test) or delegated from root.

# Suppress macOS linker warnings for CGO (Wails WebView)
ifeq ($(shell uname -s),Darwin)
export MACOSX_DEPLOYMENT_TARGET := 10.15
export CGO_CFLAGS := -mmacosx-version-min=10.15
export CGO_LDFLAGS := -mmacosx-version-min=10.15
endif

# Node 22 requires --experimental-strip-types to load .ts config files
# (vite.config.ts) via native ESM in tools like vite-plus (vp check/lint).
export NODE_OPTIONS := --experimental-strip-types

# ── Shared Variables (inherit from root or compute standalone) ────────────────

ROOT_DIR    ?= $(shell cd .. && pwd)
VERSION     ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT      ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_DATE  ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
VERSION_PKG ?= github.com/neokapi/neokapi/core/version
LDFLAGS     ?= -ldflags "-X $(VERSION_PKG).Version=$(VERSION) -X $(VERSION_PKG).Commit=$(COMMIT) -X $(VERSION_PKG).BuildDate=$(BUILD_DATE)"

GO          := go
GOTAGS      := -tags fts5
ifeq ($(shell uname -s),Darwin)
export PKG_CONFIG_PATH := /opt/homebrew/opt/icu4c@78/lib/pkgconfig:$(PKG_CONFIG_PATH)
endif
GOTEST      := $(GO) test $(GOTAGS)
GOBUILD     := $(GO) build $(GOTAGS)
GOVET       := $(GO) vet $(GOTAGS)
GOFMT       := gofmt
BIN_DIR     := $(ROOT_DIR)/bin
COVER_DIR   := $(ROOT_DIR)/coverage

FRONTEND_DIR := apps/bowrain/frontend
WEB_DIR      := apps/web
CTRL_DIR     := apps/ctrl
PULSE_DIR    := apps/pulse
KC_THEME_DIR := apps/keycloak-theme
EMAILS_DIR   := emails
CERT_DIR     := docker/traefik/certs
SERVER_PROTO_DIR := proto/v1

# Tools
GOLANGCI_LINT := $(shell which golangci-lint 2>/dev/null || { test -x "$$(go env GOPATH)/bin/golangci-lint" && echo "$$(go env GOPATH)/bin/golangci-lint"; })
PROTOC        := $(shell which protoc 2>/dev/null)
PROTOC_GEN_GO := $(shell which protoc-gen-go 2>/dev/null)

# ── CI auto-detection ────────────────────────────────────────────────────────
ifdef CI
  _RACE     := -race
  _COVMODE  := -covermode=atomic
else
  _RACE     :=
  _COVMODE  :=
endif

GOTEST_BASE := $(GOTEST) $(_RACE) -shuffle=on -timeout 15m

# Docker
DOCKER_IMAGE          := ghcr.io/neokapi/bowrain-server
DOCKER_WORKER_IMAGE   := ghcr.io/neokapi/bowrain-worker
DOCKER_WEB_IMAGE      := ghcr.io/neokapi/bowrain-web
DOCKER_KEYCLOAK_IMAGE := ghcr.io/neokapi/bowrain-keycloak

.PHONY: all help \
        build-server build-worker build-kapi-bowrain-plugin build-bowrain build-headless install-kapi-bowrain-plugin \
        ui-deps ui-build ui-check storybook storybook-dev storybook-build \
        frontend-deps frontend-dev frontend-build frontend-check build-ui \
        web-deps web-build web-check \
        ctrl-deps ctrl-dev ctrl-build ctrl-check \
        pulse-deps pulse-dev pulse-build pulse-check \
        email-deps email-build keycloak-theme keycloak-check \
        frontend-check-all \
        docker-server docker-web docker-keycloak docker-all \
        docker-push-server docker-push-web docker-push-keycloak docker-push \
        dev-deps dev-deps-down dev-server dev-worker dev-web dev-ctrl bowrain-dev certs \
        stack-up stack-up-web stack-up-tls stack-down stack-logs stack-ps stack-build \
        screenshots recordings \
        test test-fast test-unit test-race test-verbose test-integration \
        test-platform test-bowrain-cli test-bowrain test-e2e-bowrain test-e2e-cloud test-e2e-dev \
        cover fmt vet lint check proto deps deps-update clean

# ── General ──────────────────────────────────────────────────────────────────

all: help

help: ## Show bowrain targets
	@echo ""
	@echo "Bowrain targets (make -C bowrain <target>):"
	@awk '/^[a-zA-Z_-]+:.*## / { \
		match($$0, /## (.*)/); desc = substr($$0, RSTART+3); \
		match($$0, /^[a-zA-Z_-]+/); target = substr($$0, RSTART, RLENGTH); \
		printf "  \033[36m%-20s\033[0m %s\n", target, desc \
	}' $(MAKEFILE_LIST)
	@echo ""

# ── Build ────────────────────────────────────────────────────────────────────

build-server: ## Build the Bowrain REST server
	@mkdir -p $(BIN_DIR)
	$(GOBUILD) $(LDFLAGS) -o $(BIN_DIR)/bowrain-server ./cmd/bowrain-server

build-worker: ## Build the Bowrain worker
	@mkdir -p $(BIN_DIR)
	$(GOBUILD) $(LDFLAGS) -o $(BIN_DIR)/bowrain-worker ./cmd/bowrain-worker

build-kapi-bowrain-plugin: ## Build the kapi-bowrain plugin binary (manifest-driven)
	@mkdir -p $(BIN_DIR)
	cd cli && $(GOBUILD) $(LDFLAGS) -o $(BIN_DIR)/kapi-bowrain ./cmd/kapi-bowrain

build-bowrain: frontend-build ## Build the Bowrain desktop app
	cd apps/bowrain && wails3 build -ldflags "-X $(VERSION_PKG).Version=$(VERSION) -X $(VERSION_PKG).Commit=$(COMMIT) -X $(VERSION_PKG).BuildDate=$(BUILD_DATE)"

build-headless: frontend-build ## Build headless desktop binary (server mode, no GUI deps)
	@mkdir -p $(BIN_DIR)
	cd apps/bowrain && $(GOBUILD) -tags server $(LDFLAGS) -o $(BIN_DIR)/bowrain-headless .

install-kapi-bowrain-plugin: ## Install kapi-bowrain plugin binary to GOPATH/bin
	cd cli && $(GO) install $(LDFLAGS) ./cmd/kapi-bowrain

# ── Frontend (Bowrain UI) ───────────────────────────────────────────────────

frontend-deps: ## Install frontend dependencies
	cd $(FRONTEND_DIR) && vp install

frontend-dev: ## Start frontend dev server
	@printf '\033]0;Bowrain Frontend\007'
	cd $(FRONTEND_DIR) && vp run dev

frontend-build: ui-build frontend-deps ## Build frontend for production
	cd $(FRONTEND_DIR) && vp run build

frontend-check: frontend-deps ## Lint, format-check, and typecheck Bowrain frontend
	cd $(FRONTEND_DIR) && vp check

build-ui: build-server frontend-build ## Build server + frontend

# ── Shared UI Package ──────────────────────────────────────────────────────

ui-deps: ## Install shared UI package dependencies
	cd packages/ui && vp install

ui-build: ui-deps ## Build shared UI declarations
	cd packages/ui && vpx tsc

ui-check: ui-deps ## Lint, format-check, and typecheck shared UI
	cd packages/ui && vp check

storybook: ui-deps email-deps ## Launch Storybook dev server (port 6006)
	@printf '\033]0;Storybook\007'
	cd storybook && vpx storybook dev -p 6006

storybook-dev: storybook ## Alias for storybook

storybook-build: ui-deps email-deps ## Build Storybook static site
	cd storybook && vpx storybook build -o storybook-static

# ── SaaS Web UI (bowrain-server) ───────────────────────────────────────────

web-deps: ## Install SaaS web UI dependencies
	cd $(WEB_DIR) && vp install

web-build: ui-build web-deps ## Build SaaS web UI for production
	@printf '{"version":"%s","commit":"%s","build_date":"%s","component":"web"}\n' "$(VERSION)" "$(COMMIT)" "$(BUILD_DATE)" > $(WEB_DIR)/public/version.json
	cd $(WEB_DIR) && vp run build

web-check: web-deps ## Lint, format-check, and typecheck SaaS web UI
	cd $(WEB_DIR) && vp check

# ── Control Plane (ctrl.bowrain.cloud) ───────────────────────────────────────

ctrl-deps: ## Install control plane dependencies
	cd $(CTRL_DIR) && vp install

ctrl-dev: ctrl-deps ## Start control plane dev server (port 3100)
	@printf '\033]0;Ctrl UI\007'
	cd $(CTRL_DIR) && \
	VITE_ADMIN_OIDC_ISSUER_URL=https://auth.bowrain.mymac/realms/bowrain-admin \
	vp run dev

ctrl-build: ui-build ctrl-deps ## Build control plane for production
	cd $(CTRL_DIR) && vp run build

ctrl-check: ctrl-deps ## Lint, format-check, and typecheck control plane
	cd $(CTRL_DIR) && vp check

# ── Pulse Dashboard (pulse.bowrain.cloud) ─────────────────────────────────

pulse-deps: ## Install Pulse dashboard dependencies
	cd $(PULSE_DIR) && vp install

pulse-dev: pulse-deps ## Start Pulse dev server (port 3200)
	@printf '\033]0;Pulse UI\007'
	cd $(PULSE_DIR) && vp run dev

pulse-build: ui-build pulse-deps ## Build Pulse dashboard for production
	cd $(PULSE_DIR) && vp run build

pulse-check: pulse-deps ## Lint, format-check, and typecheck Pulse dashboard
	cd $(PULSE_DIR) && vp check

# ── Email Templates (React Email -> HTML) ────────────────────────────────────

email-deps: ## Install React Email template dependencies
	cd $(EMAILS_DIR) && vp install

email-build: email-deps ## Pre-render React Email templates
	cd $(EMAILS_DIR) && vp run build

# ── Keycloak Theme ─────────────────────────────────────────────────────────

keycloak-theme: ui-deps ## Build Keycloak login theme JAR
	cd $(KC_THEME_DIR) && vp run build-keycloak-theme

keycloak-check: ## Lint, format-check, and typecheck Keycloak theme
	cd $(KC_THEME_DIR) && vp check

# ── Frontend Checks (all) ────────────────────────────────────────────────

frontend-check-all: ui-check web-check ctrl-check pulse-check frontend-check keycloak-check ## Run checks across all frontend projects

# ── Docker ──────────────────────────────────────────────────────────────────

docker-server: ## Build server and worker images
	docker build -f docker/bowrain-server/Dockerfile --build-arg VERSION=$(VERSION) --build-arg COMMIT=$(COMMIT) --build-arg BUILD_DATE=$(BUILD_DATE) -t $(DOCKER_IMAGE):$(VERSION) -t $(DOCKER_IMAGE):latest $(ROOT_DIR)
	docker build -f docker/bowrain-worker/Dockerfile --build-arg VERSION=$(VERSION) --build-arg COMMIT=$(COMMIT) --build-arg BUILD_DATE=$(BUILD_DATE) -t $(DOCKER_WORKER_IMAGE):$(VERSION) -t $(DOCKER_WORKER_IMAGE):latest $(ROOT_DIR)

docker-web: ## Build web UI image
	docker build -f docker/bowrain-web/Dockerfile --build-arg VERSION=$(VERSION) --build-arg COMMIT=$(COMMIT) --build-arg BUILD_DATE=$(BUILD_DATE) -t $(DOCKER_WEB_IMAGE):$(VERSION) -t $(DOCKER_WEB_IMAGE):latest $(ROOT_DIR)

docker-keycloak: ## Build keycloak image
	docker build -f docker/keycloak/Dockerfile -t $(DOCKER_KEYCLOAK_IMAGE):$(VERSION) -t $(DOCKER_KEYCLOAK_IMAGE):latest $(ROOT_DIR)

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

# ── Dev Environment ──────────────────────────────────────────────────────────

dev-deps: ## Start dev dependencies (Traefik + Keycloak + Mailpit) in Docker
	@printf '\033]0;Dev Dependencies\007'
	docker compose -f $(ROOT_DIR)/compose.yaml up -d --wait

dev-deps-down: ## Stop dev dependencies
	docker compose -f $(ROOT_DIR)/compose.yaml down -v

bowrain-dev: ## Launch Bowrain desktop app in dev mode (hot reload)
	@printf '\033]0;Bowrain Desktop\007'
	cd apps/bowrain && wails3 dev

certs: ## Generate mkcert TLS certificates for *.bowrain.mymac
	@mkdir -p $(CERT_DIR)
	mkcert -cert-file $(CERT_DIR)/wildcard.pem -key-file $(CERT_DIR)/wildcard-key.pem \
		"*.bowrain.mymac" "bowrain.mymac"

dev-server: ## Run bowrain-server locally (no UI build; use dev-web for HMR)
	@printf '\033]0;Bowrain Server\007'
	@mkdir -p $(BIN_DIR)
	$(GOBUILD) $(LDFLAGS) -o $(BIN_DIR)/bowrain-server ./cmd/bowrain-server
	BOWRAIN_JWT_SECRET=dev-secret-change-in-production \
	BOWRAIN_OIDC_ISSUER_URL=https://auth.bowrain.mymac/realms/bowrain \
	BOWRAIN_OIDC_CLIENT_ID=bowrain \
	BOWRAIN_OIDC_CLIENT_SECRET=bowrain-secret \
	BOWRAIN_SMTP_HOST=localhost:1025 \
	BOWRAIN_SMTP_FROM=noreply@bowrain.cloud \
	BOWRAIN_DATABASE_URL=postgres://bowrain:bowrain@localhost:5432/bowrain_test?sslmode=disable \
	BOWRAIN_REDIS_URL=redis://localhost:6379 \
	BOWRAIN_NATS_URL=nats://localhost:4222 \
	BLOB_STORAGE_LOCAL_DIR=/tmp/bowrain-blobs \
	BOWRAIN_ADMIN_OIDC_ISSUER_URL=https://auth.bowrain.mymac/realms/bowrain-admin \
	BOWRAIN_ADMIN_OIDC_CLIENT_ID=bowrain-admin \
	$(BIN_DIR)/bowrain-server

dev-worker: ## Run bowrain-worker locally (processes sync push + translation jobs)
	@printf '\033]0;Bowrain Worker\007'
	@mkdir -p $(BIN_DIR)
	$(GOBUILD) $(LDFLAGS) -o $(BIN_DIR)/bowrain-worker ./cmd/bowrain-worker
	BOWRAIN_DATABASE_URL=postgres://bowrain:bowrain@localhost:5432/bowrain_test?sslmode=disable \
	BOWRAIN_NATS_URL=nats://localhost:4222 \
	BOWRAIN_REDIS_URL=redis://localhost:6379 \
	LOCAL_BLOB_DIR=/tmp/bowrain-blobs \
	$(BIN_DIR)/bowrain-worker

dev-web: ## Run web UI dev server with HMR (proxy -> localhost:8080)
	@printf '\033]0;Web UI\007'
	cd apps/web && vp run dev

dev-ctrl: ctrl-deps ## Run control plane dev server with HMR (proxy -> localhost:8080)
	@printf '\033]0;Ctrl UI\007'
	cd $(CTRL_DIR) && \
	VITE_ADMIN_OIDC_ISSUER_URL=https://auth.bowrain.mymac/realms/bowrain-admin \
	vp run dev

# ── Full Local Stack (everything in Docker) ──────────────────────────────────
# Unlike dev-deps (deps only; server/web on the host), the stack-* targets run
# the entire platform — server + async worker + backing services — in Docker on
# plain-HTTP localhost. See compose.full.yaml. Set GEMINI_API_KEY etc. in
# bowrain/.env (cp .env.example .env) for real translations; otherwise the
# worker uses the offline "demo" provider so push → translate → pull still works.

COMPOSE_FULL := docker compose -f compose.full.yaml

stack-up: ## Build + start the full local stack (server+worker+deps) in Docker
	$(COMPOSE_FULL) up -d --build --wait
	@echo "Bowrain server:  http://localhost:8080  (health: /api/v1/health)"
	@echo "Keycloak:        http://localhost:8180  (admin/admin)"
	@echo "Mailpit:         http://localhost:8025"

stack-up-web: ## Full local stack including the web UI (served at http://localhost:8080)
	$(COMPOSE_FULL) --profile web up -d --build --wait

stack-up-tls: ## Full local stack + web behind Traefik TLS at https://bowrain.mymac
	docker compose -f compose.full.yaml -f compose.tls.yaml --profile web up -d --build --wait
	@echo "Web app + API:  https://bowrain.mymac"
	@echo "Keycloak:       https://auth.bowrain.mymac  (admin/admin)"
	@echo "Mailpit:        https://mail.bowrain.mymac"
	@echo "(needs the *.bowrain.mymac host setup: dnsmasq + mkcert + 'make certs')"

stack-build: ## (Re)build the local-stack server + worker images
	$(COMPOSE_FULL) build bowrain-server bowrain-worker

stack-ps: ## Show full local stack service status
	$(COMPOSE_FULL) ps

stack-logs: ## Tail logs for the full local stack
	$(COMPOSE_FULL) logs -f --tail=100

stack-down: ## Stop the full local stack and remove its volumes
	$(COMPOSE_FULL) down -v

# ── Documentation Assets (Screenshots & Recordings) ─────────────────────────

screenshots: frontend-deps ## Generate documentation screenshots
	cd $(FRONTEND_DIR) && vp run screenshots

recordings: frontend-deps ## Generate Bowrain (GUI) video recordings
	cd $(FRONTEND_DIR) && vp run recordings:all

test-e2e-bowrain: ## Run Bowrain server end-to-end tests (requires Docker)
	bash e2e/server/run.sh

test-e2e-cloud: ## Run cloud e2e tests against a live server (set BOWRAIN_URL)
	bash e2e/cloud/run.sh

test-e2e-dev: ## Run cloud e2e tests against the dev environment
	bash e2e/cloud/run-dev.sh

# ── Test ─────────────────────────────────────────────────────────────────────

test: ## Run all platform tests
	cd core && $(GOTEST_BASE) ./... -count=1
	cd cli && $(GOTEST_BASE) ./... -count=1
	$(GOTEST_BASE) ./... -count=1

test-fast: ## Run all platform tests with caching
	cd core && $(GOTEST_BASE) ./...
	cd cli && $(GOTEST_BASE) ./...
	$(GOTEST_BASE) ./...

test-unit: ## Run platform unit tests only
	cd core && $(GOTEST_BASE) ./... -count=1 -short
	cd cli && $(GOTEST_BASE) ./... -count=1 -short
	$(GOTEST_BASE) ./... -count=1 -short

test-race: ## Run platform tests with race detector
	cd core && $(GOTEST) -race -shuffle=on ./... -count=1
	cd cli && $(GOTEST) -race -shuffle=on ./... -count=1
	$(GOTEST) -race -shuffle=on ./... -count=1

test-verbose: ## Run platform tests with verbose output
	cd core && $(GOTEST_BASE) ./... -count=1 -v
	cd cli && $(GOTEST_BASE) ./... -count=1 -v
	$(GOTEST_BASE) ./... -count=1 -v

test-integration: ## Run platform integration tests
	$(GOTEST_BASE) ./... -count=1 -tags=integration -run Integration

test-platform: ## Run platform core module tests only
	@mkdir -p $(COVER_DIR)
ifdef CI
	cd core && $(GOTEST_BASE) -coverprofile=$(COVER_DIR)/platform.out $(_COVMODE) -json ./... > ../test-results-platform.json
else
	cd core && $(GOTEST_BASE) ./... -count=1
endif

test-bowrain-cli: ## Run Bowrain CLI tests only
	@mkdir -p $(COVER_DIR)
ifdef CI
	cd cli && $(GOTEST_BASE) -coverprofile=$(COVER_DIR)/bowrain-cli.out $(_COVMODE) -json ./... > ../test-results-bowrain-cli.json
else
	cd cli && $(GOTEST_BASE) ./... -count=1
endif

test-bowrain: ## Run bowrain module tests only
	@mkdir -p $(COVER_DIR)
ifdef CI
	$(GOTEST_BASE) -coverprofile=$(COVER_DIR)/bowrain.out $(_COVMODE) -json $$($(GO) list ./... | grep -v '/apps/bowrain') > ../test-results-bowrain.json
else
	$(GOTEST_BASE) ./... -count=1
endif

# ── Coverage ─────────────────────────────────────────────────────────────────

cover: ## Run platform tests with coverage
	@mkdir -p $(COVER_DIR)
	cd core && $(GOTEST_BASE) -coverprofile=$(COVER_DIR)/platform.out $(_COVMODE) ./... -count=1
	cd cli && $(GOTEST_BASE) -coverprofile=$(COVER_DIR)/bowrain-cli.out $(_COVMODE) ./... -count=1
	$(GOTEST_BASE) -coverprofile=$(COVER_DIR)/bowrain.out $(_COVMODE) ./... -count=1

# ── Code Quality ─────────────────────────────────────────────────────────────

fmt: ## Format Go source files
	$(GOFMT) -w -s .

vet: ## Run go vet
	cd core && $(GOVET) ./...
	cd cli && $(GOVET) ./...
	$(GOVET) ./...

lint: ## Run golangci-lint
ifdef GOLANGCI_LINT
	cd core && $(GOLANGCI_LINT) run ./...
	cd cli && $(GOLANGCI_LINT) run ./...
	$(GOLANGCI_LINT) run ./...
else
	@echo "golangci-lint not installed. Run 'make tools' to install."
endif

check: fmt vet lint ## Run all code quality checks

# ── Protobuf ─────────────────────────────────────────────────────────────────

proto: ## Generate Go code from server protobuf definitions
ifndef PROTOC
	$(error "protoc not found. Install Protocol Buffers compiler.")
endif
ifndef PROTOC_GEN_GO
	$(error "protoc-gen-go not found. Run 'make tools' to install.")
endif
	protoc --go_out=. --go_opt=paths=source_relative \
		--go-grpc_out=. --go-grpc_opt=paths=source_relative \
		$(SERVER_PROTO_DIR)/*.proto

# ── Dependencies ─────────────────────────────────────────────────────────────

deps: ## Download and tidy platform dependencies
	cd core && $(GO) mod download && $(GO) mod tidy
	cd cli && $(GO) mod download && $(GO) mod tidy
	$(GO) mod download && $(GO) mod tidy

deps-update: ## Update platform dependencies
	cd core && $(GO) get -u ./... && $(GO) mod tidy
	cd cli && $(GO) get -u ./... && $(GO) mod tidy
	$(GO) get -u ./... && $(GO) mod tidy

# ── Clean ────────────────────────────────────────────────────────────────────

clean: ## Clean platform build artifacts
	rm -rf packages/ui/storybook-static
	rm -rf packages/ui/node_modules
	rm -rf $(FRONTEND_DIR)/dist
	rm -rf $(FRONTEND_DIR)/node_modules
	rm -rf $(WEB_DIR)/dist
	rm -rf $(WEB_DIR)/node_modules
	rm -rf $(CTRL_DIR)/dist
	rm -rf $(CTRL_DIR)/node_modules
	rm -rf $(KC_THEME_DIR)/dist_keycloak
	rm -rf $(KC_THEME_DIR)/node_modules
	$(GO) clean -cache -testcache
