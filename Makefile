# neokapi Makefile
# ================
# Framework targets run directly from this root directory.
# Bowrain targets forward to bowrain/Makefile.
#
# Module-specific targets live in:
#   make help              (this file)
#   make -C bowrain help   (bowrain sub-Makefile)

.DEFAULT_GOAL := help

# ── Shared Variables (exported to sub-makes) ──────────────────────────────────

export ROOT_DIR    := $(shell pwd)
export VERSION     ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
export COMMIT      := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
export BUILD_DATE  := $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
export VERSION_PKG := github.com/neokapi/neokapi/core/version
export LDFLAGS     := -ldflags "-X $(VERSION_PKG).Version=$(VERSION) -X $(VERSION_PKG).Commit=$(COMMIT) -X $(VERSION_PKG).BuildDate=$(BUILD_DATE)"

GO := go
GOTEST  := $(GO) test
GOBUILD := $(GO) build
GOVET   := $(GO) vet
GOFMT   := gofmt
BIN_DIR := $(ROOT_DIR)/bin
COVER_DIR := coverage

GOLANGCI_LINT := $(shell which golangci-lint 2>/dev/null || { test -x "$$(go env GOPATH)/bin/golangci-lint" && echo "$$(go env GOPATH)/bin/golangci-lint"; })

# ── Forwarded targets ───────────────────────────────────────────────────────
# Targets listed here run at root (framework) then forward to bowrain/Makefile.

BOTH_TARGETS := proto deps deps-update

$(BOTH_TARGETS):
	@$(MAKE) --no-print-directory _fw-$@
	@$(MAKE) -C bowrain $@

# ── Aggregate targets (framework + bowrain) ─────────────────────────────────

test: ## Run all tests (framework + bowrain)
	@$(MAKE) --no-print-directory _fw-test
	@$(MAKE) -C bowrain test

test-fast: ## Run all tests with caching
	@$(MAKE) --no-print-directory _fw-test-fast
	@$(MAKE) -C bowrain test-fast

test-unit: ## Run unit tests only (-short)
	@$(MAKE) --no-print-directory _fw-test-unit
	@$(MAKE) -C bowrain test-unit

test-race: ## Run tests with race detector
	@$(MAKE) --no-print-directory _fw-test-race
	@$(MAKE) -C bowrain test-race

test-verbose: ## Run tests with verbose output
	@$(MAKE) --no-print-directory _fw-test-verbose
	@$(MAKE) -C bowrain test-verbose

test-integration: ## Run integration tests
	@$(MAKE) --no-print-directory _fw-test-integration
	@$(MAKE) -C bowrain test-integration

fmt: ## Format Go source files
	$(GOFMT) -w -s .

vet: ## Run go vet (all modules)
	@$(MAKE) --no-print-directory _fw-vet
	@$(MAKE) -C bowrain vet

lint: ## Run golangci-lint (all modules)
	@$(MAKE) --no-print-directory _fw-lint
	@$(MAKE) -C bowrain lint

check: fmt vet lint ## Run all code quality checks

test-parallel: ## Run all tests in parallel
	@$(MAKE) --no-print-directory _fw-test & $(MAKE) -C bowrain test & wait

# ── Framework test/quality internals ────────────────────────────────────────

_fw-test:
	$(GOTEST) ./... -count=1
	cd cli && $(GOTEST) ./... -count=1
	cd kapi && $(GOTEST) ./... -count=1

_fw-test-fast:
	$(GOTEST) ./...
	cd cli && $(GOTEST) ./...
	cd kapi && $(GOTEST) ./...

_fw-test-unit:
	$(GOTEST) ./... -count=1 -short
	cd cli && $(GOTEST) ./... -count=1 -short
	cd kapi && $(GOTEST) ./... -count=1 -short

_fw-test-race:
	$(GOTEST) ./... -count=1 -race
	cd cli && $(GOTEST) ./... -count=1 -race
	cd kapi && $(GOTEST) ./... -count=1 -race

_fw-test-verbose:
	$(GOTEST) ./... -count=1 -v
	cd cli && $(GOTEST) ./... -count=1 -v
	cd kapi && $(GOTEST) ./... -count=1 -v

_fw-test-integration:
	$(GOTEST) ./... -count=1 -tags=integration -run Integration

_fw-vet:
	$(GOVET) ./...
	cd cli && $(GOVET) ./...
	cd kapi && $(GOVET) ./...

_fw-lint:
ifdef GOLANGCI_LINT
	$(GOLANGCI_LINT) run ./...
	cd cli && $(GOLANGCI_LINT) run ./...
	cd kapi && $(GOLANGCI_LINT) run ./...
else
	@echo "golangci-lint not installed. Run 'make tools' to install."
endif

_fw-proto:
	@echo "No framework protos (placeholder)"

_fw-deps:
	$(GO) mod download && $(GO) mod tidy
	cd cli && $(GO) mod download && $(GO) mod tidy
	cd kapi && $(GO) mod download && $(GO) mod tidy

_fw-deps-update:
	$(GO) get -u ./... && $(GO) mod tidy
	cd cli && $(GO) get -u ./... && $(GO) mod tidy
	cd kapi && $(GO) get -u ./... && $(GO) mod tidy

# ── Per-Module Test ─────────────────────────────────────────────────────────

test-framework: ## Run framework module tests only
	$(GOTEST) ./... -count=1

test-cli: ## Run cli module tests only
	cd cli && $(GOTEST) ./... -count=1

test-kapi: ## Run kapi CLI tests only
	cd kapi && $(GOTEST) ./... -count=1

test-platform test-bowrain-cli test-bowrain: ## Run individual bowrain module tests
	$(MAKE) -C bowrain $@

# ── Module Isolation ──────────────────────────────────────────────────────────

verify-isolation: ## Verify all Go module isolation boundaries
	GOWORK=off bash -c "go build ./..."
	GOWORK=off bash -c "cd cli && go build ./..."
	GOWORK=off bash -c "cd bowrain/core && go build ./..."
	GOWORK=off bash -c "cd kapi && go build ./..."
	GOWORK=off bash -c "cd bowrain/cli && go build ./..."
	@# kapi must not depend on bowrain
	@if cd kapi && GOWORK=off go list -m all 2>/dev/null | grep -q 'neokapi/bowrain'; then echo "ERROR: kapi depends on bowrain"; exit 1; fi
	@# bowrain must not depend on cli
	@if cd bowrain && GOWORK=off go list -m all 2>/dev/null | grep -iE 'neokapi/cli'; then echo "ERROR: bowrain depends on cli"; exit 1; fi
	@# kapi must not have heavy deps
	@if cd kapi && GOWORK=off go list -m all 2>/dev/null | grep -iE 'wails|echo|oidc|keyring'; then echo "ERROR: kapi has heavy deps"; exit 1; fi

# ── Build ────────────────────────────────────────────────────────────────────

build: ## Build the kapi CLI
	@mkdir -p $(BIN_DIR)
	cd kapi && $(GOBUILD) $(LDFLAGS) -o $(BIN_DIR)/kapi ./cmd/kapi

build-all: ## Build all Go binaries
	@mkdir -p $(BIN_DIR)
	cd kapi && $(GOBUILD) $(LDFLAGS) -o $(BIN_DIR)/kapi ./cmd/kapi
	$(MAKE) -C bowrain build-server build-worker build-bowrain-cli

# Forward bowrain build targets
build-server build-worker build-bowrain-cli build-bowrain build-headless install-bowrain-cli:
	$(MAKE) -C bowrain $@

# ── Kapi Desktop ────────────────────────────────────────────────────────────

# Node 22 requires --experimental-strip-types to load vite.config.ts natively.
export NODE_OPTIONS := --experimental-strip-types

KAPI_DESKTOP_DIR := apps/kapi-desktop

build-kapi-desktop: kapi-desktop-frontend-build ## Build the Kapi Desktop app
	cd $(KAPI_DESKTOP_DIR) && wails3 build

kapi-desktop-dev: kapi-desktop-frontend-deps ## Run Kapi Desktop in dev mode (hot reload)
	cd $(KAPI_DESKTOP_DIR) && wails3 dev

kapi-desktop-test: ## Run Kapi Desktop Go backend tests
	cd $(KAPI_DESKTOP_DIR) && $(GO) test ./backend/... -count=1

kapi-desktop-frontend-deps: ## Install Kapi Desktop frontend dependencies
	cd $(KAPI_DESKTOP_DIR)/frontend && vp install

kapi-desktop-frontend-dev: kapi-desktop-frontend-deps ## Start Kapi Desktop frontend dev server
	cd $(KAPI_DESKTOP_DIR)/frontend && vp dev --port 5174 --strictPort

kapi-desktop-frontend-build: kapi-desktop-frontend-deps ## Build Kapi Desktop frontend for production
	cd $(KAPI_DESKTOP_DIR)/frontend && vp build

kapi-desktop-frontend-test: kapi-desktop-frontend-deps ## Run Kapi Desktop frontend tests
	cd $(KAPI_DESKTOP_DIR)/frontend && vp test

kapi-desktop-frontend-check: kapi-desktop-frontend-deps ## Lint + format + typecheck Kapi Desktop frontend
	cd $(KAPI_DESKTOP_DIR)/frontend && vp check

storybook-fixtures: ## Generate Storybook fixtures from real format/tool data
	@./scripts/gen-storybook-fixtures.sh

flow-editor-deps: ## Install flow-editor dependencies
	cd packages/flow-editor && vp install

flow-editor-check: flow-editor-deps ## Lint + format + typecheck flow-editor package
	cd packages/flow-editor && vp check

flow-editor-test: flow-editor-deps ## Run flow-editor tests
	cd packages/flow-editor && vp test

kapi-desktop-storybook: kapi-desktop-frontend-deps ## Run Kapi Desktop Storybook (port 6007)
	cd $(KAPI_DESKTOP_DIR)/frontend && vp exec storybook dev -p 6007

kapi-desktop-storybook-build: kapi-desktop-frontend-deps ## Build Kapi Desktop Storybook
	cd $(KAPI_DESKTOP_DIR)/frontend && vp exec storybook build -o storybook-static

install: ## Install kapi CLI to GOPATH/bin
	cd kapi && $(GO) install $(LDFLAGS) ./cmd/kapi

# ── Coverage ─────────────────────────────────────────────────────────────────

cover: ## Run tests with coverage (merged report)
	@mkdir -p $(COVER_DIR)
	$(GOTEST) ./... -count=1 -coverprofile=$(COVER_DIR)/framework.out -covermode=atomic
	cd cli && $(GOTEST) ./... -count=1 -coverprofile=$(COVER_DIR)/cli.out -covermode=atomic
	cd kapi && $(GOTEST) ./... -count=1 -coverprofile=$(COVER_DIR)/kapi.out -covermode=atomic
	@$(MAKE) -C bowrain cover
	cat $(COVER_DIR)/framework.out > $(COVER_DIR)/coverage.out
	tail -n +2 $(COVER_DIR)/cli.out >> $(COVER_DIR)/coverage.out
	tail -n +2 $(COVER_DIR)/kapi.out >> $(COVER_DIR)/coverage.out
	tail -n +2 $(COVER_DIR)/platform.out >> $(COVER_DIR)/coverage.out
	tail -n +2 $(COVER_DIR)/bowrain-cli.out >> $(COVER_DIR)/coverage.out
	tail -n +2 $(COVER_DIR)/bowrain.out >> $(COVER_DIR)/coverage.out
	$(GO) tool cover -html=$(COVER_DIR)/coverage.out -o $(COVER_DIR)/coverage.html
	@echo "Coverage report: $(COVER_DIR)/coverage.html"

# ── E2E Tests ────────────────────────────────────────────────────────────────

test-e2e: ## Run all end-to-end tests
	$(GOTEST) ./e2e/... -count=1 -v 2>/dev/null || true
	$(MAKE) -C bowrain test-e2e-bowrain

test-e2e-kapi: ## Run kapi e2e tests
	$(GOTEST) ./e2e/... -count=1 -v

test-e2e-bowrain: ; $(MAKE) -C bowrain $@
test-e2e-cloud: ; $(MAKE) -C bowrain $@ ## Run cloud e2e tests against a live server
test-e2e-dev: ; $(MAKE) -C bowrain $@ ## Run cloud e2e tests against dev environment

# ── Bridge Tests ────────────────────────────────────────────────────────────

fetch-bridge-jar: ## Fetch bridge JAR for testing
	$(GO) run ./scripts/fetch-bridge-jar

fetch-bridge-testdata: ## Fetch bridge test data
	$(GO) run ./scripts/fetch-bridge-testdata

test-bridge-filters: ## Run bridge filter tests
	$(GOTEST) ./core/plugin/bridge/filters/... -count=1

test-bridge-pool: ## Run bridge pool tests
	$(GOTEST) ./core/plugin/bridge/... -count=1 -run Pool

test-bridge-json: ## Run bridge tests with JSON output
	$(GOTEST) ./core/plugin/bridge/filters/... -count=1 -json > $(COVER_DIR)/bridge-test-results.jsonl

test-native-json: ## Run native format tests with JSON output
	$(GOTEST) ./core/formats/... -count=1 -json > $(COVER_DIR)/native-test-results.jsonl

# ── Bench (composite target at root) ───────────────────────────────────────

bench-build: ## Build benchmark binary
	cd bench/pseudobench && $(GOBUILD) -o pseudobench .

bench-generate: ## Generate benchmark data
	cd bench/pseudobench && $(GO) run . generate

bench-run: ## Run benchmarks
	cd bench/pseudobench && $(GO) run . run

bench-run-bridge: ## Run bridge benchmarks
	cd bench/pseudobench && $(GO) run . run --bridge

bench-run-collection: ## Run collection benchmarks
	cd bench/pseudobench && $(GO) run . run --collection

bench-run-all: ## Run all benchmarks
	cd bench/pseudobench && $(GO) run . run --all

bench-versions: ## Run version benchmarks
	cd bench/pseudobench && $(GO) run . versions

bench: bench-generate bench-run-all ## Run all benchmarks + copy results to website
	cp bench/pseudobench/results/pseudobench.json website/static/data/pseudobench.json
	@echo "Results copied to website/static/data/pseudobench.json"

# ── Frontend Checks ──────────────────────────────────────────────────────────

frontend-check-all: ## Run lint, format, and typecheck across all frontend projects
	$(MAKE) -C bowrain frontend-check-all

# Forward pulse targets
pulse-build pulse-dev pulse-check:
	$(MAKE) -C bowrain $@

# ── Documentation Assets ────────────────────────────────────────────────────

screenshots recordings bowrain-cli-recordings:
	$(MAKE) -C bowrain $@

kapi-recordings: build ## Generate kapi CLI demo videos (VHS)
	./website/tapes/generate.sh

cli-recordings: kapi-recordings bowrain-cli-recordings ## Generate all CLI demo videos

docs-assets: screenshots recordings cli-recordings ## Generate all documentation assets

fetch-docs-assets: ## Download pre-built docs assets from GitHub release
	@gh release download docs-assets --pattern 'docs-assets.tar.gz' --dir /tmp --clobber
	@mkdir -p website/static
	@tar xzf /tmp/docs-assets.tar.gz -C website/static
	@rm -f /tmp/docs-assets.tar.gz
	@du -sh website/static/img website/static/video 2>/dev/null || true

# ── Polished Videos (Remotion) ──────────────────────────────────────────────

VIDEOS_DIR := website/videos

videos-deps: ; cd $(VIDEOS_DIR) && vp install --frozen-lockfile
videos-setup: videos-deps ; cd $(VIDEOS_DIR) && vp run setup-raw
videos-studio: videos-setup ; cd $(VIDEOS_DIR) && vp run studio
videos-render: videos-setup ## Render all polished demo videos
	cd $(VIDEOS_DIR) && vp run build
	cd $(VIDEOS_DIR) && vp run publish

# ── Generate (scripts at root) ──────────────────────────────────────────────

GITHUB_TOKEN       ?= $(shell gh auth token 2>/dev/null)
OKAPI_VERSION      ?= 1.48.0
OKAPI_SUREFIRE_DIR ?= okapi-surefire/$(OKAPI_VERSION)-v1
GOTEST_JSON_FILE   := coverage/bridge-test-results.jsonl
NATIVE_JSON_FILE   := coverage/native-test-results.jsonl

fetch-okapi-surefire: ## Download Okapi Surefire XML reports
	@GITHUB_TOKEN=$(GITHUB_TOKEN) bash scripts/fetch-okapi-surefire.sh

generate-test-comparison: fetch-okapi-surefire test-bridge-json test-native-json ## Generate test comparison data
	@mkdir -p website/static/data
	$(GO) run ./scripts/testcompare \
		-okapi-dir $(OKAPI_SUREFIRE_DIR) \
		-gotest-bridge-json $(GOTEST_JSON_FILE) \
		-gotest-native-json $(NATIVE_JSON_FILE) \
		-bridge-src core/plugin/bridge/filters \
		-native-src core/formats \
		-out website/static/data/test-comparison.json

generate-format-docs: ## Generate format reference JSON for the website
	$(GO) run ./scripts/gen-format-docs

generate-test-stubs: fetch-okapi-surefire ## Generate Go test stubs from Surefire XML
	$(GO) run ./scripts/gen-test-stubs -surefire-dir $(OKAPI_SUREFIRE_DIR)

# ── Documentation Site ──────────────────────────────────────────────────────

docs-deps: ; cd website && vp install --frozen-lockfile
docs-dev: ; cd website && vp run start
docs-build: ; cd website && vp run build
docs-serve: ; cd website && vp run serve

# ── Tools ────────────────────────────────────────────────────────────────────

tools: ## Install development tools
	$(GO) install google.golang.org/protobuf/cmd/protoc-gen-go@latest
	$(GO) install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
	$(GO) install github.com/golangci/golangci-lint/cmd/golangci-lint@latest

setup-remote: ## Install dependencies for cloud environments
	CLAUDE_CODE_REMOTE=true bash scripts/setup-remote.sh

pre-push: ## Run checks relevant to your changes (mirrors CI)
	@./scripts/pre-push-check.sh

pre-push-all: ## Run all checks regardless of changes
	@./scripts/pre-push-check.sh --all

gha-lint: ## Lint GitHub Actions workflow files
	@command -v actionlint >/dev/null 2>&1 || { echo "actionlint not installed."; exit 1; }
	actionlint

# ── Clean ────────────────────────────────────────────────────────────────────

clean: ## Remove all build artifacts
	rm -rf bin coverage
	@$(MAKE) -C bowrain clean

# ── Help ─────────────────────────────────────────────────────────────────────

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
			printf "  \033[36m%-28s\033[0m %s\n", targets[i], descs[i] \
		} \
		printf "\n" \
	}' $(MAKEFILE_LIST)
	@echo "  Sub-Makefile:  make -C bowrain help"
	@echo ""

.PHONY: all help $(BOTH_TARGETS) test test-fast test-unit test-race test-verbose test-integration \
        fmt vet lint check test-parallel \
        test-framework test-cli test-kapi test-platform test-bowrain-cli test-bowrain \
        verify-isolation \
        build build-all build-server build-worker build-bowrain-cli build-bowrain build-headless \
        install install-bowrain-cli \
        frontend-check-all \
        build-kapi-desktop kapi-desktop-dev kapi-desktop-test \
        kapi-desktop-frontend-deps kapi-desktop-frontend-dev kapi-desktop-frontend-build \
        kapi-desktop-frontend-test kapi-desktop-frontend-check \
        flow-editor-deps flow-editor-check flow-editor-test \
        kapi-desktop-storybook kapi-desktop-storybook-build \
        cover test-e2e test-e2e-kapi test-e2e-bowrain test-e2e-cloud test-e2e-dev \
        fetch-bridge-jar fetch-bridge-testdata test-bridge-filters test-bridge-pool test-bridge-json test-native-json \
        bench bench-build bench-generate bench-run bench-run-bridge bench-run-collection bench-run-all bench-versions \
        screenshots recordings kapi-recordings bowrain-cli-recordings cli-recordings docs-assets fetch-docs-assets \
        videos-deps videos-setup videos-studio videos-render \
        fetch-okapi-surefire generate-test-comparison generate-format-docs generate-test-stubs \
        docs-deps docs-dev docs-build docs-serve \
        tools setup-remote gha-lint clean \
        _fw-test _fw-test-fast _fw-test-unit _fw-test-race _fw-test-verbose _fw-test-integration \
        _fw-vet _fw-lint _fw-proto _fw-deps _fw-deps-update
