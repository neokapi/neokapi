# gokapi Makefile
# ================

# Variables
GO          := go
GOTEST      := $(GO) test
GOBUILD     := $(GO) build
GOVET       := $(GO) vet
GOFMT       := gofmt
MODULE      := github.com/asgeirf/gokapi
CLI_PKG     := $(MODULE)/cmd/kapi
SERVER_PKG  := $(MODULE)/cmd/gokapi-server
VERSION     ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT      := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_DATE  := $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
LDFLAGS     := -ldflags "-X $(MODULE)/cmd/kapi.version=$(VERSION) -X $(MODULE)/cmd/kapi.commit=$(COMMIT) -X $(MODULE)/cmd/kapi.buildDate=$(BUILD_DATE)"
BIN_DIR     := bin
COVER_DIR   := coverage
PROTO_DIR   := plugin/proto/v1
PROTO_FILES := $(wildcard $(PROTO_DIR)/*.proto)
FRONTEND_DIR := apps/bowrain/frontend
NPM         := npm

# Tools
GOLANGCI_LINT := $(shell which golangci-lint 2>/dev/null)
PROTOC        := $(shell which protoc 2>/dev/null)
PROTOC_GEN_GO := $(shell which protoc-gen-go 2>/dev/null)

.PHONY: all build build-server build-all build-frontend test test-unit test-integration \
        test-race lint fmt vet proto clean install cover tools help \
        frontend-deps frontend-dev frontend-build

# Default target
all: fmt vet lint test build ## Build and validate everything

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'

# ── Build ────────────────────────────────────────────────────────────────────

build: ## Build the kapi CLI
	@mkdir -p $(BIN_DIR)
	$(GOBUILD) $(LDFLAGS) -o $(BIN_DIR)/kapi $(CLI_PKG)

build-server: ## Build the gokapi REST server
	@mkdir -p $(BIN_DIR)
	$(GOBUILD) $(LDFLAGS) -o $(BIN_DIR)/gokapi-server $(SERVER_PKG)

build-all: build build-server ## Build all Go binaries

install: ## Install kapi CLI to GOPATH/bin
	$(GO) install $(LDFLAGS) $(CLI_PKG)

# ── Frontend (Bowrain UI) ───────────────────────────────────────────────────

frontend-deps: ## Install frontend dependencies
	cd $(FRONTEND_DIR) && $(NPM) install

frontend-dev: ## Start frontend dev server
	cd $(FRONTEND_DIR) && $(NPM) run dev

frontend-build: ## Build frontend for production
	cd $(FRONTEND_DIR) && $(NPM) run build

build-ui: build-server frontend-build ## Build server + frontend

# ── Test ─────────────────────────────────────────────────────────────────────

test: ## Run all tests
	$(GOTEST) ./... -count=1

test-unit: ## Run unit tests only (exclude integration)
	$(GOTEST) ./... -count=1 -short

test-race: ## Run tests with race detector
	$(GOTEST) ./... -count=1 -race

test-integration: ## Run integration tests
	$(GOTEST) ./... -count=1 -tags=integration -run Integration

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
	golangci-lint run ./...
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
	rm -rf $(FRONTEND_DIR)/dist
	rm -rf $(FRONTEND_DIR)/node_modules
	$(GO) clean -cache -testcache

# ── Dependencies ─────────────────────────────────────────────────────────────

deps: ## Download and tidy dependencies
	$(GO) mod download
	$(GO) mod tidy

deps-update: ## Update all dependencies
	$(GO) get -u ./...
	$(GO) mod tidy
