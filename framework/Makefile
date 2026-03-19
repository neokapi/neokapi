# framework/Makefile
# =================
# Build, test, and lint targets for the neokapi framework, CLI, and kapi modules.
# Can be used standalone (cd framework && make test) or delegated from root.

# ── Shared Variables (inherit from root or compute standalone) ────────────────

ROOT_DIR    ?= $(shell cd .. && pwd)
VERSION     ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT      ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_DATE  ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
VERSION_PKG ?= github.com/neokapi/neokapi/core/version
LDFLAGS     ?= -ldflags "-X $(VERSION_PKG).Version=$(VERSION) -X $(VERSION_PKG).Commit=$(COMMIT) -X $(VERSION_PKG).BuildDate=$(BUILD_DATE)"

GO          := go
GOTEST      := $(GO) test
GOBUILD     := $(GO) build
GOVET       := $(GO) vet
GOFMT       := gofmt
NPM         := npm
BIN_DIR     := $(ROOT_DIR)/bin
COVER_DIR   := $(ROOT_DIR)/coverage

BRIDGE_PROTO_DIR := core/plugin/proto/v2

# Tools
GOLANGCI_LINT := $(shell which golangci-lint 2>/dev/null || { test -x "$$(go env GOPATH)/bin/golangci-lint" && echo "$$(go env GOPATH)/bin/golangci-lint"; })
PROTOC        := $(shell which protoc 2>/dev/null)
PROTOC_GEN_GO := $(shell which protoc-gen-go 2>/dev/null)

# Bridge / Okapi
GITHUB_TOKEN         ?= $(shell gh auth token 2>/dev/null)
OKAPI_BRIDGE_VERSION ?= v2.17.0
OKAPI_VERSION        ?= 1.48.0
BRIDGE_JAR           := $(HOME)/.cache/neokapi/bridge/$(OKAPI_BRIDGE_VERSION)-okapi$(OKAPI_VERSION)/okapi-bridge.jar
GOTEST_JSON_FILE     := $(COVER_DIR)/bridge-test-results.jsonl
NATIVE_JSON_FILE     := $(COVER_DIR)/native-test-results.jsonl

# Bench
BENCH_DIR := bench/pseudobench
TIKAL     := bench/pseudobench/tikal.sh

.PHONY: all help build install \
        test test-fast test-unit test-race test-verbose test-integration \
        test-framework test-cli test-kapi test-e2e-kapi \
        test-bridge-filters test-bridge-pool test-bridge-json test-native-json \
        fetch-bridge-jar fetch-bridge-testdata \
        cover fmt vet lint check proto deps deps-update clean \
        bench-build bench-generate bench-run bench-run-all bench-versions

# ── General ──────────────────────────────────────────────────────────────────

all: help

help: ## Show framework targets
	@echo ""
	@echo "Framework targets (make -C framework <target>):"
	@awk '/^[a-zA-Z_-]+:.*## / { \
		match($$0, /## (.*)/); desc = substr($$0, RSTART+3); \
		match($$0, /^[a-zA-Z_-]+/); target = substr($$0, RSTART, RLENGTH); \
		printf "  \033[36m%-20s\033[0m %s\n", target, desc \
	}' $(MAKEFILE_LIST)
	@echo ""

# ── Build ────────────────────────────────────────────────────────────────────

build: ## Build the kapi CLI
	@mkdir -p $(BIN_DIR)
	cd kapi && $(GOBUILD) $(LDFLAGS) -o $(BIN_DIR)/kapi ./cmd/kapi

install: ## Install kapi CLI to GOPATH/bin
	cd kapi && $(GO) install $(LDFLAGS) ./cmd/kapi

# ── Test ─────────────────────────────────────────────────────────────────────

test: ## Run all framework tests
	$(GOTEST) ./... -count=1
	cd cli && $(GOTEST) ./... -count=1
	cd kapi && $(GOTEST) ./... -count=1

test-fast: ## Run all framework tests with caching
	$(GOTEST) ./...
	cd cli && $(GOTEST) ./...
	cd kapi && $(GOTEST) ./...

test-unit: ## Run framework unit tests only
	$(GOTEST) ./... -count=1 -short
	cd cli && $(GOTEST) ./... -count=1 -short
	cd kapi && $(GOTEST) ./... -count=1 -short

test-race: ## Run framework tests with race detector
	$(GOTEST) ./... -count=1 -race
	cd cli && $(GOTEST) ./... -count=1 -race
	cd kapi && $(GOTEST) ./... -count=1 -race

test-verbose: ## Run framework tests with verbose output
	$(GOTEST) ./... -count=1 -v
	cd cli && $(GOTEST) ./... -count=1 -v
	cd kapi && $(GOTEST) ./... -count=1 -v

test-integration: ## Run framework integration tests
	$(GOTEST) ./... -count=1 -tags=integration -run Integration
	cd cli && $(GOTEST) ./... -count=1 -tags=integration -run Integration

test-framework: ## Run framework module tests only
	$(GOTEST) ./... -count=1

test-cli: ## Run cli module tests only
	cd cli && $(GOTEST) ./... -count=1

test-kapi: ## Run kapi CLI tests only
	cd kapi && $(GOTEST) ./... -count=1

test-e2e-kapi: ## Run kapi CLI end-to-end tests
	bash kapi/e2e/run.sh

# ── Bridge Tests ─────────────────────────────────────────────────────────────

fetch-bridge-jar: ## Download okapi-bridge JAR from GitHub release
	@GITHUB_TOKEN=$(GITHUB_TOKEN) bash $(ROOT_DIR)/scripts/fetch-okapi-bridge.sh

fetch-bridge-testdata: ## Download okapi test data from GitHub release
	@GITHUB_TOKEN=$(GITHUB_TOKEN) bash $(ROOT_DIR)/scripts/fetch-okapi-testdata.sh

test-bridge-filters: fetch-bridge-jar fetch-bridge-testdata ## Run bridge filter integration tests
	NEOKAPI_BRIDGE_JAR=$(BRIDGE_JAR) $(GOTEST) -tags=integration -count=1 -v ./core/plugin/bridge/filters/...

test-bridge-pool: fetch-bridge-jar fetch-bridge-testdata ## Run bridge tests with shared JVM pool
	@NEOKAPI_BRIDGE_JAR=$(BRIDGE_JAR) bash $(ROOT_DIR)/scripts/run-bridge-tests.sh $(or $(BRIDGE_POOL_SIZE),4)

test-bridge-json: fetch-bridge-jar fetch-bridge-testdata ## Run bridge filter tests with JSON output
	@mkdir -p $(COVER_DIR)
	NEOKAPI_BRIDGE_JAR=$(BRIDGE_JAR) $(GOTEST) -tags=integration -count=1 -json ./core/plugin/bridge/filters/... > $(GOTEST_JSON_FILE) ; true

test-native-json: ## Run native format tests with JSON output
	@mkdir -p $(COVER_DIR)
	$(GOTEST) -count=1 -json ./core/formats/... > $(NATIVE_JSON_FILE) ; true

# ── Coverage ─────────────────────────────────────────────────────────────────

cover: ## Run framework tests with coverage
	@mkdir -p $(COVER_DIR)
	$(GOTEST) ./... -count=1 -coverprofile=$(COVER_DIR)/framework.out -covermode=atomic
	cd cli && $(GOTEST) ./... -count=1 -coverprofile=$(COVER_DIR)/cli.out -covermode=atomic
	cd kapi && $(GOTEST) ./... -count=1 -coverprofile=$(COVER_DIR)/kapi.out -covermode=atomic

# ── Code Quality ─────────────────────────────────────────────────────────────

fmt: ## Format Go source files
	$(GOFMT) -w -s .

vet: ## Run go vet
	$(GOVET) ./...
	cd cli && $(GOVET) ./...
	cd kapi && $(GOVET) ./...

lint: ## Run golangci-lint
ifdef GOLANGCI_LINT
	$(GOLANGCI_LINT) run ./...
	cd cli && $(GOLANGCI_LINT) run ./...
	cd kapi && $(GOLANGCI_LINT) run ./...
else
	@echo "golangci-lint not installed. Run 'make tools' to install."
endif

check: fmt vet lint ## Run all code quality checks

# ── Protobuf ─────────────────────────────────────────────────────────────────

proto: ## Generate Go code from bridge protobuf definitions
ifndef PROTOC
	$(error "protoc not found. Install Protocol Buffers compiler.")
endif
ifndef PROTOC_GEN_GO
	$(error "protoc-gen-go not found. Run 'make tools' to install.")
endif
	protoc --go_out=. --go_opt=paths=source_relative \
		--go-grpc_out=. --go-grpc_opt=paths=source_relative \
		$(BRIDGE_PROTO_DIR)/*.proto

# ── Dependencies ─────────────────────────────────────────────────────────────

deps: ## Download and tidy framework dependencies
	$(GO) mod download && $(GO) mod tidy
	cd cli && $(GO) mod download && $(GO) mod tidy
	cd kapi && $(GO) mod download && $(GO) mod tidy

deps-update: ## Update framework dependencies
	$(GO) get -u ./... && $(GO) mod tidy
	cd cli && $(GO) get -u ./... && $(GO) mod tidy
	cd kapi && $(GO) get -u ./... && $(GO) mod tidy

# ── Bench ────────────────────────────────────────────────────────────────────

bench-build: ## Build PseudoBench tool
	cd $(BENCH_DIR) && GOWORK=off $(GOBUILD) -o pseudobench .

bench-generate: bench-build ## Generate PseudoBench test data
	cd $(BENCH_DIR) && GOWORK=off ./pseudobench generate

bench-run: bench-build build ## Run PseudoBench (kapi native + bridge)
	cd $(BENCH_DIR) && GOWORK=off ./pseudobench run -kapi $(BIN_DIR)/kapi

bench-run-all: bench-build build ## Run PseudoBench (all engines: native, bridge, bridge-daemon, tikal)
	cd $(BENCH_DIR) && GOWORK=off ./pseudobench run -kapi $(BIN_DIR)/kapi \
		-bridge-jar $(BRIDGE_JAR) \
		$(if $(wildcard $(TIKAL)),-okapi $(ROOT_DIR)/$(TIKAL))

bench-versions: bench-build ## Build kapi from multiple git tags
	cd $(BENCH_DIR) && GOWORK=off ./pseudobench build-versions -versions "$(VERSIONS)"

# ── Clean ────────────────────────────────────────────────────────────────────

clean: ## Clean framework build artifacts
	$(GO) clean -cache -testcache
