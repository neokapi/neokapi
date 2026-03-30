# neokapi Makefile
# ================
# Orchestrates framework/ and platform/ sub-makes.
#
# Targets that exist in both sub-Makefiles (test, lint, fmt, etc.) are
# forwarded automatically via the SUBDIRS convention.
#
# Module-specific targets live in the sub-Makefiles:
#   make -C framework help
#   make -C platform help

.DEFAULT_GOAL := help

SUBDIRS := framework platform

# ── Shared Variables (exported to sub-makes) ──────────────────────────────────

export ROOT_DIR    := $(shell pwd)
export VERSION     ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
export COMMIT      := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
export BUILD_DATE  := $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
export VERSION_PKG := github.com/neokapi/neokapi/core/version
export LDFLAGS     := -ldflags "-X $(VERSION_PKG).Version=$(VERSION) -X $(VERSION_PKG).Commit=$(COMMIT) -X $(VERSION_PKG).BuildDate=$(BUILD_DATE)"

GO  := go
NPM := npm

# ── SUBDIRS convention ───────────────────────────────────────────────────────
# Targets listed here are forwarded to both framework/ and platform/ Makefiles
# in sequence. Each sub-Makefile defines the actual rules.

BOTH_TARGETS := test test-fast test-unit test-race test-verbose test-integration \
                fmt vet lint check proto deps deps-update

$(BOTH_TARGETS):
	@for dir in $(SUBDIRS); do $(MAKE) -C $$dir $@; done

test-parallel: ## Run all tests in parallel
	@$(MAKE) -C framework test & $(MAKE) -C platform test & wait

# ── Per-Module Test (forwarded to sub-Makefiles) ─────────────────────────────

test-framework test-cli test-kapi: ## Run individual framework module tests
	$(MAKE) -C framework $@

test-platform test-bowrain-cli test-bowrain: ## Run individual platform module tests
	$(MAKE) -C platform $@

# ── Module Isolation ──────────────────────────────────────────────────────────

verify-isolation: ## Verify all Go module isolation boundaries
	GOWORK=off bash -c "cd framework && go build ./..."
	GOWORK=off bash -c "cd framework/cli && go build ./..."
	GOWORK=off bash -c "cd platform/core && go build ./..."
	GOWORK=off bash -c "cd framework/kapi && go build ./..."
	GOWORK=off bash -c "cd platform/cli && go build ./..."
	@# kapi must not depend on platform
	@if cd framework/kapi && GOWORK=off go list -m all 2>/dev/null | grep -q 'neokapi/platform'; then echo "ERROR: kapi depends on platform"; exit 1; fi
	@# bowrain must not depend on cli
	@if cd platform && GOWORK=off go list -m all 2>/dev/null | grep -q 'neokapi/cli'; then echo "ERROR: bowrain depends on cli"; exit 1; fi
	@# kapi must not have heavy deps
	@if cd framework/kapi && GOWORK=off go list -m all 2>/dev/null | grep -iE 'wails|echo|oidc|keyring'; then echo "ERROR: kapi has heavy deps"; exit 1; fi

# ── Build ────────────────────────────────────────────────────────────────────

build: ## Build the kapi CLI
	$(MAKE) -C framework build

build-all: ## Build all Go binaries
	$(MAKE) -C framework build
	$(MAKE) -C platform build-server build-worker build-bowrain-cli

# Forward platform build targets
build-server build-worker build-bowrain-cli build-bowrain build-headless install-bowrain-cli:
	$(MAKE) -C platform $@

# ── Kapi Desktop ────────────────────────────────────────────────────────────

KAPI_DESKTOP_DIR := framework/apps/kapi-desktop
FRAMEWORK_NODE_OPTS := NODE_OPTIONS="--import tsx/esm"

build-kapi-desktop: kapi-desktop-frontend-build ## Build the Kapi Desktop app
	cd $(KAPI_DESKTOP_DIR) && wails3 build

kapi-desktop-dev: kapi-desktop-frontend-deps ## Run Kapi Desktop in dev mode (hot reload)
	cd $(KAPI_DESKTOP_DIR) && wails3 dev

kapi-desktop-test: ## Run Kapi Desktop Go backend tests
	cd $(KAPI_DESKTOP_DIR) && $(GO) test ./backend/... -count=1

framework-deps: ## Install framework workspace dependencies (packages + kapi-desktop frontend)
	cd framework && npm install

kapi-desktop-frontend-deps: framework-deps ## Install Kapi Desktop frontend dependencies

kapi-desktop-frontend-dev: kapi-desktop-frontend-deps ## Start Kapi Desktop frontend dev server
	cd $(KAPI_DESKTOP_DIR)/frontend && $(FRAMEWORK_NODE_OPTS) npx vp dev --port 5174 --strictPort

kapi-desktop-frontend-build: kapi-desktop-frontend-deps ## Build Kapi Desktop frontend for production
	cd $(KAPI_DESKTOP_DIR)/frontend && $(FRAMEWORK_NODE_OPTS) npx vp build

kapi-desktop-frontend-test: kapi-desktop-frontend-deps ## Run Kapi Desktop frontend tests
	cd $(KAPI_DESKTOP_DIR)/frontend && $(FRAMEWORK_NODE_OPTS) npx vp test

kapi-desktop-frontend-check: kapi-desktop-frontend-deps ## Lint + format + typecheck Kapi Desktop frontend
	cd $(KAPI_DESKTOP_DIR)/frontend && $(FRAMEWORK_NODE_OPTS) npx vp check

flow-editor-deps: ## Install flow-editor dependencies
	cd packages/flow-editor && npm install

flow-editor-check: flow-editor-deps ## Lint + format + typecheck flow-editor package
	cd packages/flow-editor && $(FRAMEWORK_NODE_OPTS) npx vp check

flow-editor-test: flow-editor-deps ## Run flow-editor tests
	cd packages/flow-editor && $(FRAMEWORK_NODE_OPTS) npx vp test

kapi-desktop-storybook: kapi-desktop-frontend-deps ## Run Kapi Desktop Storybook (port 6007)
	cd $(KAPI_DESKTOP_DIR)/frontend && npx storybook dev -p 6007

kapi-desktop-storybook-build: kapi-desktop-frontend-deps ## Build Kapi Desktop Storybook
	cd $(KAPI_DESKTOP_DIR)/frontend && npx storybook build -o storybook-static

install: ## Install kapi CLI to GOPATH/bin
	$(MAKE) -C framework install

# ── Coverage ─────────────────────────────────────────────────────────────────

COVER_DIR := coverage

cover: ## Run tests with coverage (merged report)
	@mkdir -p $(COVER_DIR)
	@for dir in $(SUBDIRS); do $(MAKE) -C $$dir cover; done
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
	$(MAKE) -C framework test-e2e-kapi
	$(MAKE) -C platform test-e2e-bowrain

test-e2e-kapi: ; $(MAKE) -C framework $@
test-e2e-bowrain: ; $(MAKE) -C platform $@
test-e2e-cloud: ; $(MAKE) -C platform $@ ## Run cloud e2e tests against a live server
test-e2e-dev: ; $(MAKE) -C platform $@ ## Run cloud e2e tests against dev environment

# ── Bridge Tests (forward to framework) ──────────────────────────────────────

fetch-bridge-jar fetch-bridge-testdata test-bridge-filters test-bridge-pool test-bridge-json test-native-json:
	$(MAKE) -C framework $@

# ── Bench (forward to framework, composite target at root) ───────────────────

bench-build bench-generate bench-run bench-run-bridge bench-run-collection bench-run-all bench-versions:
	$(MAKE) -C framework $@

bench: bench-generate bench-run-all ## Run all benchmarks + copy results to website
	cp framework/bench/pseudobench/results/pseudobench.json website/static/data/pseudobench.json
	@echo "Results copied to website/static/data/pseudobench.json"

# ── Frontend Checks ──────────────────────────────────────────────────────────

frontend-check-all: ## Run lint, format, and typecheck across all frontend projects
	$(MAKE) -C platform frontend-check-all

# Forward pulse targets
pulse-build pulse-dev pulse-check:
	$(MAKE) -C platform $@

# ── Documentation Assets ────────────────────────────────────────────────────

screenshots recordings bowrain-cli-recordings:
	$(MAKE) -C platform $@

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

videos-deps: ; cd $(VIDEOS_DIR) && $(NPM) ci
videos-setup: videos-deps ; cd $(VIDEOS_DIR) && $(NPM) run setup-raw
videos-studio: videos-setup ; cd $(VIDEOS_DIR) && $(NPM) run studio
videos-render: videos-setup ## Render all polished demo videos
	cd $(VIDEOS_DIR) && $(NPM) run build
	cd $(VIDEOS_DIR) && $(NPM) run publish

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
		-bridge-src framework/core/plugin/bridge/filters \
		-native-src framework/core/formats \
		-out website/static/data/test-comparison.json

generate-format-docs: ## Generate format reference JSON for the website
	$(GO) run ./scripts/gen-format-docs

generate-test-stubs: fetch-okapi-surefire ## Generate Go test stubs from Surefire XML
	$(GO) run ./scripts/gen-test-stubs -surefire-dir $(OKAPI_SUREFIRE_DIR)

# ── Documentation Site ──────────────────────────────────────────────────────

docs-deps: ; cd website && $(NPM) ci
docs-dev: ; cd website && $(NPM) start
docs-build: ; cd website && $(NPM) run build
docs-serve: ; cd website && $(NPM) run serve

# ── Tools ────────────────────────────────────────────────────────────────────

tools: ## Install development tools
	$(GO) install google.golang.org/protobuf/cmd/protoc-gen-go@latest
	$(GO) install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
	$(GO) install github.com/golangci/golangci-lint/cmd/golangci-lint@latest

setup-remote: ## Install dependencies for cloud environments
	CLAUDE_CODE_REMOTE=true bash scripts/setup-remote.sh

gha-lint: ## Lint GitHub Actions workflow files
	@command -v actionlint >/dev/null 2>&1 || { echo "actionlint not installed."; exit 1; }
	actionlint

# ── Clean ────────────────────────────────────────────────────────────────────

clean: ## Remove all build artifacts
	rm -rf bin coverage
	@for dir in $(SUBDIRS); do $(MAKE) -C $$dir clean; done

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
	@echo "  Sub-Makefiles:  make -C framework help  |  make -C platform help"
	@echo ""

.PHONY: all help $(BOTH_TARGETS) test-parallel \
        test-framework test-cli test-kapi test-platform test-bowrain-cli test-bowrain \
        verify-isolation \
        build build-all build-server build-worker build-bowrain-cli build-bowrain build-headless \
        install install-bowrain-cli \
        frontend-check-all \
        build-kapi-desktop kapi-desktop-dev kapi-desktop-test \
        framework-deps \
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
        tools setup-remote gha-lint clean
