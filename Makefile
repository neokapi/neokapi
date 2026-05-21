# neokapi Makefile
# ================
# Framework targets run directly from this root directory.
# Bowrain targets forward to bowrain/Makefile.
#
# Module-specific targets live in:
#   make help              (this file)
#   make -C bowrain help   (bowrain sub-Makefile)
#
# CI mode: GitHub Actions sets CI=true automatically. When CI is set,
# per-module test targets add -race, -coverprofile, -covermode=atomic,
# and -json output. Use `make ci-test-<module>` locally to reproduce.

.DEFAULT_GOAL := help

# ── Shared Variables (exported to sub-makes) ──────────────────────────────────

export ROOT_DIR    := $(shell pwd)
export VERSION     ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
export COMMIT      := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
export BUILD_DATE  := $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
export VERSION_PKG := github.com/neokapi/neokapi/core/version
export LDFLAGS     := -ldflags "-X $(VERSION_PKG).Version=$(VERSION) -X $(VERSION_PKG).Commit=$(COMMIT) -X $(VERSION_PKG).BuildDate=$(BUILD_DATE)"

GO := go
# FTS5 build tag is required by mattn/go-sqlite3 to enable FTS5 full-text
# search. Without it, TM and termbase migrations fail at runtime.
#
# ICU requirement: The FTS5 ICU tokenizer requires ICU development libraries.
#   Linux:  sudo apt-get install libicu-dev pkg-config
#   macOS:  brew install icu4c && export PKG_CONFIG_PATH="/opt/homebrew/opt/icu4c@78/lib/pkgconfig"
GOTAGS  := -tags "fts5"

# macOS Homebrew ICU: expose to pkg-config if not already on the path.
ifeq ($(shell uname -s),Darwin)
export PKG_CONFIG_PATH := /opt/homebrew/opt/icu4c@78/lib/pkgconfig:$(PKG_CONFIG_PATH)
endif
GOTEST  := $(GO) test $(GOTAGS)
GOBUILD := $(GO) build $(GOTAGS)
GOVET   := $(GO) vet $(GOTAGS)
GOFMT   := gofmt
BIN_DIR := $(ROOT_DIR)/bin
COVER_DIR := coverage

GOLANGCI_LINT := $(shell which golangci-lint 2>/dev/null || { test -x "$$(go env GOPATH)/bin/golangci-lint" && echo "$$(go env GOPATH)/bin/golangci-lint"; })

# ── CI auto-detection ────────────────────────────────────────────────────────
# GitHub Actions sets CI=true. When active, test targets add race detection,
# coverage profiling, and JSON output for JUnit reporting.

ifdef CI
  _RACE     := -race
  _COVMODE  := -covermode=atomic
else
  _RACE     :=
  _COVMODE  :=
endif

# Base test command: always shuffles, adds race in CI
GOTEST_BASE := $(GOTEST) $(_RACE) -shuffle=on

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

check-framework: _fw-fmt _fw-vet _fw-lint ## Framework-only quality checks

check-bowrain: ## Bowrain-only quality checks
	@$(MAKE) -C bowrain check

test-parallel: ## Run all tests in parallel
	@$(MAKE) --no-print-directory _fw-test & $(MAKE) -C bowrain test & wait

# ── Framework test/quality internals ────────────────────────────────────────

_fw-fmt:
	$(GOFMT) -w -s core/ cli/ kapi/ sievepen/ termbase/ providers/

_fw-test:
	$(GOTEST_BASE) ./... -count=1
	cd cli && $(GOTEST_BASE) ./... -count=1
	cd kapi && $(GOTEST_BASE) ./... -count=1

_fw-test-fast:
	$(GOTEST_BASE) ./...
	cd cli && $(GOTEST_BASE) ./...
	cd kapi && $(GOTEST_BASE) ./...

_fw-test-unit:
	$(GOTEST_BASE) ./... -count=1 -short
	cd cli && $(GOTEST_BASE) ./... -count=1 -short
	cd kapi && $(GOTEST_BASE) ./... -count=1 -short

_fw-test-race:
	$(GOTEST) -race -shuffle=on ./... -count=1
	cd cli && $(GOTEST) -race -shuffle=on ./... -count=1
	cd kapi && $(GOTEST) -race -shuffle=on ./... -count=1

_fw-test-verbose:
	$(GOTEST_BASE) ./... -count=1 -v
	cd cli && $(GOTEST_BASE) ./... -count=1 -v
	cd kapi && $(GOTEST_BASE) ./... -count=1 -v

_fw-test-integration:
	$(GOTEST_BASE) ./... -count=1 -tags=integration -run Integration

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

_fw-proto: ## Generate framework Go bindings from proto definitions
ifndef PROTOC
	$(error "protoc not found. Install Protocol Buffers compiler.")
endif
ifndef PROTOC_GEN_GO
	$(error "protoc-gen-go not found. Run 'make tools' to install.")
endif
	protoc --go_out=. --go_opt=paths=source_relative \
		--go-grpc_out=. --go-grpc_opt=paths=source_relative \
		core/plugin/proto/v1/*.proto core/plugin/proto/v2/*.proto

_fw-deps:
	$(GO) mod download && $(GO) mod tidy
	cd cli && $(GO) mod download && $(GO) mod tidy
	cd kapi && $(GO) mod download && $(GO) mod tidy

_fw-deps-update:
	$(GO) get -u ./... && $(GO) mod tidy
	cd cli && $(GO) get -u ./... && $(GO) mod tidy
	cd kapi && $(GO) get -u ./... && $(GO) mod tidy

# ── Per-Module Test ─────────────────────────────────────────────────────────
# These targets are CI-aware: when CI=true, they add -race, coverage, and
# JSON output. Locally they run fast with -count=1 only.
# Use `make ci-test-<module>` to reproduce CI behavior locally.

test-framework: ## Run framework module tests only
	@mkdir -p $(COVER_DIR)
ifdef CI
	$(GOTEST_BASE) -coverprofile=$(COVER_DIR)/framework.out $(_COVMODE) -json ./... > test-results-framework.json
else
	$(GOTEST_BASE) ./... -count=1
endif

test-cli: ## Run cli module tests only
	@mkdir -p $(COVER_DIR)
ifdef CI
	cd cli && $(GOTEST_BASE) -coverprofile=../$(COVER_DIR)/cli.out $(_COVMODE) -json ./... > ../test-results-cli.json
else
	cd cli && $(GOTEST_BASE) ./... -count=1
endif

test-kapi: ## Run kapi CLI tests only
	@mkdir -p $(COVER_DIR)
ifdef CI
	cd kapi && $(GOTEST_BASE) -coverprofile=../$(COVER_DIR)/kapi.out $(_COVMODE) -json ./... > ../test-results-kapi.json
else
	cd kapi && $(GOTEST_BASE) ./... -count=1
endif

test-platform test-bowrain-cli test-bowrain: ## Run individual bowrain module tests
	$(MAKE) -C bowrain $@

# ── CI-equivalent targets (for local reproduction) ──────────────────────────

ci-test-framework: ## Run framework tests with full CI flags locally
	$(MAKE) CI=true test-framework

ci-test-cli: ## Run cli tests with full CI flags locally
	$(MAKE) CI=true test-cli

ci-test-kapi: ## Run kapi tests with full CI flags locally
	$(MAKE) CI=true test-kapi

ci-test-platform: ## Run platform tests with full CI flags locally
	$(MAKE) -C bowrain CI=true test-platform

ci-test-bowrain-cli: ## Run Bowrain CLI tests with full CI flags locally
	$(MAKE) -C bowrain CI=true test-bowrain-cli

ci-test-bowrain: ## Run bowrain tests with full CI flags locally
	$(MAKE) -C bowrain CI=true test-bowrain

ci-test-kapi-desktop: ## Run Kapi Desktop tests with full CI flags locally
	$(MAKE) CI=true kapi-desktop-test

ci-test-all: ## Run all module tests with full CI flags locally
	$(MAKE) CI=true test-framework test-cli test-kapi kapi-desktop-test
	$(MAKE) -C bowrain CI=true test-platform test-bowrain-cli test-bowrain

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

# ── Parity (head-to-head against okapi-bridge) ──────────────────────────────
#
# `make parity-test` is the load-bearing safety net for issue #448:
# it builds a sandboxed kapi + okapi-bridge install and runs every
# parity test under cli/parity/... against that sandbox. The sandbox
# lives under .parity/ and is reused across runs (set PARITY_FORCE=1
# to rebuild).
#
# Set OKAPI_BRIDGE_REPO if your okapi-bridge clone is not at ../okapi-bridge.
# Set OKAPI_VERSION (default 1.48.0) to test against a different release.

PARITY_DIR := $(ROOT_DIR)/.parity
PARITY_REPORT := $(PARITY_DIR)/test-comparison.json

parity-sandbox: ## Build the parity sandbox (kapi + okapi-bridge plugin)
	@$(ROOT_DIR)/scripts/parity-sandbox.sh > /dev/null
	@echo "Parity sandbox: $(PARITY_DIR)"

# Tikal third corner: when the Okapi clone has a built tikal jar, the
# harness invokes it via .parity/tikal/tikal.sh — wired here so each
# parity-test run regenerates the launcher pointing at the current
# OKAPI_REPO. Tests skip gracefully when the jar isn't present.
TIKAL_LAUNCHER := $(PARITY_DIR)/tikal/tikal.sh
TIKAL_JAR_GLOB := $(OKAPI_REPO)/applications/tikal/target/okapi-application-tikal-*.jar

parity-test: parity-sandbox ## Run the full parity test suite (#448)
	@TIKAL_ENV=""; \
	if ls $(TIKAL_JAR_GLOB) >/dev/null 2>&1; then \
	    mkdir -p $(PARITY_DIR)/tikal; \
	    ln -sfn $(OKAPI_REPO)/applications/tikal/target/lib $(PARITY_DIR)/tikal/lib; \
	    printf '#!/bin/bash\nexec java -cp "%s:%s/lib/*" net.sf.okapi.applications.tikal.Main "$$@"\n' \
	        "$$(ls $(TIKAL_JAR_GLOB) | grep -v -- '-tests\.jar' | head -1)" \
	        "$(PARITY_DIR)/tikal" > $(TIKAL_LAUNCHER); \
	    chmod +x $(TIKAL_LAUNCHER); \
	    TIKAL_ENV="OKAPI_TIKAL=$(TIKAL_LAUNCHER)"; \
	    echo "[parity] tikal third corner enabled via $(TIKAL_LAUNCHER)"; \
	else \
	    echo "[parity] tikal not built at $$OKAPI_REPO — third-corner comparison will skip"; \
	fi; \
	cd cli && env $$TIKAL_ENV KAPI_PARITY_SANDBOX=$(PARITY_DIR) KAPI_PARITY_REPORT=$(PARITY_REPORT) \
	    $(GOTEST) -tags parity -count=1 -timeout 60m ./parity/...
	@echo "Parity report: $(PARITY_REPORT)"

PARITY_DASHBOARD := $(ROOT_DIR)/web/docs/static/data/parity-report.json
PARITY_FIXTURES_JSON := $(ROOT_DIR)/web/docs/static/data/parity-fixtures.json

parity-publish: parity-test ## Run the parity suite and publish dashboard JSON to the docs site
	@cd $(ROOT_DIR) && go run ./scripts/testcompare \
	    -in $(PARITY_REPORT) \
	    -out $(PARITY_DASHBOARD)
	@echo "Dashboard JSON: $(PARITY_DASHBOARD)"

parity-fixtures: ## Run the round-trip coverage suite and emit per-fixture JSON for /parity/fixtures
	@cd $(ROOT_DIR) && PARITY_FIXTURES_JSON=$(PARITY_FIXTURES_JSON) \
	    $(GOTEST) -tags parity -count=1 -timeout 30m -run TestRoundTrip_Coverage ./cli/parity/roundtrip/
	@echo "Fixtures JSON: $(PARITY_FIXTURES_JSON)"

parity-clean: ## Remove the parity sandbox to force a fresh build next run
	rm -rf $(PARITY_DIR)

# ── Contract audit (Okapi @Test methods → 4-state coverage view) ────────────
#
# `make contract-audit` is the evolution-tolerant counterpart to
# `make parity-test`: it treats the upstream Okapi Java filter tests
# as the canonical contract list, runs `mvn test` (or reuses cached
# Surefire XMLs) plus `go test -json` on the matching native packages,
# scans for `// okapi: ClassName#methodName` annotations next to Go
# tests, and emits the JSON the /contract-audit dashboard renders.
#
# Set OKAPI_REPO if your Okapi clone is not at /Users/asgeirf/src/okapi/Okapi.
# Set CONTRACT_FILTER to scope to a single filter (default: html).
#
# Canonical Okapi clone is ~/src/okapi/Okapi (cleanly tagged v1.48.0, matching
# the okapi-bridge framework_version). The older ~/src/okapi/okapi-java clone
# is stuck on a stale `v1.4.8` tag and mislabels the dashboard version — do not
# use it for the contract audit (#611).

CONTRACT_DIR             := $(ROOT_DIR)/.contract-audit
CONTRACT_REPORT          := $(ROOT_DIR)/web/docs/static/data/contract-audit.json
CONTRACT_FILTER          ?= html
OKAPI_REPO               ?= /Users/asgeirf/src/okapi/Okapi
BRIDGE_SCHEMAS           ?= $(ROOT_DIR)/../okapi-bridge/schemas
PARITY_REPORT            ?= $(ROOT_DIR)/.parity/test-comparison.json
# Maven Failsafe reports for Okapi's *IT integration tests (roundtrip /
# xliff-compare per filter), generated by `mvn verify` in the
# integration-tests/okapi module. When present, these join each filter's
# contract rows so the dashboard reflects Okapi's integration-test
# coverage too (#611). Generate with `make okapi-failsafe-reports`.
CONTRACT_FAILSAFE        ?= $(OKAPI_REPO)/integration-tests/okapi/target/failsafe-reports
# Set CONTRACT_FAIL_ON_DRIFT=1 to fail the audit when any // okapi:
# annotation references a Java class/method not present in the pinned
# Okapi Surefire output. CI sets this; locally it is opt-in.
CONTRACT_FAIL_ON_DRIFT   ?=

contract-audit: ## Generate the contract-audit dashboard JSON for $(CONTRACT_FILTER)
	@mkdir -p $(CONTRACT_DIR)
	@if [ ! -d $(OKAPI_REPO)/okapi/filters/$(CONTRACT_FILTER)/target/surefire-reports ]; then \
	    echo "[contract-audit] no Surefire output for $(CONTRACT_FILTER); running mvn test..."; \
	    cd $(OKAPI_REPO)/okapi/filters/$(CONTRACT_FILTER) && mvn -B test; \
	fi
	@echo "[contract-audit] running native go test for $(CONTRACT_FILTER)..."
	@cd $(ROOT_DIR) && go test -json ./core/formats/$(CONTRACT_FILTER)/... > $(CONTRACT_DIR)/native-$(CONTRACT_FILTER).json 2>/dev/null || true
	@cd $(ROOT_DIR) && go run ./scripts/contract-audit \
	    -okapi-surefire $(OKAPI_REPO)/okapi/filters/$(CONTRACT_FILTER)/target/surefire-reports \
	    -native-gotest $(CONTRACT_DIR)/native-$(CONTRACT_FILTER).json \
	    -native-src core/formats/$(CONTRACT_FILTER) \
	    $(if $(wildcard $(PARITY_REPORT)),-parity-report $(PARITY_REPORT),) \
	    $(if $(wildcard $(BRIDGE_SCHEMAS)),-bridge-schemas $(BRIDGE_SCHEMAS),) \
	    $(if $(CONTRACT_FAIL_ON_DRIFT),-fail-on-drift,) \
	    -okapi-version $$(cd $(OKAPI_REPO) && git describe --tags --abbrev=0 2>/dev/null || echo dev) \
	    -okapi-tag $$(cd $(OKAPI_REPO) && git describe --tags --abbrev=0 2>/dev/null || echo HEAD) \
	    -go-commit $$(git rev-parse --short HEAD) \
	    -out $(CONTRACT_REPORT)
	@echo "Contract audit: $(CONTRACT_REPORT)"

# Filters whose Surefire output exists and whose neokapi side has at
# least a config.go OR surviving // okapi: annotations. The script
# handles missing pieces gracefully (a filter with Okapi tests but no
# native package shows 0% / all-unmapped).
CONTRACT_FILTERS_ALL := archive doxygen dtd epub html icml idml json markdown messageformat mif mosestext openxml \
                        pdf plaintext po properties regex rtf tex tmx ts ttx txml transtable vignette vtt wiki xliff \
                        xliff2 yaml php xmlstream

contract-audit-all: ## Generate the dashboard for every filter with cached Surefire output
	@mkdir -p $(CONTRACT_DIR)
	@echo "[contract-audit] running native go test across all core/formats packages..."
	@# Run (and scan annotations for) every native format package, not a
	@# curated subset — otherwise formats absent from the list undercount as
	@# unmapped even when they carry // okapi: annotations (#611). go test
	@# parallelises package compilation/execution on its own.
	@cd $(ROOT_DIR) && go test -json ./core/formats/... > $(CONTRACT_DIR)/native-all.json 2>/dev/null || true
	@echo "[contract-audit] joining surefire + native + annotations..."
	@cd $(ROOT_DIR) && go run ./scripts/contract-audit \
	    -okapi-surefire $(OKAPI_REPO)/okapi/filters \
	    $(if $(wildcard $(CONTRACT_FAILSAFE)),-okapi-failsafe $(CONTRACT_FAILSAFE),) \
	    -native-gotest $(CONTRACT_DIR)/native-all.json \
	    -native-src core/formats \
	    $(if $(wildcard $(PARITY_REPORT)),-parity-report $(PARITY_REPORT),) \
	    $(if $(wildcard $(BRIDGE_SCHEMAS)),-bridge-schemas $(BRIDGE_SCHEMAS),) \
	    $(if $(CONTRACT_FAIL_ON_DRIFT),-fail-on-drift,) \
	    -okapi-version $$(cd $(OKAPI_REPO) && git describe --tags --abbrev=0 2>/dev/null || echo dev) \
	    -okapi-tag $$(cd $(OKAPI_REPO) && git describe --tags --abbrev=0 2>/dev/null || echo HEAD) \
	    -go-commit $$(git rev-parse --short HEAD) \
	    -out $(CONTRACT_REPORT)
	@echo "Contract audit: $(CONTRACT_REPORT)"

okapi-failsafe-reports: ## Generate Okapi *IT integration-test (Failsafe) reports for the contract audit
	@echo "[contract-audit] generating Okapi Failsafe IT reports (integration-tests/okapi)..."
	cd $(OKAPI_REPO) && mvn -q -pl integration-tests/okapi -am \
	    -DfailIfNoTests=false -Dmaven.test.failure.ignore=true \
	    -Dfailsafe.failIfNoSpecifiedTests=false \
	    test-compile failsafe:integration-test
	@echo "Failsafe reports: $(CONTRACT_FAILSAFE)"

contract-audit-clean: ## Remove the contract-audit working directory
	rm -rf $(CONTRACT_DIR)

# ── Build ────────────────────────────────────────────────────────────────────

build: ## Build the kapi CLI (Apache-2.0; manifest-driven plugins discovered at runtime)
	@mkdir -p $(BIN_DIR)
	cd kapi && $(GOBUILD) $(LDFLAGS) -o $(BIN_DIR)/kapi ./cmd/kapi

build-bowrain-plugin: ## Build the kapi-bowrain plugin binary (manifest-driven)
	@mkdir -p $(BIN_DIR)
	cd bowrain/cli && $(GOBUILD) $(LDFLAGS) -o $(BIN_DIR)/kapi-bowrain ./cmd/kapi-bowrain

build-all: ## Build all Go binaries
	@mkdir -p $(BIN_DIR)
	cd kapi && $(GOBUILD) $(LDFLAGS) -o $(BIN_DIR)/kapi ./cmd/kapi
	cd bowrain/cli && $(GOBUILD) $(LDFLAGS) -o $(BIN_DIR)/kapi-bowrain ./cmd/kapi-bowrain
	$(MAKE) -C bowrain build-server build-worker build-kapi-bowrain-plugin

# Forward bowrain build targets
build-server build-worker build-kapi-bowrain-plugin build-bowrain build-headless install-kapi-bowrain-plugin:
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
	cd $(KAPI_DESKTOP_DIR) && $(GOTEST_BASE) ./backend/... -count=1 -timeout 60s

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

kapi-desktop-extract: kapi-desktop-frontend-deps ## Extract translatable blocks to i18n/ (per-file .klf)
	cd $(KAPI_DESKTOP_DIR)/frontend && vp run extract

kapi-desktop-pseudo-translate: kapi-desktop-extract bin/kapi ## Pseudo-translate i18n/ → i18n-qps/
	./bin/kapi pseudo-translate $(KAPI_DESKTOP_DIR)/frontend/i18n \
		--target-lang qps \
		-o $(KAPI_DESKTOP_DIR)/frontend/i18n-qps \
		-q

kapi-desktop-compile: ## Compile i18n/ → public/translations/<locale>.json for the kapi-react runtime
	cd $(KAPI_DESKTOP_DIR)/frontend && vp run compile

kapi-desktop-translations: kapi-desktop-pseudo-translate kapi-desktop-compile ## Extract → pseudo-translate → compile

kapi-i18n-generate: ## Regenerate core/i18n/builtins/metadata.json from Go registries
	go generate ./core/i18n/...

kapi-i18n-pseudo-translate: kapi-i18n-generate bin/kapi ## Pseudo-translate builtins into core/i18n/catalogs/qps.mo
	./bin/kapi pseudo-translate core/i18n/builtins/metadata.json \
		--target-lang qps \
		-f json \
		-o core/i18n/catalogs/qps.mo \
		-q

kapi-i18n-translations: kapi-i18n-pseudo-translate ## Regenerate + pseudo-translate builtin metadata → MO

storybook-fixtures: ## Generate Storybook fixtures from real format/tool data
	@./scripts/gen-storybook-fixtures.sh

flow-editor-deps: ## Install flow-editor dependencies
	cd packages/flow-editor && vp install

flow-editor-check: flow-editor-deps ## Lint + format + typecheck flow-editor package
	cd packages/flow-editor && vp check

flow-editor-test: flow-editor-deps ## Run flow-editor tests
	cd packages/flow-editor && vp test

kapi-storybook: ## Run Kapi Storybook (port 6007)
	cd storybook && vpx storybook dev -p 6007

kapi-storybook-build: ## Build Kapi Storybook
	cd storybook && vpx storybook build -o storybook-static

bowrain-storybook: ## Run Bowrain Storybook (port 6006)
	$(MAKE) -C bowrain storybook

bowrain-storybook-build: ## Build Bowrain Storybook
	$(MAKE) -C bowrain storybook-build

install: ## Install kapi CLI to GOPATH/bin
	cd kapi && $(GO) install $(LDFLAGS) ./cmd/kapi

# ── Coverage ─────────────────────────────────────────────────────────────────

cover: ## Run tests with coverage (merged report)
	@mkdir -p $(COVER_DIR)
	$(GOTEST_BASE) -coverprofile=$(COVER_DIR)/framework.out $(_COVMODE) ./... -count=1
	cd cli && $(GOTEST_BASE) -coverprofile=../$(COVER_DIR)/cli.out $(_COVMODE) ./... -count=1
	cd kapi && $(GOTEST_BASE) -coverprofile=../$(COVER_DIR)/kapi.out $(_COVMODE) ./... -count=1
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

# ── Bench (composite target at root) ───────────────────────────────────────
#
# `make bench` is the canonical regen path used to publish PseudoBench
# results to the docs site. It depends on a built parity sandbox so the
# kapi binary, okapi-bridge launcher, and okapi-testdata corpus are all
# in place; without them the corpus discovery has nothing to walk.

PSEUDOBENCH_BIN     ?= bench/pseudobench/pseudobench
PSEUDOBENCH_RESULTS ?= bench/pseudobench/results
PSEUDOBENCH_TRACES  ?= bench/pseudobench/traces
PSEUDOBENCH_SAMPLE  ?= 0.10
PSEUDOBENCH_ITERS   ?= 3
PSEUDOBENCH_WARMUP  ?= 1
OKAPI_VERSION       ?= 1.48.0

bench-build: ## Build pseudobench binary
	cd bench/pseudobench && GOWORK=off $(GOBUILD) -o pseudobench .

bench-run: bench-build parity-sandbox ## Run pseudobench against parity sandbox (writes JSON + HTML + traces)
	@rm -rf $(PSEUDOBENCH_RESULTS) $(PSEUDOBENCH_TRACES)
	@# The jpackage app-image bundles the bridge fat-jar at app/. Pass it
	@# to pseudobench so it can also start a long-lived daemon and benchmark
	@# kapi-bridge-daemon alongside the per-call subprocess mode.
	$(eval BRIDGE_JAR := $(firstword $(wildcard $(PARITY_DIR)/plugins/okapi-bridge/Contents/app/neokapi-bridge-*-jar-with-dependencies.jar)))
	@if [ -z "$(BRIDGE_JAR)" ]; then echo "[bench] no bridge jar found under $(PARITY_DIR)/plugins/okapi-bridge/Contents/app — daemon engine will be skipped"; fi
	$(PSEUDOBENCH_BIN) run \
	    -kapi $(PARITY_DIR)/bin/kapi \
	    -okapi-bridge $(PARITY_DIR)/plugins/okapi-bridge/Contents/MacOS/kapi-okapi-bridge \
	    $(if $(BRIDGE_JAR),-bridge-jar $(BRIDGE_JAR),) \
	    -okapi-testdata $(PARITY_DIR)/okapi-testdata/$(OKAPI_VERSION) \
	    -sample $(PSEUDOBENCH_SAMPLE) \
	    -iterations $(PSEUDOBENCH_ITERS) \
	    -warmup $(PSEUDOBENCH_WARMUP) \
	    -results $(PSEUDOBENCH_RESULTS) \
	    -output $(PSEUDOBENCH_RESULTS)/output \
	    -html $(PSEUDOBENCH_RESULTS)/pseudobench.html \
	    -trace-dir $(PSEUDOBENCH_TRACES)

bench-run-full: PSEUDOBENCH_SAMPLE := 1.0
bench-run-full: bench-run ## Run pseudobench across the full corpus (slow)

bench: bench-run ## Regenerate pseudobench data and publish to web/docs/static/data
	cp $(PSEUDOBENCH_RESULTS)/pseudobench.json web/docs/static/data/pseudobench.json
	@echo "Published $(PSEUDOBENCH_RESULTS)/pseudobench.json → web/docs/static/data/pseudobench.json"

# `make bench-stress` is the publish-grade run: full 844-fixture corpus,
# 3 measurement iterations + 1 warmup. On M1 Max takes ~30-40 min mostly
# in the per-file trace pass — each kapi invocation pays JVM startup,
# and bridge subprocess + okapi multiply that by 844. Use this when
# refreshing the website data; use plain `make bench` (10% sample) for
# quick local iteration.
bench-stress: PSEUDOBENCH_SAMPLE := 1.0
bench-stress: PSEUDOBENCH_ITERS  := 3
bench-stress: PSEUDOBENCH_WARMUP := 1
bench-stress: bench-run ## Stress-run full corpus and publish to docs (slow, ~30-40 min)
	cp $(PSEUDOBENCH_RESULTS)/pseudobench.json web/docs/static/data/pseudobench.json
	@echo "Published $(PSEUDOBENCH_RESULTS)/pseudobench.json → web/docs/static/data/pseudobench.json"

# ── Frontend Checks ──────────────────────────────────────────────────────────

frontend-check-all: ## Run lint, format, and typecheck across all frontend projects
	$(MAKE) -C bowrain frontend-check-all

# Forward pulse targets
pulse-build pulse-dev pulse-check:
	$(MAKE) -C bowrain $@

# ── Documentation Assets ────────────────────────────────────────────────────
#
# Walkthrough engine (issue #425): scenes are recorded by docs-kapi.yml /
# docs-bowrain.yml workflows from web/docs/scenes/ and bowrain/web/docs/scenes/.
# The legacy screenshots/recordings/cli-recordings/docs-assets/Remotion
# pipeline is removed — see commit history for what was here.

fetch-docs-assets: ## Download legacy docs assets (transitional, until walkthrough engine fully covers)
	@gh release download docs-assets --pattern 'docs-assets.tar.gz' --dir /tmp --clobber
	@mkdir -p web/docs/static
	@tar xzf /tmp/docs-assets.tar.gz -C web/docs/static
	@rm -f /tmp/docs-assets.tar.gz
	@du -sh web/docs/static/img web/docs/static/video 2>/dev/null || true

# ── Generate (scripts at root) ──────────────────────────────────────────────

generate-format-docs: ## Generate format reference JSON for the website
	$(GO) run ./scripts/gen-format-docs

# ── Documentation Site ──────────────────────────────────────────────────────

docs-deps: ; cd web/docs && vp install --frozen-lockfile
docs-dev: ; cd web/docs && vp run start
docs-build: ; cd web/docs && vp run build
docs-serve: ; cd web/docs && vp run serve

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
        parity-sandbox parity-test parity-publish parity-clean \
        contract-audit contract-audit-all contract-audit-clean okapi-failsafe-reports \
        fmt vet lint check check-framework check-bowrain test-parallel \
        test-framework test-cli test-kapi test-platform test-bowrain-cli test-bowrain \
        ci-test-framework ci-test-cli ci-test-kapi ci-test-platform \
        ci-test-bowrain-cli ci-test-bowrain ci-test-kapi-desktop ci-test-all \
        verify-isolation \
        build build-all build-server build-worker build-kapi-bowrain-plugin build-bowrain-plugin build-bowrain build-headless \
        install install-kapi-bowrain-plugin \
        frontend-check-all \
        build-kapi-desktop kapi-desktop-dev kapi-desktop-test \
        kapi-desktop-frontend-deps kapi-desktop-frontend-dev kapi-desktop-frontend-build \
        kapi-desktop-frontend-test kapi-desktop-frontend-check kapi-desktop-extract \
        kapi-desktop-pseudo-translate kapi-desktop-compile kapi-desktop-translations \
        kapi-i18n-generate kapi-i18n-pseudo-translate kapi-i18n-translations \
        flow-editor-deps flow-editor-check flow-editor-test \
        kapi-storybook kapi-storybook-build bowrain-storybook bowrain-storybook-build \
        cover test-e2e test-e2e-kapi test-e2e-bowrain test-e2e-cloud test-e2e-dev \
        bench bench-build bench-generate bench-run bench-run-collection bench-run-all bench-versions \
        fetch-docs-assets \
        generate-format-docs \
        docs-deps docs-dev docs-build docs-serve \
        tools setup-remote gha-lint clean \
        _fw-fmt _fw-test _fw-test-fast _fw-test-unit _fw-test-race _fw-test-verbose _fw-test-integration \
        _fw-vet _fw-lint _fw-proto _fw-deps _fw-deps-update
