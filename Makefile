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

# ── kapi dogfood isolation ────────────────────────────────────────────────
# This repo dogfoods kapi: a *.kapi recipe sits at the repo root, which kapi
# auto-discovers via a git-style upward walk from any in-repo cwd. Every
# in-repo kapi invocation that is NOT the dogfood workflow itself must opt out
# of that discovery (KAPI_NO_PROJECT) and point kapi at a throwaway
# config/data/cache home, so it can never read the developer's ~/.config/kapi,
# their user-installed plugins, or silently bind to the dogfood recipe.
# KAPI_PLUGINS_DIR_ONLY also excludes the *system* plugin roots (Homebrew,
# /usr/share) — XDG_DATA_HOME only isolates the user root — so an in-repo kapi
# discovers no globally-installed plugins at all.
# Prefix in-repo kapi calls with $(KAPI_ISO_ENV). See CLAUDE.md "Dogfooding".
KAPI_ISO_DIR := $(CURDIR)/.kapi-iso
KAPI_ISO_ENV := KAPI_NO_PROJECT=1 KAPI_PLUGINS_DIR_ONLY=1 KAPI_CONFIG_DIR=$(KAPI_ISO_DIR)/config XDG_DATA_HOME=$(KAPI_ISO_DIR)/data XDG_CACHE_HOME=$(KAPI_ISO_DIR)/cache

GOLANGCI_LINT := $(shell which golangci-lint 2>/dev/null || { test -x "$$(go env GOPATH)/bin/golangci-lint" && echo "$$(go env GOPATH)/bin/golangci-lint"; })
PROTOC        := $(shell which protoc 2>/dev/null)
PROTOC_GEN_GO := $(shell which protoc-gen-go 2>/dev/null)

# ── CI auto-detection ────────────────────────────────────────────────────────
# GitHub Actions sets CI=true and GITHUB_EVENT_NAME=<event>. We run two CI
# profiles, keyed off the event, to keep PR feedback fast without weakening the
# main gate:
#
#   • pull_request  → FAST: no race detector, no -shuffle, no coverage. Dropping
#     -shuffle/-coverprofile lets Go's per-package test cache serve unchanged
#     packages (both flags otherwise force a re-run), and dropping -race removes
#     a 2–3× compile+run multiplier. JUnit -json output is kept (it caches).
#   • push / schedule → FULL: -race, -shuffle, coverage — the canonical record.
#     Every commit merged to main and the nightly run still get the full gate.
#
# Locally (no CI) we keep -shuffle but no race/coverage, as before.

ifdef CI
  _COVMODE := -covermode=atomic
  ifeq ($(GITHUB_EVENT_NAME),pull_request)
    _RACE    :=
    _SHUFFLE :=
    _CI_FAST := 1
  else
    _RACE    := -race
    _SHUFFLE := -shuffle=on
    _CI_FAST :=
  endif
else
  _RACE    :=
  _SHUFFLE := -shuffle=on
  _COVMODE :=
  _CI_FAST :=
endif

# Base test command: shuffles outside fast-PR mode, adds race on push/nightly.
GOTEST_BASE := $(GOTEST) $(_RACE) $(_SHUFFLE)

# cov,<outfile>: expands to coverage flags on the full gate, and to nothing in
# fast-PR mode (a -coverprofile flag would disable Go's test result cache).
ifdef _CI_FAST
  cov =
else
  cov = -coverprofile=$(1) $(_COVMODE)
endif

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

format-acceptance: ## Run native-format consumer-toolchain acceptance tests (plutil/resgen/xmllint/node; each check auto-skips if its tool is absent)
	# Scoped to the formats that ship a //go:build acceptance suite — running
	# ./core/formats/... would also pull in unrelated packages' fixture-dependent
	# tests (e.g. xliff2's okapi byte-equal corpus). Add new formats here as they
	# gain acceptance coverage.
	# Clear NODE_OPTIONS so node tooling spawned by the JSON-schema + MDX checks do
	# not inherit a flag the runner's node rejects (e.g. CI sets
	# --experimental-strip-types, which Node 20 refuses). These checks need none.
	NODE_OPTIONS= $(GO) test -tags acceptance -count=1 \
		./core/formats/xcstrings/ ./core/formats/arb/ ./core/formats/resx/ \
		./core/formats/androidxml/ ./core/formats/applestrings/ \
		./core/formats/i18next/ ./core/formats/designtokens/ ./core/formats/mdx/

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
	$(GOTEST_BASE) $(call cov,$(COVER_DIR)/framework.out) -json ./... > test-results-framework.json
else
	$(GOTEST_BASE) ./... -count=1
endif

test-cli: ## Run cli module tests only
	@mkdir -p $(COVER_DIR)
ifdef CI
	cd cli && $(GOTEST_BASE) $(call cov,../$(COVER_DIR)/cli.out) -json ./... > ../test-results-cli.json
else
	cd cli && $(GOTEST_BASE) ./... -count=1
endif

test-kapi: ## Run kapi CLI tests only
	@mkdir -p $(COVER_DIR)
ifdef CI
	cd kapi && $(GOTEST_BASE) $(call cov,../$(COVER_DIR)/kapi.out) -json ./... > ../test-results-kapi.json
else
	cd kapi && $(GOTEST_BASE) ./... -count=1
endif

test-platform test-bowrain-cli test-bowrain-plugin test-bowrain: ## Run individual bowrain module tests
	$(MAKE) -C bowrain $@

# Bowrain Desktop backend tests run on their own (the bowrain module's
# `test-bowrain` excludes apps/bowrain under CI) because the Wails app backend
# needs the GTK/WebKit toolchain on Linux — mirrors `kapi-desktop-test`. Driven
# by the `bowrain-desktop` CI job.
bowrain-desktop-test: ## Run Bowrain Desktop Go backend tests
	cd bowrain/apps/bowrain && $(GOTEST_BASE) ./backend/... -count=1 -timeout 120s

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

ci-test-bowrain-desktop: ## Run Bowrain Desktop tests with full CI flags locally
	$(MAKE) CI=true bowrain-desktop-test

ci-test-all: ## Run all module tests with full CI flags locally
	$(MAKE) CI=true test-framework test-cli test-kapi kapi-desktop-test bowrain-desktop-test
	$(MAKE) -C bowrain CI=true test-platform test-bowrain-cli test-bowrain-plugin test-bowrain

# ── CI job bodies (thin-CI) ─────────────────────────────────────────────────
#
# Each ci.yml job whose steps were bespoke inline shell now delegates to one of
# these targets (`run: make ci-<job>`), so the Makefile is the single source of
# truth a maintainer can run locally to reproduce a red CI job. The non-`run:`
# steps (checkout, setup actions, artifact upload, test-reporter) stay in the
# workflow; only the shell bodies live here. Commands mirror the YAML verbatim
# (e.g. bare `go build`, no fts5 tag) so local == CI.
#
# The lint jobs are not mirrored here (they post PR annotations via the pinned
# golangci-lint-action and gha-lint bootstraps actionlint), but `make lint` and
# CI lint are otherwise equivalent: the fts5/parity build tags live in
# .golangci.yml (run.build-tags), so both analyze the same cgo/FTS5/parity code
# — provided libicu-dev is on PKG_CONFIG_PATH, which the lint CI job installs.

ci-frontend: ## Mirror the CI `frontend` job: check/test/build the bowrain web frontends
	# bowrain/packages/ui and bowrain/apps/web consume @neokapi/{ui,flow-editor},
	# which import `@neokapi/kapi-react/runtime` (a built ./dist subpath export).
	# Build kapi-react first so that subpath resolves (mirrors ci-kapi-desktop-frontend).
	cd packages/kapi-react && vp run build
	cd bowrain/packages/ui && vp check
	cd bowrain/packages/ui && vp test
	cd bowrain/apps/web && vp check
	cd bowrain/apps/web && vp test
	cd bowrain/apps/web && vp build
	cd bowrain/apps/bowrain/frontend && vp check
	cd bowrain/apps/ctrl && vp check
	cd bowrain/apps/pulse && vp check
	cd bowrain/apps/keycloak-theme && vp check
	cd bowrain/emails && vp check

ci-kapi-desktop-frontend: ## Mirror the CI `kapi-desktop` job's frontend half (Go backend test is a separate step)
	cd packages/kapi-react && vp run build
	cd packages/ui && vp check
	cd packages/flow-editor && vp check
	cd apps/kapi-desktop/frontend && vp check
	cd packages/flow-editor && vp test
	cd apps/kapi-desktop/frontend && vp test
	cd storybook && vpx storybook build -o storybook-static

ci-bowrain-desktop-frontend: ## Mirror the CI `bowrain-desktop` job's frontend half (Go backend test is a separate step)
	# Build kapi-react first so its `/runtime` subpath export resolves for the
	# @neokapi/{ui,flow-editor} components the desktop frontend pulls in.
	cd packages/kapi-react && vp run build
	cd bowrain/apps/bowrain/frontend && vp test

ci-kapi-react: ## Mirror the CI `kapi-react` job: typecheck/validate/test/build kapi-format + kapi-react
	cd packages/kapi-format && vp run typecheck
	cd packages/kapi-format && vp run validate
	cd packages/kapi-format && vp run test
	cd packages/kapi-react && vp run typecheck
	cd packages/kapi-react && vp run test
	cd packages/kapi-react && vp run build

ci-build: ## Mirror the CI `build` job: build all three binaries (no fts5) + assert module isolation
	@mkdir -p bowrain/apps/web/dist && echo placeholder > bowrain/apps/web/dist/index.html
	@mkdir -p apps/kapi-desktop/frontend/dist && echo placeholder > apps/kapi-desktop/frontend/dist/index.html
	cd kapi && go build -o ../bin/kapi ./cmd/kapi
	cd bowrain/cli && go build -o ../../bin/kapi-bowrain ./cmd/kapi-bowrain
	cd bowrain && go build -o ../bin/bowrain-server ./cmd/bowrain-server
	GOWORK=off bash -c "go build ./..."
	GOWORK=off bash -c "cd cli && go build ./..."
	GOWORK=off bash -c "cd bowrain/core && go build ./..."
	GOWORK=off bash -c "cd kapi && go build ./..."
	GOWORK=off bash -c "cd bowrain/cli && go build ./..."
	@# kapi must not depend on platform / bowrain / heavy deps
	@if cd kapi && GOWORK=off go list -m all | grep -q 'neokapi/platform'; then echo "kapi should not depend on platform"; exit 1; fi
	@if cd bowrain && GOWORK=off go list -m all | grep -q 'neokapi/cli'; then echo "bowrain should not depend on cli"; exit 1; fi
	@if cd kapi && GOWORK=off go list -m all | grep -iE 'wails|labstack/echo|keycloak'; then exit 1; fi

ci-tidy: ## Mirror the CI `tidy-check` job: go mod tidy across all modules + fail on drift
	@for dir in . cli kapi apps/kapi-desktop bowrain/core bowrain/cli bowrain/plugin bowrain/plugin/schema bowrain; do \
	  echo "Checking $$dir..."; \
	  (cd "$$dir" && go mod tidy); \
	done
	@if ! git diff --exit-code -- '**go.mod' '**go.sum'; then \
	  echo "::error::go.mod/go.sum files are not tidy. Run 'make deps' locally."; \
	  exit 1; \
	fi

# ── Module Isolation ──────────────────────────────────────────────────────────

verify-isolation: ## Verify all Go module isolation boundaries
	GOWORK=off bash -c "go build ./..."
	GOWORK=off bash -c "cd cli && go build ./..."
	GOWORK=off bash -c "cd bowrain/core && go build ./..."
	GOWORK=off bash -c "cd kapi && go build ./..."
	GOWORK=off bash -c "cd bowrain/cli && go build ./..."
	@# kapi must not depend on bowrain
	@if cd kapi && GOWORK=off go list -m all 2>/dev/null | grep -q 'neokapi/bowrain'; then echo "ERROR: kapi depends on bowrain"; exit 1; fi
	@# bowrain/core must not depend on the main bowrain module (framework-only)
	@if cd bowrain/core && GOWORK=off go list -m all 2>/dev/null | grep -qE 'neokapi/bowrain($$| )'; then echo "ERROR: bowrain/core depends on the main bowrain module"; exit 1; fi
	@# bowrain must not depend on cli
	@if cd bowrain && GOWORK=off go list -m all 2>/dev/null | grep -iE 'neokapi/cli'; then echo "ERROR: bowrain depends on cli"; exit 1; fi
	@# kapi must not have heavy deps
	@if cd kapi && GOWORK=off go list -m all 2>/dev/null | grep -iE 'wails|echo|oidc|keyring'; then echo "ERROR: kapi has heavy deps"; exit 1; fi

# audit-modules asserts the module isolation contract and fails on drift. For
# each isolated module it runs a GOWORK=off build (so the module resolves
# against its own go.mod, not the workspace — a boundary violation that the
# go.work overlay would otherwise hide), then a GOWORK=off `go mod tidy` and
# fails if tidy was not a no-op (stale or missing requires, or a require that
# pulls in a forbidden boundary-crossing module, all leave a diff). The
# pre-tidy go.mod/go.sum are snapshotted and restored, so the check is robust
# whether the changes are committed (CI) or still in the working tree (local
# pre-push). Mirrors the per-module isolation commands in CLAUDE.md "Build
# Conventions".
#
# bowrain/core additionally gets an import-level assertion: its transitive
# package imports (GOWORK=off go list -deps ./...) must contain NO package from
# the main bowrain module — only bowrain/core/* and bowrain/plugin/schema are
# allowed under the bowrain/ tree. This fails fast on a re-introduced
# bowrain/sync or bowrain/proto/v1 import (which would otherwise re-add the
# require + replace on the main bowrain module and re-couple the framework-only
# core to redis/echo/the gRPC service surface).
#
# Modules audited (path → expected boundary):
#   .                  framework — no cli/bowrain deps
#   cli                framework only — no bowrain dep
#   bowrain/core       framework (+ plugin/schema) only — no cli AND no main bowrain dep
#   kapi               framework + cli only — no bowrain dep
#   apps/kapi-desktop  framework + cli (+ plugin/schema) only — no bowrain dep
#   bowrain/cli        framework + cli + bowrain/core (the kapi-bowrain plugin)
#   bowrain            framework + bowrain/core (the platform)
#
# bowrain and bowrain/cli are not isolation boundaries (they legitimately depend
# on several modules), but they are audited for the same go.mod/go.sum tidiness —
# e.g. a require that should be indirect after a package moves. CI's Tidy Check
# covers all modules, so they belong here too.
#
# Build pattern per module: most build ./..., but apps/kapi-desktop's main
# package embeds frontend/dist (//go:embed all:frontend/dist) which only exists
# after a frontend build, so — like `make kapi-desktop-test` — we build only
# ./backend/... for it. `go mod tidy` still resolves the whole module graph
# (embeds don't affect dependency resolution), so the boundary contract holds.
AUDIT_MODULES := . cli bowrain/core kapi apps/kapi-desktop bowrain/cli bowrain

audit-modules: ## Assert module isolation + go.mod/go.sum tidiness (fails on drift)
	@set -e; rc=0; for dir in $(AUDIT_MODULES); do \
	  echo ">> audit $$dir"; \
	  pkgs="./..."; [ "$$dir" = "apps/kapi-desktop" ] && pkgs="./backend/..."; \
	  ( cd "$$dir" && GOWORK=off $(GO) build $$pkgs ) || { echo "ERROR: $$dir failed isolated (GOWORK=off) build"; exit 1; }; \
	  if [ "$$dir" = "bowrain/core" ]; then \
	    bad=$$( cd "$$dir" && GOWORK=off $(GO) list -deps ./... 2>/dev/null \
	              | grep -E '^github\.com/neokapi/neokapi/bowrain(/|$$)' \
	              | grep -vE '^github\.com/neokapi/neokapi/bowrain/(core|plugin/schema)(/|$$)' || true ); \
	    if [ -n "$$bad" ]; then \
	      echo "ERROR: bowrain/core must be framework-only — it imports the main bowrain module:"; \
	      echo "$$bad" | sed 's/^/    /'; \
	      echo "  (move the shared code into a low package under bowrain/core, e.g. bowrain/core/sync or bowrain/core/proto/sync/v1)"; \
	      rc=1; \
	    fi; \
	  fi; \
	  cp "$$dir/go.mod" "$$dir/go.mod.audit.bak"; \
	  [ -f "$$dir/go.sum" ] && cp "$$dir/go.sum" "$$dir/go.sum.audit.bak" || true; \
	  ( cd "$$dir" && GOWORK=off $(GO) mod tidy ) || { echo "ERROR: $$dir failed go mod tidy"; exit 1; }; \
	  if ! diff -q "$$dir/go.mod.audit.bak" "$$dir/go.mod" >/dev/null 2>&1 || \
	     { [ -f "$$dir/go.sum.audit.bak" ] && ! diff -q "$$dir/go.sum.audit.bak" "$$dir/go.sum" >/dev/null 2>&1; }; then \
	    echo "ERROR: $$dir go.mod/go.sum not tidy — run 'cd $$dir && GOWORK=off go mod tidy' and commit"; rc=1; \
	  fi; \
	  mv "$$dir/go.mod.audit.bak" "$$dir/go.mod"; \
	  [ -f "$$dir/go.sum.audit.bak" ] && mv "$$dir/go.sum.audit.bak" "$$dir/go.sum" || true; \
	done; \
	[ $$rc -eq 0 ] || exit 1
	@echo "audit-modules: all module boundaries clean and go.mod/go.sum tidy"

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

PARITY_DASHBOARD := $(ROOT_DIR)/web/static/data/parity-report.json
PARITY_FIXTURES_JSON := $(ROOT_DIR)/web/static/data/parity-fixtures.json

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

# ── Regenerate the Okapi-harvested parity fixtures ──────────────────────────
#
# cli/parity/formats/fixtures_*_generated.go are extracted by
# scripts/okapi-test-scan from the upstream Okapi Java @Test classes. They are
# NOT regenerated by `make test` and have no //go:generate directive (the source
# path is machine-specific). Refresh them after bumping OKAPI_VERSION or when
# upstream adds @Test fixtures, then review the diff. Requires the Okapi clone
# (OKAPI_REPO). The class lists below are the source of truth for what each file
# extracts; keep them in committed order so regeneration stays byte-stable.
OKAPI_FIXTURE_SPECS := \
	dtd=DTDFilterTest \
	html=HtmlConfigurationSupportTest,HtmlEventTest,HtmlSnippetsTest,SkipEncodingDeclarationTest \
	json=JSONFilterTest,JsonSnippetParserTest \
	markdown=MarkdownFilterTest,MarkdownWriterTest \
	po=POFilterTest,POWriterTest \
	properties=PropertiesFilterTest \
	regex=RegexFilterTest \
	tmx=TmxFilterTest \
	ts=TsFilterTest \
	wiki=WikiFilterTest,WikiWriterTest \
	xliff=XLIFFFilterTest,XLIFFFilterXtmPropTest \
	yaml=YamlFilterTest,YamlParserTest,YmlFilterTest

regen-okapi-fixtures: ## Re-extract cli/parity/formats/fixtures_*_generated.go from the Okapi Java tests
	@[ -d "$(OKAPI_REPO)/okapi/filters" ] || { echo "OKAPI_REPO not found at $(OKAPI_REPO)"; exit 1; }
	@cd $(ROOT_DIR) && set -e; for spec in $(OKAPI_FIXTURE_SPECS); do \
	    fmt=$${spec%%=*}; classes=$${spec#*=}; \
	    echo "[regen] $$fmt ($$classes)"; \
	    go run ./scripts/okapi-test-scan \
	        -src $(OKAPI_REPO)/okapi/filters \
	        -class "$$classes" \
	        -package formats \
	        -out cli/parity/formats/fixtures_$${fmt}_generated.go; \
	done
	@echo "[regen] done — review 'git diff cli/parity/formats/fixtures_*_generated.go'"

regen-srx-parity-golden: ## Regenerate SRX parity golden from the real Okapi (okapi-apps tikal jars)
	@bash scripts/srx-parity/gen-golden.sh
	@echo "[regen] review 'git diff core/segment/srx/testdata/parity/golden.jsonl'"

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
CONTRACT_REPORT          := $(ROOT_DIR)/web/static/data/contract-audit.json
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

# Busybox multi-call links: kgrep / ksed / kcat dispatch to the kapi binary by
# argv[0] (see cli BusyboxRoot). The Homebrew formula creates these symlinks on
# install; mirror that locally so `make build` yields the short commands too.
LINK_KAPI_BUSYBOX = for n in kgrep ksed kcat; do ln -sf kapi $(BIN_DIR)/$$n; done

build: ## Build the kapi CLI (Apache-2.0; manifest-driven plugins discovered at runtime)
	@mkdir -p $(BIN_DIR)
	cd kapi && $(GOBUILD) $(LDFLAGS) -o $(BIN_DIR)/kapi ./cmd/kapi
	@$(LINK_KAPI_BUSYBOX)

# File-path alias so targets can declare a `bin/kapi` prerequisite (e.g. the
# l10n-* and *-pseudo-translate dogfood targets). `build` is phony, so this
# always reruns it — that's intended: callers want a CLI built from current
# source. Without this rule a clean checkout fails with
# "No rule to make target 'bin/kapi'".
bin/kapi: build

build-bowrain-plugin: ## Build the kapi-bowrain plugin binary (manifest-driven)
	@mkdir -p $(BIN_DIR)
	cd bowrain/cli && $(GOBUILD) $(LDFLAGS) -o $(BIN_DIR)/kapi-bowrain ./cmd/kapi-bowrain

PLUGIN_DIR := packages/kapi-claude-plugin
plugin-bundle: build ## Generate the Claude Code plugin skills/ from the embedded source (gitignored; built for release)
	@rm -rf $(PLUGIN_DIR)/skills
	@mkdir -p $(PLUGIN_DIR)/skills
	$(KAPI_ISO_ENV) $(BIN_DIR)/kapi skills export --dir $(PLUGIN_DIR)/skills >/dev/null
	@echo "Generated $(PLUGIN_DIR)/skills from the embedded skills (cli/skills/data)"

dev-skills: build ## Install kapi/bowrain skills into ./.claude/skills for in-repo dogfooding (gitignored)
	$(KAPI_ISO_ENV) $(BIN_DIR)/kapi skills install --target project >/dev/null
	@echo "Installed kapi/bowrain skills into .claude/skills (gitignored; canonical source is cli/skills/data)"

build-all: ## Build all Go binaries
	@mkdir -p $(BIN_DIR)
	cd kapi && $(GOBUILD) $(LDFLAGS) -o $(BIN_DIR)/kapi ./cmd/kapi
	@$(LINK_KAPI_BUSYBOX)
	cd bowrain/cli && $(GOBUILD) $(LDFLAGS) -o $(BIN_DIR)/kapi-bowrain ./cmd/kapi-bowrain
	$(MAKE) -C bowrain build-server build-worker build-kapi-bowrain-plugin

# Forward bowrain build targets
build-server build-worker build-kapi-bowrain-plugin build-bowrain build-headless install-kapi-bowrain-plugin:
	$(MAKE) -C bowrain $@

# ── SaT ML segmenter plugin (plugins/sat) ────────────────────────────────────
# Builds the kapi-sat plugin binary. The default target builds WITHOUT the ONNX
# backend (no cgo, no native deps) — useful for CI and for shipping a binary
# whose protocol/manifest are exercisable; segment requests then report that
# the binary lacks ONNX support.
#
# `build-sat-plugin-onnx` builds the real in-process segmenter and requires the
# native deps (CGO):
#   - onnxruntime shared library (download from microsoft/onnxruntime releases;
#     point the binary at it at RUNTIME via KAPI_SAT_ORT_LIB)
#   - the daulet/tokenizers static library (libtokenizers.a) on the linker path
#     (download from the daulet/tokenizers GitHub releases or `make build` it)
# Set SAT_TOKENIZERS_LIB to the directory containing libtokenizers.a.
# See plugins/sat/README.md for full install instructions.
build-sat-plugin: ## Build the kapi-sat plugin (no ONNX backend; pure Go)
	@mkdir -p $(BIN_DIR)
	cd plugins/sat && GOWORK=off $(GO) build $(LDFLAGS) -o $(BIN_DIR)/kapi-sat ./cmd/kapi-sat

build-sat-plugin-onnx: ## Build kapi-sat WITH the ONNX backend (requires onnxruntime + libtokenizers; CGO)
	@mkdir -p $(BIN_DIR)
	cd plugins/sat && GOWORK=off CGO_ENABLED=1 \
		CGO_LDFLAGS="-L$(SAT_TOKENIZERS_LIB)" \
		$(GO) build $(LDFLAGS) -tags onnx -o $(BIN_DIR)/kapi-sat ./cmd/kapi-sat

test-sat-plugin: ## Run kapi-sat pure-Go tests (protocol + algorithm + cache)
	cd plugins/sat && GOWORK=off $(GO) test ./...

# Package a signed-ready distribution tarball for the HOST platform: builds
# kapi-sat -tags onnx, bundles the onnxruntime shared lib at lib/<name> beside
# the binary (so an installed plugin needs no KAPI_SAT_ORT_LIB), and emits
# kapi-sat_<version>_<os>_<arch>.tar.gz under $(BIN_DIR)/sat-dist.
#
# Requires the same two native deps as build-sat-plugin-onnx:
#   SAT_TOKENIZERS_LIB  dir containing libtokenizers.a (linked at build time)
#   SAT_ORT_DIR         extracted onnxruntime release dir (its shared lib is
#                       copied into the tarball; downloaded from
#                       microsoft/onnxruntime releases)
# The release matrix (.github/workflows/release.yml: build-sat-plugin) runs the
# underlying script per platform; this target is the local equivalent.
package-sat-plugin: ## Package a kapi-sat distribution tarball for the host platform (CGO; needs SAT_TOKENIZERS_LIB + SAT_ORT_DIR)
	@test -n "$(SAT_TOKENIZERS_LIB)" || { echo "set SAT_TOKENIZERS_LIB to the dir containing libtokenizers.a"; exit 1; }
	@test -n "$(SAT_ORT_DIR)" || { echo "set SAT_ORT_DIR to the extracted onnxruntime release dir"; exit 1; }
	scripts/package-sat-plugin.sh \
		--version "$(VERSION)" \
		--ort-dir "$(SAT_ORT_DIR)" \
		--tokenizers-lib "$(SAT_TOKENIZERS_LIB)" \
		--out-dir "$(BIN_DIR)/sat-dist"

# ── kapi-check ML checker plugin ─────────────────────────────────────────────
# Mirrors the kapi-sat plugin: a cgo, -tags onnx sidecar that links
# daulet/tokenizers (static) at build time and loads the onnxruntime SHARED
# library at runtime (point the binary at it via KAPI_CHECK_ORT_LIB, or let the
# packaged tarball's lib/<name> satisfy it). The e5-small int8 model is NOT
# built in — it is acquired explicitly with `kapi-check pull` (downloads from
# HuggingFace into the XDG cache), matching common practice (vale sync / spacy
# download / ollama pull). Set CHECK_TOKENIZERS_LIB to the dir with
# libtokenizers.a. See plugins/check/README.md for full install instructions.
build-check-plugin: ## Build the kapi-check plugin (no ONNX backend; pure Go)
	@mkdir -p $(BIN_DIR)
	cd plugins/check && GOWORK=off $(GO) build $(LDFLAGS) -o $(BIN_DIR)/kapi-check ./cmd/kapi-check

build-check-plugin-onnx: ## Build kapi-check WITH the ONNX backend (requires onnxruntime + libtokenizers; CGO)
	@mkdir -p $(BIN_DIR)
	cd plugins/check && GOWORK=off CGO_ENABLED=1 \
		CGO_LDFLAGS="-L$(CHECK_TOKENIZERS_LIB)" \
		$(GO) build $(LDFLAGS) -tags onnx -o $(BIN_DIR)/kapi-check ./cmd/kapi-check

test-check-plugin: ## Run kapi-check pure-Go tests (protocol + vec + model cache)
	cd plugins/check && GOWORK=off $(GO) test ./...

# Package a signed-ready kapi-check distribution tarball for the HOST platform:
# builds kapi-check -tags onnx, bundles the onnxruntime shared lib at lib/<name>
# beside the binary (so an installed plugin needs no KAPI_CHECK_ORT_LIB), and
# emits kapi-check_<version>_<os>_<arch>.tar.gz under $(BIN_DIR)/check-dist.
# Needs the same two native deps as build-check-plugin-onnx:
#   CHECK_TOKENIZERS_LIB  dir containing libtokenizers.a (linked at build time)
#   CHECK_ORT_DIR         extracted onnxruntime release dir (its shared lib is
#                         bundled into the tarball; from microsoft/onnxruntime)
# The release matrix (.github/workflows/release-check.yml) runs the underlying
# script per platform; this target is the local equivalent.
package-check-plugin: ## Package a kapi-check distribution tarball for the host platform (CGO; needs CHECK_TOKENIZERS_LIB + CHECK_ORT_DIR)
	@test -n "$(CHECK_TOKENIZERS_LIB)" || { echo "set CHECK_TOKENIZERS_LIB to the dir containing libtokenizers.a"; exit 1; }
	@test -n "$(CHECK_ORT_DIR)" || { echo "set CHECK_ORT_DIR to the extracted onnxruntime release dir"; exit 1; }
	scripts/package-check-plugin.sh \
		--version "$(VERSION)" \
		--ort-dir "$(CHECK_ORT_DIR)" \
		--tokenizers-lib "$(CHECK_TOKENIZERS_LIB)" \
		--out-dir "$(BIN_DIR)/check-dist"

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
	$(KAPI_ISO_ENV) ./bin/kapi pseudo-translate $(KAPI_DESKTOP_DIR)/frontend/i18n \
		--target-lang qps \
		-o $(KAPI_DESKTOP_DIR)/frontend/i18n-qps \
		-q

kapi-desktop-compile: ## Compile i18n/ → public/translations/<locale>.json for the kapi-react runtime
	cd $(KAPI_DESKTOP_DIR)/frontend && vp run compile

kapi-desktop-translations: kapi-desktop-pseudo-translate kapi-desktop-compile ## Extract → pseudo-translate → compile

kapi-i18n-generate: ## Regenerate core/i18n/builtins/metadata.json from Go registries
	go generate ./core/i18n/...

kapi-i18n-pseudo-translate: kapi-i18n-generate bin/kapi ## Pseudo-translate builtins into core/i18n/catalogs/qps.mo
	$(KAPI_ISO_ENV) ./bin/kapi pseudo-translate core/i18n/builtins/metadata.json \
		--target-lang qps \
		-f json \
		-o core/i18n/catalogs/qps.mo \
		-q

kapi-i18n-translations: kapi-i18n-pseudo-translate ## Regenerate + pseudo-translate builtin metadata → MO

# ── Dogfood localization (root recipe: neokapi.kapi) ────────────────────────
# These targets ARE the dogfood workflow, so they deliberately run WITHOUT
# $(KAPI_ISO_ENV): they bind to the repo-root project and its .kapi/ state.
# Reviewed translations are committed as TMX under l10n/tm/; the project TM
# and termbase are rebuilt from those seeds (l10n-seed), then each surface is
# produced by TM leverage so output only ever contains reviewed strings.
L10N_LANGS := nb

# Seeds are committed in the native KLF-family forms (.klftm / .klftb):
# deterministic, lossless, and identity-preserving, so wipe-and-reseed
# reproduces the TM/termbase state exactly. TMX/CSV are the lossy
# interchange tier — emit them on demand with l10n-review-export.
l10n-seed: bin/kapi ## Rebuild .kapi/ termbase + TM from the committed l10n/ seeds
	@mkdir -p .kapi/cache
	@rm -f .kapi/termbase.db .kapi/tm.db
	./bin/kapi termbase import l10n/termbase.klftb
	@for f in l10n/tm/*.klftm; do \
		[ -e "$$f" ] || continue; \
		./bin/kapi tm import "$$f"; \
	done

l10n-review-export: l10n-seed ## Emit disposable TMX/CSV review views of the native seeds → l10n/review/
	@mkdir -p l10n/review
	./bin/kapi tm export --format tmx -o l10n/review/tm-all.tmx
	./bin/kapi termbase export --format csv -s en -t nb -o l10n/review/termbase-en-nb.csv
	@echo "Review views written to l10n/review/ (gitignored; the .klftm/.klftb seeds are the source of truth)"

l10n-builtins: l10n-seed kapi-i18n-generate ## Builtin tool/format metadata → core/i18n/catalogs/<lang>.mo (TM-driven)
	@for lang in $(L10N_LANGS); do \
		./bin/kapi tm-leverage core/i18n/builtins/metadata.json -f json \
			--target-lang $$lang -o core/i18n/catalogs/$$lang.mo || exit 1; \
	done

l10n-builtins-check: bin/kapi ## Terminology gate over the builtin metadata translations
	@for lang in $(L10N_LANGS); do \
		./bin/kapi tm-leverage core/i18n/builtins/metadata.json -f json \
			--target-lang $$lang -o /tmp/l10n-builtins-$$lang.json -q && \
		./bin/kapi term-check /tmp/l10n-builtins-$$lang.json -f json \
			--source-lang en --target-lang $$lang || exit 1; \
	done

l10n-desktop: l10n-seed kapi-desktop-extract ## Kapi Desktop UI strings → public/translations/<lang>.json (TM-driven)
	@for lang in $(L10N_LANGS); do \
		./bin/kapi tm-leverage $(KAPI_DESKTOP_DIR)/frontend/i18n \
			--target-lang $$lang \
			-o $(KAPI_DESKTOP_DIR)/frontend/i18n-$$lang || exit 1; \
		(cd $(KAPI_DESKTOP_DIR)/frontend && vp run compile:$$lang) || exit 1; \
	done

kapi-cli-i18n-generate: ## Regenerate cli/i18n/commands.json from the cobra command tree
	cd cli && go generate ./i18n/...

l10n-cli: l10n-seed kapi-cli-i18n-generate ## CLI help + output chrome → cli/i18n/catalogs/<lang>.mo (TM-driven)
	@for lang in $(L10N_LANGS); do \
		./bin/kapi tm-leverage cli/i18n/commands.json -f json \
			--target-lang $$lang -o cli/i18n/catalogs/$$lang.mo || exit 1; \
	done
	@echo "Note: rebuild the binary (make build) to embed the refreshed cli catalogs —"
	@echo "bin/kapi only shows the new translations after a rebuild."

# (The former l10n-landing target is gone with web/landing: the landing page
# was folded into the Docusaurus home, so its strings localize through the
# docs-site path. l10n/tm/landing-nb.klftm stays — the TM leverages that copy
# wherever it resurfaces.)
l10n: l10n-builtins l10n-desktop l10n-cli ## Rebuild all dogfood localization outputs from the l10n/ seeds

l10n-verify: l10n-builtins l10n-cli ## CI gate: Go-side l10n artifacts regenerate byte-identically from the seeds
	git diff --exit-code core/i18n/builtins core/i18n/catalogs cli/i18n/commands.json cli/i18n/catalogs

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

bench: bench-run ## Regenerate pseudobench data and publish to web/static/data
	cp $(PSEUDOBENCH_RESULTS)/pseudobench.json web/static/data/pseudobench.json
	@echo "Published $(PSEUDOBENCH_RESULTS)/pseudobench.json → web/static/data/pseudobench.json"

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
	cp $(PSEUDOBENCH_RESULTS)/pseudobench.json web/static/data/pseudobench.json
	@echo "Published $(PSEUDOBENCH_RESULTS)/pseudobench.json → web/static/data/pseudobench.json"

# ── Content-check quality eval ───────────────────────────────────────────────
# Scores the content checks (do-not-translate, placeholder, …) against a
# labeled corpus and regenerates the /check-eval dashboard data. The companion
# test (go test ./scripts/checkeval) gates the build on any regression — a new
# false positive or a missed finding — mirroring how `make parity` gates format
# faithfulness. The corpus grows from real corrections (issue #759).
check-eval: ## Run the content-check quality eval → web/src/pages/check-eval/_eval.json
	$(GO) run ./scripts/checkeval
	@echo "Published check-eval report → web/src/pages/check-eval/_eval.json"

# ── Frontend Checks ──────────────────────────────────────────────────────────

frontend-check-all: ## Run lint, format, and typecheck across all frontend projects
	$(MAKE) -C bowrain frontend-check-all

# Forward pulse targets
pulse-build pulse-dev pulse-check:
	$(MAKE) -C bowrain $@

# ── Documentation Assets ────────────────────────────────────────────────────
#
# Walkthrough engine (issue #425): scenes are recorded by docs-kapi.yml /
# docs-bowrain.yml workflows from web/scenes/ and bowrain/web/docs/scenes/.
# The legacy screenshots/recordings/cli-recordings/docs-assets/Remotion
# pipeline is removed — see commit history for what was here.

# Regenerate every neokapi-branded logo/icon/favicon from the two-background
# source pair (web/assets/neokapi-logo-2-{black,white}.png): combines them
# into one transparent, watermark-free master and fans it out. Fully scripted —
# no AI. Re-render the demo videos afterwards (make harness-videos) to pick up
# the new mascot. Bowrain is a separate brand: see scripts/generate-icons.sh.
logo: ## Regenerate all neokapi logo/icon/favicon assets from the source pair
	@bash scripts/generate-neokapi-logo.sh

fetch-docs-assets: ## Download legacy docs assets (transitional, until walkthrough engine fully covers)
	@gh release download docs-assets --pattern 'docs-assets.tar.gz' --dir /tmp --clobber
	@mkdir -p web/static
	@tar xzf /tmp/docs-assets.tar.gz -C web/static
	@rm -f /tmp/docs-assets.tar.gz
	@du -sh web/static/img web/static/video 2>/dev/null || true

publish-docs-assets: ## Publish web/static/{img,video} to the docs-assets release (merges, never drops)
	@bash scripts/publish-docs-assets.sh

# Bowrain's framed walkthrough videos (harness-rendered bowrain-web/ +
# bowrain-desktop/) are recorded on the desktop against a real running stack and
# live under bowrain/web/docs/static/video/ (gitignored). They ship via their own
# bowrain-docs-assets release — symmetric with kapi's docs-assets — staged by
# docs-bowrain.yml.
fetch-bowrain-docs-assets: ## Download bowrain docs assets from the bowrain-docs-assets release
	@gh release download bowrain-docs-assets --pattern 'bowrain-docs-assets.tar.gz' --dir /tmp --clobber
	@mkdir -p bowrain/web/docs/static
	@tar xzf /tmp/bowrain-docs-assets.tar.gz -C bowrain/web/docs/static
	@rm -f /tmp/bowrain-docs-assets.tar.gz
	@du -sh bowrain/web/docs/static/img bowrain/web/docs/static/video 2>/dev/null || true

publish-bowrain-docs-assets: ## Publish bowrain/web/docs/static/{img,video} to the bowrain-docs-assets release (merges, never drops)
	@bash scripts/publish-bowrain-docs-assets.sh

# harness/ records kapi driven by Claude Code as narrated 1-min explainer videos
# and publishes them theme-matched (light + dark) into the docs site. Built and
# published from your desktop — no CI required. See harness/Makefile for details.
harness-deps: ## Install the demo-video harness deps (node + Playwright)
	$(MAKE) -C harness deps

harness-videos: ## Render + convert the docs demo videos (light + dark) → web/static/video/kapi/
	$(MAKE) -C harness videos

# Phased video pipeline — seed once, record every screencast (the only phase
# needing the bowrain stack), then narrate, then package. Bring the stack up
# once and re-render freely without re-recording. `harness-videos-staged` runs
# the whole thing with the stack up only for seed+record. FORCE=1 redoes a phase.
harness-seed: ## Phase 0: seed bowrain (BowMart workspace + content) + mint record tokens → harness/.env
	$(MAKE) -C harness seed

harness-record: ## Phase 1: record all screencasts + artifacts (needs the bowrain stack up + harness-seed)
	$(MAKE) -C harness record FORCE=$(FORCE)

harness-narrate: ## Phase 2: synthesize all narration (no Docker; TTS only)
	$(MAKE) -C harness narrate FORCE=$(FORCE)

harness-package: ## Phase 3: render + publish all assets from persisted captures (no Docker/network)
	$(MAKE) -C harness package FORCE=$(FORCE)

harness-videos-all: harness-seed harness-record harness-narrate harness-package ## All phases in sequence (reuse existing outputs)

harness-videos-staged: ## Full fresh pass: stack up → seed → record, then narrate + package offline
	-$(MAKE) -C bowrain stack-up-web
	$(MAKE) harness-seed                || { $(MAKE) -C bowrain stack-down; exit 1; }
	$(MAKE) harness-record FORCE=1      || { $(MAKE) -C bowrain stack-down; exit 1; }
	$(MAKE) -C bowrain stack-down
	$(MAKE) harness-narrate FORCE=1
	$(MAKE) harness-package FORCE=1

# ── Generate (scripts at root) ──────────────────────────────────────────────

# okapi-bridge plugin dir feeding the reference dataset. Override with
# BRIDGE_PLUGIN=/path. Falls back to built-in-only when the dir is absent
# (the generator warns rather than fails).
BRIDGE_PLUGIN ?= $(ROOT_DIR)/../okapi-bridge/dist/plugin

generate-reference-docs: ## Generate the unified format + tool reference dataset (built-in + okapi-bridge) → packages/reference-data/data
	$(GO) run ./scripts/gen-refs $(if $(wildcard $(BRIDGE_PLUGIN)),-bridge $(BRIDGE_PLUGIN),)

check-reference-docs: ## Drift gate: fail if the committed reference dataset is stale vs. source (gates the built-in subset; absent okapi-bridge is fine)
	$(GO) run ./scripts/gen-refs -check $(if $(wildcard $(BRIDGE_PLUGIN)),-bridge $(BRIDGE_PLUGIN),)

# Superseded by generate-reference-docs; kept as an alias for existing callers.
generate-format-docs: generate-reference-docs

generate-contract-types: ## Generate the shared TS IO-contract types (packages/contract-types) from Go (core/schema)
	$(GO) run ./scripts/gen-contract-types

check-contract-types: ## Drift gate: fail if the committed contract types are stale vs. core/schema
	$(GO) run ./scripts/gen-contract-types -check

generate-reference-pages: ## Generate static per-entry reference MDX pages (R4, #673) → web/docs/reference/{commands,formats,tools}
	cd web && node --no-warnings --experimental-strip-types scripts/gen-reference-pages.ts

# ── Documentation Site ──────────────────────────────────────────────────────

docs-deps: ; cd web && vp install --frozen-lockfile
docs-dev: docs-wasm ; cd web && vp run start
docs-build: ; cd web && vp run build
docs-serve: ; cd web && vp run serve

# Output dir for the in-browser playground (gitignored; built locally or in CI).
WASM_DEMO_DIR := web/static/wasm

web-wasm-demo: ## Build the in-browser playground wasm + JS glue → web/static/wasm/
	@mkdir -p $(WASM_DEMO_DIR)
	GOOS=js GOARCH=wasm $(GO) build -o $(WASM_DEMO_DIR)/kapi.wasm ./cmd/kapi-wasm
	@cp "$$($(GO) env GOROOT)/lib/wasm/wasm_exec.js" $(WASM_DEMO_DIR)/wasm_exec.js
	@ls -lh $(WASM_DEMO_DIR)/kapi.wasm | awk '{print "  built",$$NF,$$5}'

web-wasm-cli: ## Build the in-browser kapi CLI (wasm) → web/static/wasm/kapi-cli.wasm
	@mkdir -p $(WASM_DEMO_DIR)
	cd kapi && GOOS=js GOARCH=wasm $(GO) build -o $(CURDIR)/$(WASM_DEMO_DIR)/kapi-cli.wasm ./cmd/kapi-wasm-cli
	@cp "$$($(GO) env GOROOT)/lib/wasm/wasm_exec.js" $(WASM_DEMO_DIR)/wasm_exec.js
	@# Precompress for the browser: the kit prefers kapi-cli.wasm.gz and inflates
	@# it via DecompressionStream('gzip'), so this works without the host having
	@# to set Content-Encoding (GitHub Pages / Docusaurus static serving do not).
	@gzip -9 -f -k -c $(WASM_DEMO_DIR)/kapi-cli.wasm > $(WASM_DEMO_DIR)/kapi-cli.wasm.gz
	@ls -lh $(WASM_DEMO_DIR)/kapi-cli.wasm | awk '{print "  built",$$NF,$$5}'
	@ls -lh $(WASM_DEMO_DIR)/kapi-cli.wasm.gz | awk '{print "  built",$$NF,$$5}'

# Stage the in-browser wasm for `docs-dev` when it's missing (a fresh checkout
# has none — the binaries are gitignored) OR stale (any engine Go source is newer
# than the built wasm). The freshness check means changing the engine and
# re-running `docs-dev` rebuilds automatically, instead of silently serving an
# old binary (which surfaced as missing exports / unsegmented output). Force a
# rebuild anytime with `make web-wasm-demo web-wasm-cli`.
WASM_SRC_DIRS := core cli kapi providers sievepen termbase cmd
docs-wasm:
	@if [ -f $(WASM_DEMO_DIR)/kapi.wasm ] && [ -f $(WASM_DEMO_DIR)/kapi-cli.wasm.gz ] && \
	   [ -z "$$(find $(WASM_SRC_DIRS) -name '*.go' -newer $(WASM_DEMO_DIR)/kapi-cli.wasm.gz 2>/dev/null | head -1)" ]; then \
		echo "  wasm up to date in $(WASM_DEMO_DIR)"; \
	else \
		echo "  staging in-browser wasm (missing or engine sources changed)…"; \
		$(MAKE) web-wasm-demo web-wasm-cli; \
	fi

docs-verify-snippets: web-wasm-cli ## Verify every RunnableSnippet + scene smoke_contract runs green in wasm
	node --experimental-strip-types scripts/verify-snippets/harness.ts

klf-smoke: web-wasm-cli ## Verify KLF Go(wasm)↔TS parity for the docs Tests page (serialization, preview, anchors, validation)
	node --experimental-strip-types scripts/verify-snippets/klf-smoke.ts

klz-smoke: build ## Verify the resumable .klz workspace lifecycle (open→step→finish == one-shot; pack stable)
	bash scripts/klz-smoke.sh $(BIN_DIR)/kapi

klz-wasm-smoke: web-wasm-cli ## Verify .klz workspace + .kapi project run in the browser WASM engine (JSON + Office)
	GOROOT="$$($(GO) env GOROOT)" node --experimental-strip-types scripts/klz-wasm-smoke.ts

# ── Pages publishing (local) ──────────────────────────────────────────────────
#
# Local equivalents of the docs-kapi.yml / docs-bowrain.yml / web-landing.yml +
# pages-deploy.yml chain: build each site with the PRODUCTION base URL pinned,
# then deploy via scripts/publish-pages.sh (clone neokapi.github.io, slot the
# builds, push with rebase-retry). The production base MUST be pinned or the
# live site 404s every asset (Vite/Docusaurus bake the base into the bundle).
# These are a manual escape hatch; the normal path is push-to-main → workflows.
# Pass PAGES_PUBLISH_YES=1 to skip the confirm prompt, DRY_RUN=1 to build-only.

BOWRAIN_LANDING_BASE := /web/bowrain/
NEOKAPI_DOCS_BASE    := /web/neokapi/
BOWRAIN_DOCS_BASE    := /web/bowrain/docs/

landing-build: ## Build the bowrain landing page with its production base URL → bowrain/web/landing/dist
	cd bowrain/web/landing && VITE_BASE=$(BOWRAIN_LANDING_BASE) vp run build

docs-build-prod: web-wasm-demo web-wasm-cli ## Build the kapi docs+landing site with the production base (run fetch-docs-assets first to stage videos) → web/build
	cd web && DOCS_BASE_URL=$(NEOKAPI_DOCS_BASE) vp run build

bowrain-docs-build-prod: ## Build the standalone bowrain docs site with the production base → bowrain/web/docs/build
	cd bowrain/web/docs && corepack pnpm install --ignore-workspace
	cd bowrain/web/docs && DOCS_BASE_URL=$(BOWRAIN_DOCS_BASE) vpx docusaurus build

publish-landing: landing-build ## Build + deploy the bowrain landing page to neokapi.github.io (PAGES_PUBLISH_YES=1 to skip prompt)
	@bash scripts/publish-pages.sh bowrain-landing

publish-website: fetch-docs-assets docs-build-prod fetch-bowrain-docs-assets bowrain-docs-build-prod ## Build + deploy the kapi & bowrain docs sites to neokapi.github.io
	@bash scripts/publish-pages.sh neokapi-docs bowrain-docs

# ── Tools ────────────────────────────────────────────────────────────────────

tools: ## Install development tools
	$(GO) install google.golang.org/protobuf/cmd/protoc-gen-go@latest
	$(GO) install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
	$(GO) install github.com/golangci/golangci-lint/cmd/golangci-lint@latest

setup-remote: ## Install dependencies for cloud environments
	CLAUDE_CODE_REMOTE=true bash scripts/setup-remote.sh

pre-push: ## Run checks relevant to your changes (mirrors CI)
	@./scripts/pre-push-check.sh

pre-push-all: audit-modules ## Run all checks regardless of changes
	@./scripts/pre-push-check.sh --all

gha-lint: ## Lint GitHub Actions workflow files
	@command -v actionlint >/dev/null 2>&1 || { echo "actionlint not installed."; exit 1; }
	actionlint

# ── Release ───────────────────────────────────────────────────────────────────
# Releases are tag-driven. `make release v=1.3.4` tags + pushes v1.3.4;
# release.yml then builds & publishes the CLI, desktop apps, Docker images,
# Homebrew casks and the plugin registry (macOS/Linux signed+notarized in CI).
# The Windows binaries come out as CI artifacts and are signed locally
# afterwards (SimplySign is a local Mac step) with `make release-windows`.

# Version is passed as v=1.3.4 (a leading "v" is tolerated, e.g. v=v1.3.4).
# The git tag is always vX.Y.Z; $(VER) is the bare X.Y.Z.
v   ?=
VER := $(patsubst v%,%,$(strip $(v)))
TAG := v$(VER)

.PHONY: release release-windows release-winget

release: ## Tag + push a release (v=1.3.4); CI builds & publishes the rest
	@[ -n "$(strip $(v))" ] || { echo "usage: make release v=1.3.4"; exit 1; }
	@echo "$(VER)" | grep -Eq '^[0-9]+\.[0-9]+\.[0-9]+' || { echo "✗ version must look like 1.3.4 (got '$(v)')"; exit 1; }
	@test -z "$$(git status --porcelain)" || { echo "✗ working tree not clean"; exit 1; }
	@test "$$(git rev-parse --abbrev-ref HEAD)" = "main" || { echo "✗ not on main"; exit 1; }
	@git fetch --quiet origin main
	@test "$$(git rev-parse HEAD)" = "$$(git rev-parse origin/main)" || { echo "✗ local main is not in sync with origin/main"; exit 1; }
	@git rev-parse "$(TAG)" >/dev/null 2>&1 && { echo "✗ tag $(TAG) already exists"; exit 1; } || true
	@printf "Tag and push %s at %s? [y/N] " "$(TAG)" "$$(git rev-parse --short HEAD)"; read ok; [ "$$ok" = "y" ] || { echo aborted; exit 1; }
	git tag -a "$(TAG)" -m "Release $(TAG)"
	git push origin "$(TAG)"
	@echo ""
	@echo "Pushed $(TAG). Follow CI with:  gh run watch"
	@echo "After CI finishes, sign Windows (with SimplySign Desktop logged in):"
	@echo "    make release-windows v=$(VER)"

release-windows: ## Sign the Windows artifacts + finalize the release (after CI; SimplySign logged in)
	@[ -n "$(strip $(v))" ] || { echo "usage: make release-windows v=1.3.4"; exit 1; }
	JSIGN_KEYSTORE="$${JSIGN_KEYSTORE:-$$HOME/simplysign-pkcs11.cfg}" \
		./scripts/publish-windows-signed.sh "$(TAG)"

release-winget: ## Submit the signed CLI to winget-pkgs (after release-windows; needs WINGET_TOKEN + `komac new` bootstrap)
	@[ -n "$(strip $(v))" ] || { echo "usage: make release-winget v=1.3.4"; exit 1; }
	gh workflow run winget.yml --repo neokapi/neokapi -f tag="$(TAG)"

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
        parity-sandbox parity-test parity-publish parity-clean parity-fixtures regen-okapi-fixtures check-eval \
        contract-audit contract-audit-all contract-audit-clean okapi-failsafe-reports \
        fmt vet lint check check-framework check-bowrain test-parallel \
        test-framework test-cli test-kapi test-platform test-bowrain-cli test-bowrain-plugin test-bowrain \
        bowrain-desktop-test \
        ci-test-framework ci-test-cli ci-test-kapi ci-test-platform \
        ci-test-bowrain-cli ci-test-bowrain ci-test-kapi-desktop ci-test-bowrain-desktop ci-test-all \
        ci-frontend ci-kapi-desktop-frontend ci-bowrain-desktop-frontend ci-kapi-react ci-build ci-tidy \
        verify-isolation audit-modules \
        build build-all build-server build-worker build-kapi-bowrain-plugin build-bowrain-plugin build-bowrain build-headless \
        plugin-bundle dev-skills \
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
        bench bench-build bench-run bench-run-full bench-stress \
        logo fetch-docs-assets publish-docs-assets harness-deps harness-videos \
        harness-seed harness-record harness-narrate harness-package harness-videos-all harness-videos-staged \
        fetch-bowrain-docs-assets publish-bowrain-docs-assets \
        generate-format-docs generate-reference-docs check-reference-docs generate-reference-pages \
        generate-contract-types check-contract-types \
        docs-deps docs-dev docs-wasm docs-build docs-serve docs-verify-snippets \
        landing-build docs-build-prod bowrain-docs-build-prod publish-landing publish-website \
        tools setup-remote gha-lint clean \
        _fw-fmt _fw-test _fw-test-fast _fw-test-unit _fw-test-race _fw-test-verbose _fw-test-integration \
        _fw-vet _fw-lint _fw-proto _fw-deps _fw-deps-update
