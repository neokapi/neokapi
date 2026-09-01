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
# --match 'v[0-9]*': release tags share the tag namespace with per-package tags
# (contract-types-v0.1.0, sat-v1.0.2, vision-models-v1). Unfiltered, git describe
# reports whichever of those is nearest to HEAD as the CLI's own version.
export VERSION     ?= $(shell git describe --tags --match 'v[0-9]*' --always --dirty 2>/dev/null || echo "dev")
export COMMIT      := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
export BUILD_DATE  := $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
export VERSION_PKG := github.com/neokapi/neokapi/core/version
# Opt-out CLI telemetry (epic 018, workstream G): release builds bake the
# PostHog project key (and optionally a first-party endpoint) into
# host/telemetry via ldflags. Both are OPTIONAL — unset, dev and self-built
# binaries carry no key and emit nothing. Set KAPI_POSTHOG_KEY (and, for a
# proxy host, KAPI_POSTHOG_ENDPOINT) in the release environment to enable.
export TELEMETRY_PKG := github.com/neokapi/neokapi/host/telemetry
LDFLAGS_X := -X $(VERSION_PKG).Version=$(VERSION) -X $(VERSION_PKG).Commit=$(COMMIT) -X $(VERSION_PKG).BuildDate=$(BUILD_DATE)
ifneq ($(strip $(KAPI_POSTHOG_KEY)),)
LDFLAGS_X += -X $(TELEMETRY_PKG).apiKey=$(KAPI_POSTHOG_KEY)
ifneq ($(strip $(KAPI_POSTHOG_ENDPOINT)),)
LDFLAGS_X += -X $(TELEMETRY_PKG).endpoint=$(KAPI_POSTHOG_ENDPOINT)
endif
endif
export LDFLAGS     := -ldflags "$(LDFLAGS_X)"

GO := go
# FTS5 build tag is required by mattn/go-sqlite3 to enable FTS5 full-text
# search. Without it, content memory and terms migrations fail at runtime.
#
# ICU requirement: The FTS5 ICU tokenizer requires ICU development libraries.
#   Linux:  sudo apt-get install libicu-dev pkg-config
#   macOS:  brew bundle   (see Brewfile; this Makefile finds the keg itself)
# `make doctor` reports what is missing.
GOTAGS  := -tags "fts5"

# macOS Homebrew ICU: expose the keg to pkg-config if it is not already there.
#
# Resolved, never hardcoded. icu4c is keg-only and version-pinned (icu4c@76,
# @78, …), so its path moves on every ICU major bump — a pinned icu4c@NN keeps
# working on the machine that wrote it and breaks every cgo build on the next
# one, with a bare "[build failed]" and no mention of ICU. Prefer Homebrew's
# unversioned alias, which tracks the current keg; fall back to the newest
# versioned keg installed. Both Apple Silicon and Intel prefixes are searched.
ifeq ($(shell uname -s),Darwin)
ICU_PKGCONFIG := $(firstword $(wildcard /opt/homebrew/opt/icu4c/lib/pkgconfig /usr/local/opt/icu4c/lib/pkgconfig))
ifeq ($(ICU_PKGCONFIG),)
ICU_PKGCONFIG := $(shell ls -d /opt/homebrew/opt/icu4c@*/lib/pkgconfig /usr/local/opt/icu4c@*/lib/pkgconfig 2>/dev/null | sort -V | tail -1)
endif
ifneq ($(ICU_PKGCONFIG),)
export PKG_CONFIG_PATH := $(ICU_PKGCONFIG):$(PKG_CONFIG_PATH)
endif
endif
GOTEST  := $(GO) test $(GOTAGS)
GOBUILD := $(GO) build $(GOTAGS)
GOVET   := $(GO) vet $(GOTAGS)
GOFMT   := gofmt
BIN_DIR := $(ROOT_DIR)/bin
COVER_DIR := coverage

# ── Workspace layout (no absolute home paths, ever) ──────────────────────────
# Some targets reach outside this repo: sibling repos in the multi-repo
# workspace (okapi-bridge, registry, …) and unrelated reference checkouts (the
# upstream Okapi Framework Java tree, the DocLang spec). Those locations are
# per-developer, so they are named by environment variable with a
# repo-relative default — never by an absolute home path. A fresh clone in the
# conventional layout needs no environment at all; anything else sets the one
# variable it needs.
#
# Conventional layout the defaults assume:
#
#   <checkouts>/                      $(NEOKAPI_CHECKOUTS_DIR)
#   ├── neokapi/                      $(NEOKAPI_WORKSPACE_DIR)
#   │   ├── neokapi/                  this repo ($(ROOT_DIR))
#   │   ├── okapi-bridge/
#   │   └── registry/ …
#   ├── okapi/Okapi/                  $(NEOKAPI_OKAPI_DIR)
#   └── doclang-project/doclang/      $(NEOKAPI_DOCLANG_DIR)
#
# `make check-abs-paths` (part of `make lint`) keeps absolute home paths from
# creeping back in. See docs/internals/workspace-paths.md.
#
# The workspace is the parent of the MAIN checkout, not of $(CURDIR): in a
# linked git worktree (.claude/worktrees/<name>/) $(CURDIR)/.. is
# .claude/worktrees. git's common dir points at the main checkout's .git in
# both cases, so derive from that and fall back to $(CURDIR)/.. outside git
# (e.g. a source tarball).
_NEOKAPI_GIT_COMMON := $(shell git rev-parse --path-format=absolute --git-common-dir 2>/dev/null)
ifeq ($(strip $(_NEOKAPI_GIT_COMMON)),)
NEOKAPI_WORKSPACE_DIR ?= $(abspath $(CURDIR)/..)
else
NEOKAPI_WORKSPACE_DIR ?= $(abspath $(dir $(_NEOKAPI_GIT_COMMON))..)
endif
NEOKAPI_CHECKOUTS_DIR ?= $(abspath $(NEOKAPI_WORKSPACE_DIR)/..)
NEOKAPI_OKAPI_DIR     ?= $(NEOKAPI_CHECKOUTS_DIR)/okapi/Okapi
NEOKAPI_DOCLANG_DIR   ?= $(NEOKAPI_CHECKOUTS_DIR)/doclang-project/doclang
export NEOKAPI_WORKSPACE_DIR NEOKAPI_CHECKOUTS_DIR NEOKAPI_OKAPI_DIR NEOKAPI_DOCLANG_DIR

# Historical override name for the upstream Okapi clone, kept because CI and
# muscle memory pass it. Defined here rather than beside the contract-audit
# block so TIKAL_JAR_GLOB (an immediate `:=`, defined earlier in the file) sees
# a resolved value instead of the empty string.
OKAPI_REPO ?= $(NEOKAPI_OKAPI_DIR)
export OKAPI_REPO

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
KAPI_ISO_ENV := KAPI_NO_PROJECT=1 KAPI_TELEMETRY=0 KAPI_PLUGINS_DIR_ONLY=1 KAPI_CONFIG_DIR=$(KAPI_ISO_DIR)/config XDG_DATA_HOME=$(KAPI_ISO_DIR)/data XDG_CACHE_HOME=$(KAPI_ISO_DIR)/cache KAPI_PLUGINS_DIR=$(KAPI_ISO_DIR)/plugins

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

# PR CI rider: the fast PR profile drops -race (see the CI auto-detection block
# above), and main is unprotected — so a race introduced in a PR could merge
# green. This target races only the packages the PR actually changed (capped;
# see the script header for the fallback rules), keeping the signal cheap while
# push/nightly still run the full race gate. BASE_SHA/HEAD_SHA default to
# origin/main...HEAD for local use.
test-race-changed: i18n-catalogs ## Race-test only the Go packages changed vs BASE_SHA (PR CI)
	@bash scripts/test-race-changed.sh

test-verbose: ## Run tests with verbose output
	@$(MAKE) --no-print-directory _fw-test-verbose
	@$(MAKE) -C bowrain test-verbose

test-integration: ## Run the //go:build integration lane (Postgres-backed cross-store suites; needs Docker or BOWRAIN_TEST_POSTGRES_URL)
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

# NOTE: this is a FIXER, not a check — it rewrites files and exits 0. The
# corresponding check is `make check-gofmt`, which is what CI gates on.
fmt: ## Format Go source files
	$(GOFMT) -w -s .

vet: ## Run go vet (all modules)
	@$(MAKE) --no-print-directory _fw-vet
	@$(MAKE) -C bowrain vet

lint: check-abs-paths check-eval-publishable check-local-actions check-deploy-paths check-vocabulary check-desktop-interchange check-vocab-packs check-comment-history check-reference-provenance check-run-projection check-locale-display check-sidebar-ids check-package-licenses check-archive-licenses check-plugin-licenses check-plugin-release-latest check-tracked-binaries check-extract-fixtures check-gofmt ## Run golangci-lint (all modules) + repo hygiene guards
	@$(MAKE) --no-print-directory _fw-lint
	@$(MAKE) -C bowrain lint

check-abs-paths: ## Guard: no absolute home path (/Users/…, /home/…, C:\Users\…) in tracked files
	@./scripts/check-abs-paths.sh

# The lab's transcripts are COMMITTED, so they never pass through the publish
# script that checks the skill eval's. Same shape of risk and a shorter path to a
# public repository: an agent with bypassPermissions, a shell and $HOME recorded
# whole. Checked here, where every commit sees it.
check-eval-publishable: ## Guard: no credential shape in a committed eval transcript
	@./scripts/check-eval-publishable.sh web/static/authoring-lab

check-local-actions: ## Guard: a workflow's local `uses: ./…` resolves where that job checked the repo out
	@./scripts/check-local-actions.sh

check-deploy-paths: ## Guard: the deploy workflow triggers on every framework dir compiled into the deployed binaries
	@./scripts/check-deploy-paths.sh

check-run-projection: ## Guard: a Run sequence is projected through a declared RunSpec, never a hand-rolled walk
	@./scripts/check-run-projection.sh

.PHONY: check-locale-display
check-locale-display: ## Guard: a language is shown by name, not by its code alone
	@./scripts/check-locale-display.sh

check-sidebar-ids: ## Guard: every doc id a Docusaurus sidebar names resolves to a page
	@./scripts/check-sidebar-ids.sh

check-vocabulary: ## Guard: no retired framing or retired vocabulary in prose, product strings and build metadata
	@./scripts/check-vocabulary.sh

check-desktop-interchange: ## Guard: the Kapi Desktop surface names no interchange format (TMX/XLIFF/termbase)
	@./scripts/check-desktop-interchange.sh

check-comment-history: ## Guard: comments state what the code IS, not what it was changed from
	@./scripts/check-comment-history.sh

check-reference-provenance: ## Guard: the committed reference dataset comes only from this repo (no okapi-bridge)
	@./scripts/check-reference-provenance.sh

check-lockfile-idempotent: ## Guard: re-resolving pnpm-lock.yaml with the pinned pnpm is a no-op
	@./scripts/check-lockfile-idempotent.sh

check-package-licenses: ## Guard: every non-private package.json declares a license and ships its LICENSE
	@./scripts/check-package-licenses.sh

check-archive-licenses: ## Guard: every release archive ships the license text of the work inside it
	@./scripts/check-archive-licenses.sh

check-plugin-licenses: ## Guard: every plugin tarball ships the license text of the work inside it
	@./scripts/check-plugin-licenses.sh

check-plugin-release-latest: ## Guard: no plugin release claims the repo's "latest"
	@./scripts/check-plugin-release-latest.sh

check-tracked-binaries: ## Guard: no compiled executable (ELF/Mach-O/PE) is tracked in git
	@./scripts/check-tracked-binaries.sh

check-gofmt: ## Guard: every tracked .go file is gofmt-clean (gofmt -l -s); `make fmt` fixes
	@./scripts/check-gofmt.sh

# Not in `lint`: this one needs the pnpm workspace installed, so it runs in the
# frontend CI job (which always runs on a pull request and has already done
# `vp install`) rather than in the toolchain-free repo-guards job.
#
# The contract it protects is `vp check --fix` before committing frontend work.
# A tree that is not already a fixed point of its own formatter makes that
# contract rewrite files the contributor never touched, and the reviewer's job
# becomes separating the two. Nothing else notices: the drift is committed
# state, so every other check is green with it in place.
check-fmt-fixed-point: ## Guard: the tree is already a fixed point of `vp fmt` (needs `vp install`)
	@vp fmt . --check

workspace-paths: ## Print the resolved locations outside this repo (see docs/internals/workspace-paths.md)
	@echo "NEOKAPI_WORKSPACE_DIR = $(NEOKAPI_WORKSPACE_DIR)"
	@echo "NEOKAPI_CHECKOUTS_DIR = $(NEOKAPI_CHECKOUTS_DIR)"
	@echo "NEOKAPI_OKAPI_DIR     = $(NEOKAPI_OKAPI_DIR)"
	@echo "NEOKAPI_DOCLANG_DIR   = $(NEOKAPI_DOCLANG_DIR)"

check: fmt vet lint ## Run all code quality checks

check-framework: _fw-fmt _fw-vet _fw-lint ## Framework-only quality checks

check-bowrain: ## Bowrain-only quality checks
	@$(MAKE) -C bowrain check

# The docs playground compiles the CLI to js/wasm, so any dependency added to
# core/, host/, cli/ or kapi/ has to build for that target too — and plenty do
# not (a TTY, signals, cgo, the system clipboard). CI has always built it, but
# only in the docs workflows, so the failure arrived minutes after a push that
# `make pre-push` had passed. Compile only: no link, no gzip, ~1s warm.
check-wasm: i18n-catalogs ## Compile-check the in-browser CLI for js/wasm (the docs playground target)
	@cd kapi && GOOS=js GOARCH=wasm $(GO) build -o /dev/null ./cmd/kapi-wasm-cli
	@GOOS=js GOARCH=wasm $(GO) build -o /dev/null ./cmd/kapi-wasm
	@echo "✓ js/wasm builds (kapi-wasm-cli, kapi-wasm)"

test-parallel: ## Run all tests in parallel
	@$(MAKE) --no-print-directory _fw-test & $(MAKE) -C bowrain test & wait

# ── Generated build inputs ──────────────────────────────────────────────────

# The embedded gettext catalogs. core/i18n and host/i18n resolve
# `//go:embed catalogs/*.mo` at compile time, and the MO is build output — the
# repository carries the translated JSON it is compiled from. So every target
# that builds, tests, vets or lints either module declares this first, and a
# build that skipped it fails on the embed rather than silently shipping
# English.
#
# The compiler is pure Go in the framework module and imports neither package
# it writes for, so it builds and runs against an empty catalogs directory:
# no bootstrap cycle, and no kapi binary in the loop. It rewrites a catalog
# only when the bytes change, so a repeat run invalidates nothing.
i18n-catalogs: ## Compile the embedded MO catalogs from the committed translated JSON
	@$(GO) run ./core/i18n/gen/catalogs

# ── Framework test/quality internals ────────────────────────────────────────

_fw-fmt:
	$(GOFMT) -w -s core/ host/ cli/ kapi/ memory/ terms/ providers/

_fw-test: i18n-catalogs
	$(GOTEST_BASE) ./... -count=1
	cd host && $(GOTEST_BASE) ./... -count=1
	cd cli && $(GOTEST_BASE) ./... -count=1
	cd kapi && $(GOTEST_BASE) ./... -count=1
# The eval harnesses are their own workspace modules (they import bowrain's
# Bedrock provider), so the root ./... pattern never reaches them — without
# these lines their corpus/scoring gates would not run anywhere.
	cd scripts/batcheval && $(GOTEST_BASE) ./... -count=1
	cd scripts/contexteval && $(GOTEST_BASE) ./... -count=1

_fw-test-fast: i18n-catalogs
	$(GOTEST_BASE) ./...
	cd host && $(GOTEST_BASE) ./...
	cd cli && $(GOTEST_BASE) ./...
	cd kapi && $(GOTEST_BASE) ./...

_fw-test-unit: i18n-catalogs
	$(GOTEST_BASE) ./... -count=1 -short
	cd host && $(GOTEST_BASE) ./... -count=1 -short
	cd cli && $(GOTEST_BASE) ./... -count=1 -short
	cd kapi && $(GOTEST_BASE) ./... -count=1 -short

_fw-test-race: i18n-catalogs
	$(GOTEST) -race -shuffle=on ./... -count=1
	cd host && $(GOTEST) -race -shuffle=on ./... -count=1
	cd cli && $(GOTEST) -race -shuffle=on ./... -count=1
	cd kapi && $(GOTEST) -race -shuffle=on ./... -count=1

_fw-test-verbose: i18n-catalogs
	$(GOTEST_BASE) ./... -count=1 -v
	cd host && $(GOTEST_BASE) ./... -count=1 -v
	cd cli && $(GOTEST_BASE) ./... -count=1 -v
	cd kapi && $(GOTEST_BASE) ./... -count=1 -v

_fw-test-integration: i18n-catalogs
	@bash scripts/test-integration.sh $(_RACE) $(_SHUFFLE)

# host/ carries the runtime and every service the CLI dispatches to, so it is
# vetted and linted alongside the modules that import it — not left to the
# module that happens to build it.
_fw-vet: i18n-catalogs
	$(GOVET) ./...
	cd host && $(GOVET) ./...
	cd cli && $(GOVET) ./...
	cd kapi && $(GOVET) ./...
	cd scripts/batcheval && $(GOVET) ./...
	cd scripts/contexteval && $(GOVET) ./...

_fw-lint: i18n-catalogs
ifdef GOLANGCI_LINT
	$(GOLANGCI_LINT) run ./...
	cd host && $(GOLANGCI_LINT) run ./...
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
		core/proto/content/v1/*.proto core/proto/engine/v1/*.proto \
		core/proto/sync/v1/*.proto \
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

test-framework: i18n-catalogs ## Run framework module tests only (incl. the eval-harness modules)
	@mkdir -p $(COVER_DIR)
ifdef CI
# The eval harnesses (scripts/batcheval, scripts/contexteval) are separate
# workspace modules, so the root ./... pattern never reaches them. Their tests
# gate the corpora, the scoring, and the price-table sync — keyless and fast.
# One shell, `|| rc=$$?` per suite: a root-suite failure must not stop the eval
# suites from running (and from appearing in the JSON the reporters read) —
# the single-run form completed every package even when some failed.
	rc=0; \
	$(GOTEST_BASE) $(call cov,$(COVER_DIR)/framework.out) -json ./... > test-results-framework.json || rc=$$?; \
	( cd scripts/batcheval && $(GOTEST_BASE) -json ./... >> ../../test-results-framework.json ) || rc=$$?; \
	( cd scripts/contexteval && $(GOTEST_BASE) -json ./... >> ../../test-results-framework.json ) || rc=$$?; \
	exit $$rc
else
	$(GOTEST_BASE) ./... -count=1
	cd scripts/batcheval && $(GOTEST_BASE) ./... -count=1
	cd scripts/contexteval && $(GOTEST_BASE) ./... -count=1
endif

test-cli: i18n-catalogs ## Run host + cli module tests only
	@mkdir -p $(COVER_DIR)
ifdef CI
	cd host && $(GOTEST_BASE) $(call cov,../$(COVER_DIR)/host.out) -json ./... > ../test-results-cli.json
	cd cli && $(GOTEST_BASE) $(call cov,../$(COVER_DIR)/cli.out) -json ./... >> ../test-results-cli.json
else
	cd host && $(GOTEST_BASE) ./... -count=1
	cd cli && $(GOTEST_BASE) ./... -count=1
endif

test-kapi: i18n-catalogs ## Run kapi CLI tests only
	@mkdir -p $(COVER_DIR)
ifdef CI
	cd kapi && $(GOTEST_BASE) $(call cov,../$(COVER_DIR)/kapi.out) -json ./... > ../test-results-kapi.json
else
	cd kapi && $(GOTEST_BASE) ./... -count=1
endif

test-platform test-bowrain-plugin test-bowrain: i18n-catalogs ## Run individual bowrain module tests
	$(MAKE) -C bowrain $@

# ── EngineService example clients (contract lock) ───────────────────────────
# Each client starts `kapi engine serve`, extracts a JSON fixture, pseudo-
# translates it via Process, merges it back, and asserts the result is
# byte-identical to the CLI doing the same — locking the gRPC contract from
# two foreign languages. Run in CI (engine-examples job) and locally.
engine-examples: engine-examples-node engine-examples-python ## Run the EngineService Python + Node example clients against bin/kapi

engine-examples-node: build ## Run the Node EngineService example client
	cd examples/engine-client-node && npm install --no-audit --no-fund --silent
	cd examples/engine-client-node && $(KAPI_ISO_ENV) KAPI_BIN=$(BIN_DIR)/kapi node client.mjs

engine-examples-python: build ## Run the Python EngineService example client
	cd examples/engine-client-python && python3 -m venv .venv && .venv/bin/pip install -q -r requirements.txt
	cd examples/engine-client-python && $(KAPI_ISO_ENV) KAPI_BIN=$(BIN_DIR)/kapi .venv/bin/python client.py

# Bowrain Desktop backend tests run on their own (the bowrain module's
# `test-bowrain` excludes apps/bowrain under CI) because the Wails app backend
# needs the GTK/WebKit toolchain on Linux — mirrors `kapi-desktop-test`. Driven
# by the `bowrain-desktop` CI job.
bowrain-desktop-test: i18n-catalogs ## Run Bowrain Desktop Go backend tests
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

ci-test-bowrain: ## Run bowrain tests with full CI flags locally
	$(MAKE) -C bowrain CI=true test-bowrain

ci-test-kapi-desktop: ## Run Kapi Desktop tests with full CI flags locally
	$(MAKE) CI=true kapi-desktop-test

ci-test-bowrain-desktop: ## Run Bowrain Desktop tests with full CI flags locally
	$(MAKE) CI=true bowrain-desktop-test

ci-test-all: ## Run all module tests with full CI flags locally
	$(MAKE) CI=true test-framework test-cli test-kapi kapi-desktop-test bowrain-desktop-test
	$(MAKE) -C bowrain CI=true test-platform test-bowrain-plugin test-bowrain

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
	# The vocabulary packs are also gated in reference-data-drift.yml, but that
	# workflow's paths filter lists Go and reference-data sources — not the TS
	# copies. A pull request that adds a second TS copy touches neither, so the
	# gate would not run for the one change it exists to catch. This job has no
	# path gate on pull requests, so the guard runs here too.
	node scripts/format-ops/check-vocab-packs.mjs
	# Guard against vite-plus / vite-alias version drift before any vp check runs,
	# so the failure is an actionable one-liner rather than a puzzling TS2321.
	bash scripts/audit-vite-alias.sh
	# The tree must already be formatted the way `vp check --fix` formats it —
	# see check-fmt-fixed-point. ~0.4s over the whole workspace.
	$(MAKE) --no-print-directory check-fmt-fixed-point
	# bowrain/packages/ui and bowrain/apps/web consume @neokapi/{ui,flow-editor},
	# which import `@neokapi/i18n-react/runtime` (a built ./dist subpath export).
	# Build neokapi-i18n first so that subpath resolves (mirrors ci-kapi-desktop-frontend).
	cd packages/i18n-react && vp run build
	cd bowrain/packages/ui && vp check
	# `vp check` reads the workspace lint ignore set, which skips stories — so
	# nothing compiled a *.stories.tsx or the Storybook mock adapter, and the
	# mock drifted 62 members behind ApiAdapter without a job noticing (#1801).
	# tsc over the package tsconfig is what covers them.
	cd bowrain/packages/ui && vp run typecheck:stories
	cd bowrain/packages/ui && vp test
	# packages/app was never covered here, so a broken viewFromPath sat on main
	# from #1462 until #1533: the sidebar stopped highlighting on /terms and no
	# job noticed. It holds the routing every surface depends on.
	cd bowrain/packages/app && vp check
	cd bowrain/packages/app && vp test
	cd bowrain/apps/web && vp check
	cd bowrain/apps/web && vp test
	cd bowrain/apps/web && vp build
	cd bowrain/apps/bowrain/frontend && vp check
	cd bowrain/apps/ctrl && vp check
	# ctrl and pulse each sit on a subdomain of the domain the landing and docs
	# sites publish as cookieless, and each starts PostHog from its own module —
	# so each carries its own gate, and a job has to run it (#1940).
	cd bowrain/apps/ctrl && vp test
	cd bowrain/apps/pulse && vp check
	cd bowrain/apps/pulse && vp test
	cd bowrain/apps/keycloak-theme && vp check
	cd bowrain/emails && vp check
	# Non-blocking Storybook coverage report (informational; does not fail the job).
	node scripts/story-coverage.mjs || true

ci-kapi-desktop-frontend: ## Mirror the CI `kapi-desktop` job's frontend half (Go backend test is a separate step)
	# Bindings wrapper/id pairing + call-name reachability. Runs before the
	# typecheck because `tsc` cannot catch either: the dispatch layer types the
	# bindings as Record<string, …>, so a wrong method name is invisible to the
	# compiler and silently returns null at runtime.
	$(MAKE) check-wails-bindings
	cd packages/i18n-react && vp run build
	cd packages/ui && vp check
	cd packages/flow-editor && vp check
	cd apps/kapi-desktop/frontend && vp check
	cd packages/flow-editor && vp test
	cd apps/kapi-desktop/frontend && vp test
	cd storybook && vpx storybook build -o storybook-static

ci-bowrain-desktop-frontend: ## Mirror the CI `bowrain-desktop` job's frontend half (Go backend test is a separate step)
	# Wrapper/id pairing + id-map freshness (both apps). Repeated here rather
	# than only in ci-kapi-desktop-frontend because the two desktop jobs are
	# independently path-gated: a bowrain-only change never runs that job.
	$(MAKE) check-wails-bindings
	# Build neokapi-i18n first so its `/runtime` subpath export resolves for the
	# @neokapi/{ui,flow-editor} components the desktop frontend pulls in.
	cd packages/i18n-react && vp run build
	cd bowrain/apps/bowrain/frontend && vp test

ci-i18n-react: ## Mirror the CI `neokapi-i18n` job: typecheck/validate/test/build kapi-format + neokapi-i18n
	cd packages/kapi-format && vp run typecheck
	cd packages/kapi-format && vp run validate
	cd packages/kapi-format && vp run test
	cd packages/i18n-react && vp run typecheck
	cd packages/i18n-react && vp run test
	cd packages/i18n-react && vp run build

ci-build: i18n-catalogs ## Mirror the CI `build` job: build all three binaries (no fts5) + assert module isolation
	@mkdir -p bowrain/apps/web/dist && echo placeholder > bowrain/apps/web/dist/index.html
	@mkdir -p apps/kapi-desktop/frontend/dist && echo placeholder > apps/kapi-desktop/frontend/dist/index.html
	@# Deliberately no fts5 — this target proves the packages compile and the
	@# module boundaries hold, and nothing here is ever executed. The binaries
	@# go to a scratch directory rather than $(BIN_DIR) precisely because they
	@# lack FTS5: a non-fts5 kapi left at bin/kapi still runs, and dies only on
	@# its first memory or terms query.
	@mkdir -p $(BIN_DIR)/ci-build
	cd kapi && go build -o $(BIN_DIR)/ci-build/kapi ./cmd/kapi
	cd bowrain/plugin && go build -o $(BIN_DIR)/ci-build/kapi-bowrain ./cmd/kapi-bowrain
	cd bowrain && go build -o $(BIN_DIR)/ci-build/bowrain-server ./cmd/bowrain-server
	GOWORK=off bash -c "go build ./..."
	GOWORK=off bash -c "cd host && go build ./..."
	GOWORK=off bash -c "cd cli && go build ./..."
	GOWORK=off bash -c "cd bowrain/core && go build ./..."
	GOWORK=off bash -c "cd kapi && go build ./..."
	GOWORK=off bash -c "cd bowrain/plugin && go build ./..."
	@# kapi must not depend on platform / bowrain / heavy deps
	@if cd kapi && GOWORK=off go list -m all | grep -q 'neokapi/platform'; then echo "kapi should not depend on platform"; exit 1; fi
	@if cd bowrain && GOWORK=off go list -m all | grep -q 'neokapi/cli'; then echo "bowrain should not depend on cli"; exit 1; fi
	@if cd kapi && GOWORK=off go list -m all | grep -iE 'wails|labstack/echo|keycloak'; then exit 1; fi

# The plugins/* modules are outside go.work, so nothing in the workspace build
# would ever notice them drifting. They did: plugins/sat carried a stale
# golang.org/x/text, which made `GOWORK=off go test ./...` — i.e. the whole of
# `make test-sat-plugin` — refuse to run with "updates to go.mod needed", and
# plugins/vision and plugins/pdfium had incomplete go.sum/go.mod. Tidy them here
# so the module that no job builds still cannot rot.
ci-tidy: ## Mirror the CI `tidy-check` job: go mod tidy across all modules + fail on drift
	@for dir in . host cli kapi apps/kapi-desktop bowrain/core bowrain/plugin bowrain \
	            plugins/sat plugins/check plugins/vision plugins/asr plugins/av plugins/pdfium \
	            plugins/sourcecode; do \
	  echo "Checking $$dir..."; \
	  (cd "$$dir" && go mod tidy); \
	done
	@if ! git diff --exit-code -- '**go.mod' '**go.sum'; then \
	  echo "::error::go.mod/go.sum files are not tidy. Run 'make deps' locally."; \
	  exit 1; \
	fi

# ── Module Isolation ──────────────────────────────────────────────────────────

# Module isolation boundaries are asserted by `audit-modules` below (folded in
# from the retired `verify-isolation`, which matched on `go list -m all` and
# false-flagged kapi's legitimate go-keyring/go-oidc, erroring on a green tree).

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
# the main bowrain module — only bowrain/core/* is allowed under the bowrain/
# tree. This fails fast on a re-introduced bowrain/sync or bowrain/proto/v1
# import (which would otherwise re-add the require + replace on the main bowrain
# module and re-couple the framework-only core to redis/echo/the gRPC service
# surface).
#
# Four more import-level (go list -deps) assertions, matched on PACKAGES so a
# transitive dep cannot dodge them: every Apache-licensed module (., host, cli,
# kapi, apps/kapi-desktop) must import NO AGPL bowrain package — the license
# boundary, with no exception, which also enforces kapi↛bowrain; kapi must not
# link wails/echo; and the main bowrain module must
# not depend on cli. These replace the old `verify-isolation` target, which
# matched on `go list -m all` and so false-flagged kapi's legitimately linked
# go-keyring and (transitive) go-oidc — it errored on a green tree and ran in no
# workflow. oidc/keyring reach kapi legitimately (sigstore cosign, keychain),
# so they are deliberately not asserted against.
#
# Modules audited (path → expected boundary):
#   .                  framework — no host/cli/bowrain deps
#   host               framework only — the cobra-free app runtime (NO cobra)
#   cli                framework + host — the thin cobra shell; no bowrain dep
#   bowrain/core       framework only — no cli AND no main bowrain dep
#   kapi               framework + host + cli only — no bowrain dep
#   apps/kapi-desktop  framework + host only — NO cli, NO cobra
#   bowrain/plugin     framework + host + cli + bowrain/core (bowrain behavior + the kapi-bowrain plugin binary)
#   bowrain            framework + host + bowrain/core (the platform; host for the host/flowdef flow catalog)
#
# bowrain and bowrain/plugin are not isolation boundaries (they legitimately depend
# on several modules), but they are audited for the same go.mod/go.sum tidiness —
# e.g. a require that should be indirect after a package moves. CI's Tidy Check
# covers all modules, so they belong here too.
#
# Build pattern per module: most build ./..., but apps/kapi-desktop's main
# package embeds frontend/dist (//go:embed all:frontend/dist) which only exists
# after a frontend build, so — like `make kapi-desktop-test` — we build only
# ./backend/... for it. `go mod tidy` still resolves the whole module graph
# (embeds don't affect dependency resolution), so the boundary contract holds.
AUDIT_MODULES := . host cli bowrain/core kapi apps/kapi-desktop bowrain/plugin bowrain

audit-modules: i18n-catalogs ## Assert module isolation + go.mod/go.sum tidiness (fails on drift)
	@set -e; rc=0; for dir in $(AUDIT_MODULES); do \
	  echo ">> audit $$dir"; \
	  pkgs="./..."; [ "$$dir" = "apps/kapi-desktop" ] && pkgs="./backend/..."; \
	  ( cd "$$dir" && GOWORK=off $(GO) build $$pkgs ) || { echo "ERROR: $$dir failed isolated (GOWORK=off) build"; exit 1; }; \
	  if [ "$$dir" = "bowrain/core" ]; then \
	    bad=$$( cd "$$dir" && GOWORK=off $(GO) list -deps ./... 2>/dev/null \
	              | grep -E '^github\.com/neokapi/neokapi/bowrain(/|$$)' \
	              | grep -vE '^github\.com/neokapi/neokapi/bowrain/core(/|$$)' || true ); \
	    if [ -n "$$bad" ]; then \
	      echo "ERROR: bowrain/core must be framework-only — it imports the main bowrain module:"; \
	      echo "$$bad" | sed 's/^/    /'; \
	      echo "  (move the shared code down to where both sides may reach it — the framework, e.g. core/venue or core/proto/sync/v1)"; \
	      rc=1; \
	    fi; \
	  fi; \
	  if [ "$$dir" = "apps/kapi-desktop" ]; then \
	    bad=$$( cd "$$dir" && GOWORK=off $(GO) list -deps ./backend/... 2>/dev/null \
	              | grep -E '^(github\.com/spf13/cobra|github\.com/neokapi/neokapi/cli)(/|$$)' || true ); \
	    if [ -n "$$bad" ]; then \
	      echo "ERROR: kapi-desktop must stay cobra-free — it links the cli module or cobra:"; \
	      echo "$$bad" | sed 's/^/    /'; \
	      echo "  (the desktop depends on the host module only; move the needed symbol into host/)"; \
	      rc=1; \
	    fi; \
	  fi; \
	  case "$$dir" in \
	    .|host|cli|kapi|apps/kapi-desktop) \
	      bad=$$( cd "$$dir" && GOWORK=off $(GO) list -deps $$pkgs 2>/dev/null \
	                | grep -E '^github\.com/neokapi/neokapi/bowrain(/|$$)' || true ); \
	      if [ -n "$$bad" ]; then \
	        echo "ERROR: Apache-licensed module $$dir imports an AGPL bowrain package (there is no exception):"; \
	        echo "$$bad" | sed 's/^/    /'; \
	        rc=1; \
	      fi; \
	      ;; \
	  esac; \
	  if [ "$$dir" = "kapi" ]; then \
	    bad=$$( cd "$$dir" && GOWORK=off $(GO) list -deps ./... 2>/dev/null \
	              | grep -iE 'wailsapp/wails|labstack/echo' || true ); \
	    if [ -n "$$bad" ]; then \
	      echo "ERROR: kapi links a heavy bowrain-side dependency (wails/echo):"; \
	      echo "$$bad" | sed 's/^/    /'; \
	      rc=1; \
	    fi; \
	  fi; \
	  if [ "$$dir" = "bowrain" ]; then \
	    bad=$$( cd "$$dir" && GOWORK=off $(GO) list -deps ./... 2>/dev/null \
	              | grep -E '^github\.com/neokapi/neokapi/cli(/|$$)' || true ); \
	    if [ -n "$$bad" ]; then \
	      echo "ERROR: bowrain depends on the cli module (it composes over host, not cli):"; \
	      echo "$$bad" | sed 's/^/    /'; \
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

# check-module-boundaries asserts the package-level license/architecture
# boundaries the tree relies on but no CI job enforced (audit-modules, which also
# runs them, is a pre-push-only target): kapi-desktop must link neither cobra nor
# the cli module — the Apache desktop stays cli-free — bowrain/core must import
# no package from the AGPL main bowrain module, and no Apache-licensed module
# may reach a package under the AGPL tree at all.
#
# That last assertion carries no exception list, and it must never grow one. The
# separately-licensed recipe vocabulary that used to be the single allowed
# import is a package under host now, so nothing Apache reaches into bowrain/ —
# and an exception is how a boundary stops being one. If a package genuinely
# needs to be read from both sides, it belongs below the line rather than on an
# allowlist above it.
#
# Wired into CI as the `module-boundaries` job, gated on any_go OR kapi_desktop so
# a desktop-only PR that reaches for cobra is still caught (any_go has no
# apps/kapi-desktop filter).
APACHE_MODULES := . host cli kapi apps/kapi-desktop

check-module-boundaries: i18n-catalogs ## Assert kapi-desktop cli/cobra-free + Apache modules link no AGPL
	@bad=$$(cd apps/kapi-desktop && GOWORK=off $(GO) list -deps ./backend/... 2>/dev/null \
	          | grep -E '^(github\.com/spf13/cobra|github\.com/neokapi/neokapi/cli)(/|$$)' || true); \
	  if [ -n "$$bad" ]; then echo "ERROR: kapi-desktop must stay cobra-free and cli-free — it links:"; echo "$$bad" | sed 's/^/    /'; exit 1; fi
	@bad=$$(cd bowrain/core && GOWORK=off $(GO) list -deps ./... 2>/dev/null \
	          | grep -E '^github\.com/neokapi/neokapi/bowrain(/|$$)' \
	          | grep -vE '^github\.com/neokapi/neokapi/bowrain/core(/|$$)' || true); \
	  if [ -n "$$bad" ]; then echo "ERROR: bowrain/core must be framework-only — it imports the main bowrain module:"; echo "$$bad" | sed 's/^/    /'; exit 1; fi
	@set -e; for dir in $(APACHE_MODULES); do \
	  pkgs="./..."; [ "$$dir" = "apps/kapi-desktop" ] && pkgs="./backend/..."; \
	  bad=$$( cd "$$dir" && GOWORK=off $(GO) list -deps $$pkgs 2>/dev/null \
	            | grep -E '^github\.com/neokapi/neokapi/bowrain(/|$$)' || true ); \
	  if [ -n "$$bad" ]; then \
	    echo "ERROR: Apache-licensed module $$dir imports an AGPL bowrain package (there is no exception):"; \
	    echo "$$bad" | sed 's/^/    /'; exit 1; \
	  fi; \
	done
	@bad=$$(cd bowrain/plugin && GOWORK=off $(GO) list -deps ./cmd/kapi-bowrain 2>/dev/null \
	          | grep -E '^github\.com/neokapi/neokapi/bowrain(/|$$)' \
	          | grep -vE '^github\.com/neokapi/neokapi/bowrain/plugin(/|$$)' || true); \
	  if [ -n "$$bad" ]; then \
	    echo "ERROR: the kapi-bowrain binary is declared Apache-2.0 (bowrain/plugin/LICENSE) but links AGPL:"; \
	    echo "$$bad" | sed 's/^/    /'; \
	    echo "  Its manifest, the plugin registry and the Homebrew formula all declare Apache-2.0."; \
	    echo "  Either move what it needs below the line, or the declaration is false on three public surfaces."; \
	    exit 1; \
	  fi
	@echo "check-module-boundaries: kapi-desktop cli/cobra-free, bowrain/core framework-only, Apache modules AGPL-free, kapi-bowrain Apache-clean"

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

# The go invocation below spells both tags in one flag and calls $(GO), not
# $(GOTEST): go does not union repeated -tags, the last occurrence wins, and
# $(GOTEST) bakes in $(GOTAGS). `$(GOTEST) -tags parity` therefore dropped fts5
# and ran the parity suite — the adjudicator for every round-trip change — in a
# build configuration no other target uses. Nothing failed, because fts5 is a
# runtime SQL capability rather than a compile gate.
parity-test: parity-sandbox i18n-catalogs ## Run the full parity test suite (#448)
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
	    $(GO) test -tags "fts5,parity" -count=1 -timeout 60m ./parity/...
	@echo "Parity report: $(PARITY_REPORT)"

# Parity output stays inside the sandbox. It used to be published to
# web/static/data/ to back the /parity page; that page was retired with the
# bridge's product surface (#1073), so the summary is a local maintainer
# artifact now — nothing parity-related reaches the docs site.
PARITY_SUMMARY := $(PARITY_DIR)/parity-report.json

parity-publish: parity-test ## Run the parity suite and write the per-filter summary to .parity/
	@cd $(ROOT_DIR) && go run ./scripts/testcompare \
	    -in $(PARITY_REPORT) \
	    -out $(PARITY_SUMMARY)
	@echo "Parity summary: $(PARITY_SUMMARY)"

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
# Set NEOKAPI_OKAPI_DIR (or OKAPI_REPO) if your Okapi clone is not at the
# conventional $(NEOKAPI_CHECKOUTS_DIR)/okapi/Okapi.
# Set CONTRACT_FILTER to scope to a single filter (default: html).
#
# The canonical Okapi clone is the `Okapi` repository, cleanly tagged v1.48.0,
# matching the okapi-bridge framework_version. A sibling `okapi-java` clone is
# stuck on a stale `v1.4.8` tag and mislabels the dashboard version — do not
# use it for the contract audit (#611).

CONTRACT_DIR             := $(ROOT_DIR)/.contract-audit
CONTRACT_REPORT          := $(ROOT_DIR)/web/static/data/contract-audit.json
CONTRACT_FILTER          ?= html
BRIDGE_SCHEMAS           ?= $(NEOKAPI_WORKSPACE_DIR)/okapi-bridge/schemas
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
	    $(if $(wildcard $(CONTRACT_FAILSAFE)),-okapi-failsafe $(CONTRACT_FAILSAFE),) \
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
	    $(if $(wildcard $(CONTRACT_FAILSAFE)),-okapi-failsafe $(CONTRACT_FAILSAFE),) \
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

# Busybox multi-call links: kgrep / ksed / kcat / kconv / kdiff dispatch to the
# kapi binary by argv[0] (see cli BusyboxRoot). The Homebrew formula creates
# these symlinks on install; mirror that locally so `make build` yields the short
# commands too.
LINK_KAPI_BUSYBOX = for n in kgrep ksed kcat kconv kdiff; do ln -sf kapi $(BIN_DIR)/$$n; done

build: i18n-catalogs ## Build the kapi CLI (Apache-2.0; manifest-driven plugins discovered at runtime)
	@mkdir -p $(BIN_DIR)
	cd kapi && $(GOBUILD) $(LDFLAGS) -o $(BIN_DIR)/kapi ./cmd/kapi
	@$(LINK_KAPI_BUSYBOX)

# File-path alias so targets can declare a `bin/kapi` prerequisite (e.g. the
# l10n-* and *-pseudo-translate dogfood targets). `build` is phony, so this
# always reruns it — that's intended: callers want a CLI built from current
# source. Without this rule a clean checkout fails with
# "No rule to make target 'bin/kapi'".
bin/kapi: build

build-bowrain-plugin: i18n-catalogs ## Build the kapi-bowrain plugin binary (manifest-driven)
	@mkdir -p $(BIN_DIR)
	cd bowrain/plugin && $(GOBUILD) $(LDFLAGS) -o $(BIN_DIR)/kapi-bowrain ./cmd/kapi-bowrain

PLUGIN_DIR := packages/kapi-claude-plugin
PLUGIN_SKILLS_DIR := $(PLUGIN_DIR)/plugins/kapi/skills
SKILLS_SRC := cli/skills/data
plugin-bundle: ## Assemble the Claude Code plugin skills/ from the source tree (gitignored; built for release)
	@rm -rf $(PLUGIN_SKILLS_DIR)
	@mkdir -p $(PLUGIN_SKILLS_DIR)
	@cp -R $(SKILLS_SRC)/kapi $(PLUGIN_SKILLS_DIR)/kapi
	@echo "Assembled $(PLUGIN_SKILLS_DIR) from $(SKILLS_SRC)"

# publish-plugin syncs the assembled marketplace bundle to the neokapi-plugins
# marketplace repo. PLUGIN_MARKETPLACE_REPO defaults to the public repo; override
# for testing. Requires push access (a PAT/deploy key in CI).
PLUGIN_MARKETPLACE_REPO ?= neokapi/claude-plugins
publish-plugin: plugin-bundle ## Sync the assembled plugin bundle → the neokapi-plugins marketplace repo
	scripts/publish-plugin.sh "$(PLUGIN_DIR)" "$(PLUGIN_MARKETPLACE_REPO)"

# publish-skill mirrors the portable Agent Skill into the neokapi/agent-skills
# collection (under agent-skills/kapi/) so `npx skills add neokapi/agent-skills`
# installs it into any SKILL.md-aware tool (Copilot, Cursor, Windsurf, …).
# Requires push access.
SKILL_REPO ?= neokapi/agent-skills
publish-skill: ## Sync the portable skill → the neokapi/agent-skills collection (npx skills add)
	scripts/publish-skill.sh "$(SKILLS_SRC)/kapi" "$(SKILL_REPO)"

publish-integrations: publish-plugin publish-skill ## Publish both the Claude plugin and the portable skill

dev-skills: ## Copy the bundled skills into ./.claude/skills for in-repo dogfooding (gitignored)
	@mkdir -p .claude/skills
	@rm -rf .claude/skills/kapi
	@cp -R $(SKILLS_SRC)/kapi .claude/skills/kapi
	@echo "Copied skills into .claude/skills (gitignored; canonical source is $(SKILLS_SRC))"

build-all: i18n-catalogs ## Build all Go binaries
	@mkdir -p $(BIN_DIR)
	cd kapi && $(GOBUILD) $(LDFLAGS) -o $(BIN_DIR)/kapi ./cmd/kapi
	@$(LINK_KAPI_BUSYBOX)
	cd bowrain/plugin && $(GOBUILD) $(LDFLAGS) -o $(BIN_DIR)/kapi-bowrain ./cmd/kapi-bowrain
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

# ── kapi-pdfium PDF reader plugin ────────────────────────────────────────────
# A Mode-C (daemon/gRPC) plugin that reads PDFs via Google's PDFium (cgo,
# go-pdfium). Isolated in a subprocess so a malformed PDF can't crash kapi, and
# the heavy dependency stays out of the core binary. For DISTRIBUTION the
# PDFium SHARED library is bundled beside the binary and found via rpath
# (scripts/package-pdfium-plugin.sh) — the kapi-sat pattern; no static archive,
# no ICU-coexistence concern. For local dev, point PKG_CONFIG_PATH at a libpdfium
# .pc and run with the lib on the loader path. See plugins/pdfium/README.md.
# The pdfium_experimental tag wires go-pdfium's experimental marked-content APIs,
# which the tagged-PDF structure path (internal/pdfreader/structtree.go) needs to
# bridge struct elements to text. The bundled bblanchon libpdfium exports them;
# without the tag the plugin still builds and falls back to geometric structure.
build-pdfium-plugin: ## Build the kapi-pdfium plugin (CGO; needs libpdfium on PKG_CONFIG_PATH)
	@mkdir -p $(BIN_DIR)
	cd plugins/pdfium && GOWORK=off CGO_ENABLED=1 \
		$(GO) build -tags pdfium_experimental $(LDFLAGS) -o $(BIN_DIR)/kapi-pdfium ./cmd/kapi-pdfium

package-pdfium-plugin: ## Package a kapi-pdfium tarball for the host platform (CGO; needs PDFIUM_DIR = extracted bblanchon pdfium)
	@test -n "$(PDFIUM_DIR)" || { echo "set PDFIUM_DIR to an extracted bblanchon pdfium dir (include/ + lib/)"; exit 1; }
	scripts/package-pdfium-plugin.sh --version "$(VERSION)" --pdfium-dir "$(PDFIUM_DIR)" --out-dir "$(BIN_DIR)/pdfium-dist"

test-pdfium-plugin: ## Run kapi-pdfium tests (CGO; needs libpdfium on PKG_CONFIG_PATH + loader path)
	cd plugins/pdfium && GOWORK=off CGO_ENABLED=1 $(GO) test -tags pdfium_experimental ./...

# ── kapi-vision document-vision (OCR) plugin ─────────────────────────────────
# Mirrors kapi-sat: a cgo, -tags onnx sidecar that loads the onnxruntime SHARED
# library at runtime. The default build (no tag) is pure Go (stub engine) and
# CI-safe; -tags onnx links the real RapidOCR/PP-OCRv5 pipeline. For DISTRIBUTION
# the release tarball is self-contained — it bundles the onnxruntime shared lib
# (lib/) AND the PP-OCRv5 model assets (models/) beside the binary
# (scripts/package-vision-plugin.sh). onnxruntime is pinned to 1.25.0 to match
# yalue/onnxruntime_go (ORT_API_VERSION 25). See plugins/vision/README.md.
build-vision-plugin: ## Build the kapi-vision plugin (no ONNX backend; pure Go)
	@mkdir -p $(BIN_DIR)
	cd plugins/vision && GOWORK=off $(GO) build $(LDFLAGS) -o $(BIN_DIR)/kapi-vision ./cmd/kapi-vision

build-vision-plugin-onnx: ## Build kapi-vision WITH the ONNX backend (CGO; loads onnxruntime at runtime)
	@mkdir -p $(BIN_DIR)
	cd plugins/vision && GOWORK=off CGO_ENABLED=1 \
		$(GO) build $(LDFLAGS) -tags onnx -o $(BIN_DIR)/kapi-vision ./cmd/kapi-vision

# Package a self-contained distribution tarball for the HOST platform: builds
# kapi-vision -tags onnx, bundles the onnxruntime shared lib + PP-OCRv5 models.
#   VISION_ORT_DIR     extracted onnxruntime release dir (microsoft/onnxruntime)
#   VISION_MODELS_DIR  dir with ppocrv5_det.onnx / ppocrv5_rec.onnx / ppocrv5_dict.txt
package-vision-plugin: ## Package a kapi-vision tarball for the host platform (CGO; needs VISION_ORT_DIR + VISION_MODELS_DIR)
	@test -n "$(VISION_ORT_DIR)" || { echo "set VISION_ORT_DIR to the extracted onnxruntime release dir"; exit 1; }
	@test -n "$(VISION_MODELS_DIR)" || { echo "set VISION_MODELS_DIR to the dir with ppocrv5_* assets"; exit 1; }
	scripts/package-vision-plugin.sh \
		--version "$(VERSION)" \
		--ort-dir "$(VISION_ORT_DIR)" \
		--models-dir "$(VISION_MODELS_DIR)" \
		--out-dir "$(BIN_DIR)/vision-dist"

test-vision-plugin: ## Run kapi-vision pure-Go tests (protocol + algorithms + models cache)
	cd plugins/vision && GOWORK=off $(GO) test ./...

test-sat-plugin: ## Run kapi-sat pure-Go tests (protocol + algorithm + cache)
	cd plugins/sat && GOWORK=off $(GO) test ./...

test-asr-plugin: ## Run kapi-asr pure-Go tests (protocol + whisper model plumbing)
	cd plugins/asr && GOWORK=off $(GO) test ./...

# The plugin modules that need nothing but a Go toolchain, aggregated so CI has
# one target to call. Deliberately NOT here:
#   • pdfium — links libpdfium; see test-pdfium-plugin, which needs the library
#     on PKG_CONFIG_PATH and the loader path.
#   • av — ships no test files (only cmd/kapi-av).
# The -tags onnx suites for vision and sat need a real onnxruntime and stay in
# the nightly vision-onnx job. Each module is its own go.mod outside go.work,
# hence GOWORK=off in each recipe.
test-plugins: ## Run the plugin modules whose tests need no system library (sat, check, vision, asr, sourcecode)
	@$(MAKE) --no-print-directory test-sat-plugin
	@$(MAKE) --no-print-directory test-check-plugin
	@$(MAKE) --no-print-directory test-vision-plugin
	@$(MAKE) --no-print-directory test-asr-plugin
	@$(MAKE) --no-print-directory test-sourcecode-plugin

# ── kapi-sourcecode reader plugin ────────────────────────────────────────────
# Reads the prose out of source files with tree-sitter grammars, so a string
# that is a path, a flag or an identifier is never graded as a sentence.
#
# cgo, but no system dependency: the grammars are vendored C compiled by the Go
# build, so unlike kapi-pdfium this needs nothing on PKG_CONFIG_PATH and its
# tests belong on the PR gate. Isolated in a subprocess anyway — a parser fault
# on a malformed file stays in the plugin.
#
# READ-ONLY by design: the manifest declares capabilities ["read"] and there is
# no writer. See plugins/sourcecode/internal/proseread for why.
build-sourcecode-plugin: ## Build the kapi-sourcecode reader plugin → bin/kapi-sourcecode
	cd plugins/sourcecode && GOWORK=off CGO_ENABLED=1 $(GO) build -o $(BIN_DIR)/kapi-sourcecode ./cmd/kapi-sourcecode

test-sourcecode-plugin: ## Run kapi-sourcecode tests (grammar-driven prose extraction)
	cd plugins/sourcecode && GOWORK=off CGO_ENABLED=1 $(GO) test ./...

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

# Only the plugin set is pinned — KAPI_NO_PROJECT stays unset, so dev mode
# still opens real projects (and the sample projects) under your real config
# and keychain, the way a real user would. Without this, the app picks up
# whatever plugins happen to be installed on this machine (Homebrew, a stray
# $KAPI_PLUGINS_DIR left over from something else, …), so what dev mode can
# exercise varies by developer and by day. .dev-plugins/ starts empty
# (gitignored); place a plugin's install directory in it to test against.
KAPI_DESKTOP_DEV_PLUGINS_ENV := KAPI_PLUGINS_DIR_ONLY=1 KAPI_PLUGINS_DIR=$(CURDIR)/$(KAPI_DESKTOP_DIR)/.dev-plugins

kapi-desktop-dev: kapi-desktop-frontend-deps ## Run Kapi Desktop in dev mode (hot reload)
	@mkdir -p $(KAPI_DESKTOP_DIR)/.dev-plugins
	cd $(KAPI_DESKTOP_DIR) && $(KAPI_DESKTOP_DEV_PLUGINS_ENV) wails3 dev

regen-kapimart-sample: build ## Regenerate the KapiMart sample's history (targets, content memory, unit-state ledger)
	@./apps/kapi-desktop/backend/sample/gen/regenerate.sh

kapi-desktop-test: i18n-catalogs ## Run Kapi Desktop Go backend tests
	cd $(KAPI_DESKTOP_DIR) && $(GOTEST_BASE) ./backend/... -count=1 -timeout 180s

# ── Wails bindings (committed AND regenerated at release) ───────────────────
# Both desktop apps commit their generated bindings (.gitignore re-includes them
# with `!`) *and* regenerate them during release.yml / release-bowrain.yml. Two
# copies of one generated artifact drift, and nothing in the ordinary build
# notices: the shipped app gets fresh bindings while local dev, `wails3 dev` and
# the CI typecheck all build against the committed copy.
#
# Bindings are a static analysis of the service API, so the output is invariant
# to the -ldflags content, to CGO_ENABLED, and to the wails3 CLI version — all
# three verified byte-identical on both apps. That is what makes a byte gate
# honest, and it is also why the release steps' cgo-on generation and this
# cgo-off generation agree despite differing.
#
# Hence no version stamp is threaded here, and cgo is off: with it on, the ICU
# cgo packages compile during the analysis, which is what got the kapi-desktop
# generator SIGTERM'd on a CI runner (852 packages, ~46s cold vs ~6s).
# release.yml already ran bindings cgo-off on Windows for the same reason, noting
# the surface is cgo-independent.
#
# Unlike the release steps this deliberately does NOT run `go mod tidy`, which
# would mutate go.mod/go.sum and collide with the `tidy-check` gate.
#
# NOTE: the release workflows still carry their own copy of this invocation and
# install the wails3 CLI with `@latest`. Folding them onto these targets (and a
# pinned CLI) is the obvious next step, but their desktop matrix includes Windows
# runners where `make` is not reliably on PATH, and the release path only runs on
# tags — so it is not verifiable from a PR and is deliberately left alone here.
WAILS_BINDINGS_ENV   := CGO_ENABLED=0
WAILS_BINDINGS_FLAGS := -f "-tags production,fts5 -trimpath -buildvcs=false -ldflags=\"\"" -clean=true
BOWRAIN_DESKTOP_DIR  := bowrain/apps/bowrain

# Install the wails3 CLI at the exact wails version the apps build against, so
# the generator can never disagree with the library. `go install …@latest` would
# make the byte gate drift repo-wide the day upstream tags a release; deriving
# the pin from go.mod keeps one source of truth. (Verified byte-identical output
# across two CLI versions, so the pin is future-proofing, not a current fix.)
#
# One CLI on PATH serves both desktop apps, so their pins must agree — assert it
# rather than silently generating one app's bindings with the other's generator.
# The queries run with GOWORK=off deliberately: inside the workspace `go list -m`
# reports the MVS-unified version, so both modules would always look identical
# and the assertion would never fire.
wails3-cli: ## Install the wails3 CLI pinned to the wails version both desktop apps require
	@kv="$$(cd $(KAPI_DESKTOP_DIR) && GOWORK=off $(GO) list -m -f '{{.Version}}' github.com/wailsapp/wails/v3)"; \
	bv="$$(cd $(BOWRAIN_DESKTOP_DIR)/../.. && GOWORK=off $(GO) list -m -f '{{.Version}}' github.com/wailsapp/wails/v3)"; \
	if [ "$$kv" != "$$bv" ]; then \
		echo "wails3-cli: the desktop apps pin different wails versions (kapi-desktop=$$kv, bowrain=$$bv);"; \
		echo "  one wails3 CLI cannot generate both apps' bindings faithfully — align the two go.mod files."; \
		exit 1; \
	fi; \
	echo "wails3-cli: installing wails3 $$kv"; \
	$(GO) install github.com/wailsapp/wails/v3/cmd/wails3@$$kv

kapi-desktop-bindings: i18n-catalogs ## Regenerate the committed Kapi Desktop Wails bindings + wbridge id map
	cd $(KAPI_DESKTOP_DIR) && $(WAILS_BINDINGS_ENV) wails3 generate bindings $(WAILS_BINDINGS_FLAGS)
	cd $(KAPI_DESKTOP_DIR)/frontend && node scripts/gen-wails-id-map.mjs

bowrain-desktop-bindings: i18n-catalogs ## Regenerate the committed Bowrain Desktop Wails bindings + wbridge id map
	cd $(BOWRAIN_DESKTOP_DIR) && $(WAILS_BINDINGS_ENV) wails3 generate bindings $(WAILS_BINDINGS_FLAGS)
	cd $(BOWRAIN_DESKTOP_DIR)/frontend && node scripts/gen-wails-id-map.mjs

wails-bindings: kapi-desktop-bindings bowrain-desktop-bindings ## Regenerate both desktop apps' committed Wails bindings

# Node-only semantic gate: proves each JS wrapper name and the id it passes to
# $Call.ByID are a consistent pair (recomputing Wails' own fnv32a(FQN)), that the
# wbridge id maps are app.js's projection, and that every name kapi-desktop
# dispatches through `call("Name")` is actually exported. No Go toolchain, ~0.1s
# — so it rides along in the cheap frontend job for both apps.
check-wails-bindings: ## Gate: Wails wrapper/id pairing, id maps, and call-name reachability (both desktop apps)
	node scripts/check-wails-bindings.mjs

# Byte-drift gates: regenerate and fail if the committed copy differs. These need
# the Go toolchain and the wails3 CLI (see wails3-cli), so they run in the
# path-gated desktop jobs that already set Go up — no C toolchain or libicu-dev,
# since generation is cgo-off. `git diff` alone misses a *new* generated file, so
# untracked output under bindings/ fails too — that is exactly how the memory/ +
# terms/ package rename slipped through in the first place.
check-kapi-desktop-bindings: kapi-desktop-bindings ## Drift gate: committed Kapi Desktop bindings regenerate identically
	git diff --exit-code $(KAPI_DESKTOP_DIR)/frontend/bindings \
		$(KAPI_DESKTOP_DIR)/frontend/src/demo/wails-id-map.generated.json
	@untracked="$$(git ls-files --others --exclude-standard $(KAPI_DESKTOP_DIR)/frontend/bindings)"; \
	if [ -n "$$untracked" ]; then \
		echo "check-kapi-desktop-bindings: regeneration produced untracked files (commit them):"; \
		echo "$$untracked" | sed 's/^/  /'; \
		exit 1; \
	fi
	@echo "check-kapi-desktop-bindings: committed bindings match the Go backend"

check-bowrain-desktop-bindings: bowrain-desktop-bindings ## Drift gate: committed Bowrain Desktop bindings regenerate identically
	git diff --exit-code $(BOWRAIN_DESKTOP_DIR)/frontend/bindings \
		$(BOWRAIN_DESKTOP_DIR)/frontend/src/demo/wails-id-map.generated.json
	@untracked="$$(git ls-files --others --exclude-standard $(BOWRAIN_DESKTOP_DIR)/frontend/bindings)"; \
	if [ -n "$$untracked" ]; then \
		echo "check-bowrain-desktop-bindings: regeneration produced untracked files (commit them):"; \
		echo "$$untracked" | sed 's/^/  /'; \
		exit 1; \
	fi
	@echo "check-bowrain-desktop-bindings: committed bindings match the Go backend"

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

# Invoke the neokapi-i18n CLI by its built entrypoint rather than `vpx neokapi-i18n`,
# which resolves the bin via the workspace and falls back to an npm fetch (404)
# in a fresh CI checkout where the bin isn't linked. node-on-dist is environment
# independent — it only needs the package built (the i18n-react-build prereq).
NEOKAPI_I18N_CLI := node $(CURDIR)/packages/i18n-react/dist/cli.js

i18n-react-build: ## Build @neokapi/i18n-react (runtime + vite plugin + CLI) into dist/
	cd packages/i18n-react && vp run build

# ── Multilingual surfaces (the dogfood loop) ─────────────────────────────────
#
# The repo dogfoods kapi through the root kapi.yaml recipe, which declares
# every surface as a content collection with a `target:` template. Bringing all
# of them up to date is one kapi verb between two build stages:
#
#   l10n-extract    source strings out of React and Go source    — kapi cannot
#   l10n-converge   the whole recipe, one `kapi up`              — the loop
#   l10n-compile    catalogs into shippable runtime dictionaries — kapi cannot
#
# The extractors and the catalog compilers stay outside the recipe, because a
# recipe may not name a subprocess (AD-038: "a recipe is trusted" is the
# assumption execution trust exists to disprove). Everything between them is
# one `kapi up`: it compiles the committed context under `.kapi/` into the
# project store itself — keyed by each bundle's content digest, so an unchanged
# bundle costs a read and a pulled edit recompiles exactly itself — re-extracts
# the block store from the working tree, runs the recipe's flow over every
# collection and locale, and writes the targets. There are no per-surface
# targets, because a per-surface target would be a hand-rolled subset of what
# the recipe already declares; `kapi up --plan` reports the pending work per
# (collection, locale) without running anything, and `kapi up --passes 1` is a
# single pass for a quick iteration.
#
# `make l10n` runs all three. `make l10n-build` runs the two build stages with no
# loop between them, and `make l10n-verify` runs that and fails if a
# BUILD-DERIVED artifact moved — a generated-vs-source gate over the tier that
# is a function of committed source alone. The target-language tier is the
# loop's: reported by `make l10n-report`, gated by nothing, because pending
# target work is normal and must never fail a build (CLAUDE.md, "Target-language
# drift").
#
# The `l10n-*` names are retained spellings on a developer-facing internal
# surface; the concept is the repo's own multilingual content.

L10N_LANGS := nb

# The convergence stage binds the dogfood project deliberately — it is the one
# workflow the isolation contract in CLAUDE.md carves out — so it does NOT set
# KAPI_NO_PROJECT. It does pin the venue: in an install that carries
# kapi-bowrain, `kapi up` dispatches the loop to the server, and `make l10n`
# would stop being the local, credential-free pass over the committed context
# that a developer, a fresh clone and the derived-artifact gate all depend on.
# Discovering no plugins is what keeps the venue local; the nightly
# (dogfood-sync.yml) is where the server venue runs, deliberately.
KAPI_LOOP_ENV := KAPI_PLUGINS_DIR_ONLY=1

KAPI_DESKTOP_FRONTEND := $(KAPI_DESKTOP_DIR)/frontend
BOWRAIN_APP_DIR       := bowrain/packages/app
EMAILS_DIR            := bowrain/emails
LANDING_DIR           := bowrain/web/landing

# These flags and each package's own `extract` script are one definition in two
# places, so `check-extract-fixtures` compares them: a surface with a package
# script must be invoked with the same flag set from either side.
#
# An --ignore is a glob `fs/promises.glob` matches against the path it yields —
# `../` prefix and all — and `**` does not match a leading `..` segment. A bare
# `**/…` ignore therefore fires only for --src roots that are themselves
# unprefixed, and is dead for every `../`-prefixed root. So the exclude set is
# spelled once per distinct prefix among a surface's roots: $(call
# src-ignores,<prefix>) is one rooted copy, and a surface passes every prefix
# its --src flags use.
#
# What the set excludes is fixtures — tests and stories. Their strings are not
# product copy: extracted, they ship in the runtime catalogs and reach the
# translate stage as if a human had written them for users.
src-ignores = --ignore "$(1)**/*.stories.tsx" --ignore "$(1)**/*.test.tsx" --ignore "$(1)**/__tests__/**" --ignore "$(1)**/stories/**"
# The bowrain surfaces also keep their `demo/` fixture trees out.
bowrain-ui-ignores = $(call src-ignores,$(1)) --ignore "$(1)**/demo/**"

# One variable per surface, and the extract targets below run exactly these —
# so `l10n-extract-globs` can hand the whole set to the fixture guard and be
# checking what the pipeline actually scans.
KAPI_DESKTOP_EXTRACT_SRC := --src "src/**/*.{tsx,jsx}" --src "../../../packages/ui/src/**/*.tsx" --src "../../../packages/flow-editor/src/**/*.tsx" --src "../../../packages/status-views/src/**/*.tsx" --src "../../../packages/context-explorer/src/**/*.tsx" $(call src-ignores,) $(call src-ignores,../../../)
BOWRAIN_UI_IGNORES        := $(call bowrain-ui-ignores,)
BOWRAIN_APP_EXTRACT_SRC   := --src "src/**/*.{tsx,jsx}" --src "../ui/src/**/*.tsx" --src "../../apps/web/src/**/*.tsx" --src "../../apps/bowrain/frontend/src/**/*.tsx" --src "../../../packages/context-explorer/src/**/*.tsx" $(BOWRAIN_UI_IGNORES) $(call bowrain-ui-ignores,../) $(call bowrain-ui-ignores,../../) $(call src-ignores,../../../)
# ctrl and pulse each scan only their own src/, so the unprefixed set is whole.
BOWRAIN_SHELL_EXTRACT_SRC := --src "src/**/*.{tsx,jsx}" $(BOWRAIN_UI_IGNORES)
EMAILS_EXTRACT_SRC        := --src "src/*.tsx" --ignore "src/*.stories.tsx" --ignore "src/storybook-decorator.tsx"
LANDING_EXTRACT_SRC       := --src "src/**/*.{tsx,jsx}"

# A surface whose own wrapper components render translatable text carries a
# componentMap in a neokapi-i18n.config.json, and *both* halves of the pipeline
# have to read it: the vite transform imports the file unconditionally, while
# the extract CLI only sees it when handed --config. A key is a hash over the
# resolved JSX path, so a map present on one side only produces catalog keys the
# running app never asks for — the string is extracted, translated, compiled and
# then silently unresolvable. Surfaces with no such config pass no flag.
KAPI_DESKTOP_EXTRACT_CONFIG  := --config neokapi-i18n.config.json
BOWRAIN_APP_EXTRACT_CONFIG   := --config neokapi-i18n.config.json
# ctrl and pulse render the same shared app components, so they read the app's map.
BOWRAIN_SHELL_EXTRACT_CONFIG := --config ../../packages/app/neokapi-i18n.config.json
EMAILS_EXTRACT_CONFIG        := --config neokapi-i18n.config.json
LANDING_EXTRACT_CONFIG       :=

# The directory every recorded source path is relative to, for the two surfaces
# whose --src roots reach outside their own package.
#
# Left to the working directory, a catalog records its source as
# `../../apps/bowrain/frontend/src/App.tsx` — a path that names a real file only
# to someone who knows which directory the extract ran in. bowrain, holding the
# catalog and not the checkout, does not: it showed a reviewer a row reading
# `apps/bowrain/frontend/src/App.kbf.json`, which looks like a repository path
# and is not one. Declared at the repository root the same file records as
# `bowrain/apps/bowrain/frontend/src/App.tsx`, which means the same thing read
# from anywhere.
#
# This is the document's IDENTITY as well as what a reviewer reads — it spells
# every block id under it — so moving a root re-keys that surface's catalogs and
# its stored blocks. Declare it once and leave it: see
# docs/internals/l10n-ci.md.
#
# ctrl, pulse, emails and landing scan only their own `src/`, where the working
# directory is already the root, so they declare nothing and record what they
# always did.
KAPI_DESKTOP_SOURCE_ROOT := ../../..
BOWRAIN_APP_SOURCE_ROOT  := ../../..

# The whole argv each extract target runs, so `l10n-extract-globs` prints the
# real invocation and the guard checks what the pipeline actually does.
KAPI_DESKTOP_EXTRACT_FLAGS  := $(KAPI_DESKTOP_EXTRACT_CONFIG) --out i18n/ --source-root $(KAPI_DESKTOP_SOURCE_ROOT) --target-locale qps $(KAPI_DESKTOP_EXTRACT_SRC)
BOWRAIN_APP_EXTRACT_FLAGS   := $(BOWRAIN_APP_EXTRACT_CONFIG) --out i18n/ --source-root $(BOWRAIN_APP_SOURCE_ROOT) --target-locale qps $(BOWRAIN_APP_EXTRACT_SRC)
BOWRAIN_SHELL_EXTRACT_FLAGS := $(BOWRAIN_SHELL_EXTRACT_CONFIG) --out i18n/ --target-locale qps $(BOWRAIN_SHELL_EXTRACT_SRC)
EMAILS_EXTRACT_FLAGS        := $(EMAILS_EXTRACT_CONFIG) --out i18n/ --target-locale qps $(EMAILS_EXTRACT_SRC)
LANDING_EXTRACT_FLAGS       := $(LANDING_EXTRACT_CONFIG) --out i18n/ --target-locale qps $(LANDING_EXTRACT_SRC)

# Every surface whose source catalogs are per-file .kbf.json under <dir>/i18n
# and whose targets land in <dir>/i18n-<lang>. The pseudo-translation pass and
# the recipe's collections both key off this shape, so it is one list.
L10N_KBF_DIRS := $(KAPI_DESKTOP_FRONTEND) $(BOWRAIN_APP_DIR) bowrain/apps/ctrl bowrain/apps/pulse $(EMAILS_DIR) $(LANDING_DIR)

# The landing page is the one entry in that list whose qps build is PUBLISHED
# (bowrain.cloud/qps/) rather than read locally by a developer. It takes its
# marker setting from the recipe, the way the docs sites do; every other surface
# keeps the ▒ brackets, because there they are how an untranslated string is
# spotted at a glance.
L10N_KBF_PROBE_DIRS := $(filter-out $(LANDING_DIR),$(L10N_KBF_DIRS))

# <surface dir>:<compile output, relative to it>. The bowrain app compiles the
# one catalog tree into both shells that render it.
L10N_COMPILE_TARGETS := \
	$(KAPI_DESKTOP_FRONTEND):public/translations \
	$(BOWRAIN_APP_DIR):../../apps/web/public/translations \
	$(BOWRAIN_APP_DIR):../../apps/bowrain/frontend/public/translations \
	bowrain/apps/ctrl:public/translations \
	bowrain/apps/pulse:public/translations \
	$(EMAILS_DIR):translations \
	$(LANDING_DIR):translations

# The committed artifacts this pipeline owns, in two tiers told apart by what
# each is a function of.
#
# BUILD-DERIVED (L10N_DERIVED) — committed source alone. The two generated
# inventories the extract stage writes out of the Go registries and the cobra
# command tree; the English email renders; and the whole `qps` probe tier, which
# is a mechanical expansion of the extracted source catalogs and is then
# compiled like any other locale. Nothing here passes through the project store,
# so regenerating it must reproduce it byte for byte — l10n-verify says so.
#
# LOOP-OWNED (L10N_LOOP_OWNED) — the target-language tier `kapi up` writes out
# of the project store, which holds the union of what git carries and what a
# venue pull brought home. A byte gate over it would require a checkout with no
# server to reproduce wording a reviewer approved on one, and would overwrite
# that wording the moment it could not. So this tier carries no byte gate.
#
# It is not ungated. What cannot be asserted about it is that it reproduces;
# what can be asserted is that what was written in it is sound — it parses, it
# carries its source's placeholders, and it did not translate a machine
# identifier the recipe never declared translatable. `make l10n-content-check`
# reads the committed tier, `scripts/check-sync-backed.sh` refuses a run whose
# output fails, and l10n-collapse-check still asserts existence. Coverage stays
# reported and never gated (`make l10n-report`), because a string the target
# does not carry falls back to its source, which is pending work.
#
# The compiled runtime dictionaries split the same way, and by the same rule:
# `translations/qps.json` is compiled from the pseudo expansion of the source
# catalogs, `translations/<lang>.json` from what the loop materialized. The qps
# side keeps every surface's compiler under the byte gate, so a compiler change
# still surfaces as drift.
#
# Two entries are git magic pathspecs, and they are kept OUT of the plain lists
# because the two contexts that consume them need opposite quoting. A Make
# recipe is a /bin/sh command string, where bare parentheses are a syntax error
# — there the pathspec must be single-quoted (the *_SH variants). The
# path-printing targets feed scripts that build an argv array, where quote
# characters would reach git literally — they get the bare form. Every consumer
# therefore appends the spec it needs to the plain list.
L10N_SIDECAR_SPEC       := :(glob)harness/demos/*/demo.*.yaml
L10N_SIDECAR_SPEC_SH    := ':(glob)harness/demos/*/demo.*.yaml'
# `*` in a git glob pathspec does not cross a `/`, so this is the English
# renders at the top of the directory and none of the per-locale trees below it.
L10N_MAIL_RENDER_SPEC    := :(glob)bowrain/mailer/templates/*.html
L10N_MAIL_RENDER_SPEC_SH := ':(glob)bowrain/mailer/templates/*.html'

L10N_DERIVED := \
	core/i18n/builtins/metadata.json \
	host/i18n/commands.json \
	core/i18n/catalogs/qps.json \
	bowrain/mailer/subjects/qps.json \
	$(KAPI_DESKTOP_FRONTEND)/public/translations/qps.json \
	bowrain/apps/web/public/translations/qps.json \
	bowrain/apps/bowrain/frontend/public/translations/qps.json \
	bowrain/apps/ctrl/public/translations/qps.json \
	bowrain/apps/pulse/public/translations/qps.json \
	$(LANDING_DIR)/translations/qps.json \
	$(LANDING_DIR)/head/qps.json \
	bowrain/mailer/templates/qps

# One locale's committed loop output. The Go catalog directories contribute
# their <lang>.json — the compiled <lang>.mo beside them is gitignored build
# output, which every untracked scan here skips because it honours .gitignore.
L10N_LOOP_LOCALE = core/i18n/catalogs/$(1).json host/i18n/catalogs/$(1).json \
	bowrain/mailer/subjects/$(1).json \
	$(KAPI_DESKTOP_FRONTEND)/public/translations/$(1).json \
	bowrain/apps/web/public/translations/$(1).json \
	bowrain/apps/bowrain/frontend/public/translations/$(1).json \
	bowrain/apps/ctrl/public/translations/$(1).json \
	bowrain/apps/pulse/public/translations/$(1).json \
	$(LANDING_DIR)/translations/$(1).json \
	bowrain/mailer/templates/$(1)

L10N_LOOP_CATALOGS := $(foreach lang,$(L10N_LANGS),$(call L10N_LOOP_LOCALE,$(lang)))

# Stage 1: extract.
# The source side only. Deliberately never the target side: a push and a build
# both need the source catalogs, and target drift must never gate either.

kapi-desktop-extract: kapi-desktop-frontend-deps i18n-react-build ## Extract Kapi Desktop UI strings → i18n/ (per-file .kbf.json)
	cd $(KAPI_DESKTOP_FRONTEND) && $(NEOKAPI_I18N_CLI) extract $(KAPI_DESKTOP_EXTRACT_FLAGS)

bowrain-app-extract: i18n-react-build ## Extract bowrain app+ui+shell strings → bowrain/packages/app/i18n/
	cd $(BOWRAIN_APP_DIR) && $(NEOKAPI_I18N_CLI) extract $(BOWRAIN_APP_EXTRACT_FLAGS)

bowrain-ctrl-extract: i18n-react-build ## Extract ctrl admin-app strings → bowrain/apps/ctrl/i18n/
	cd bowrain/apps/ctrl && $(NEOKAPI_I18N_CLI) extract $(BOWRAIN_SHELL_EXTRACT_FLAGS)

bowrain-pulse-extract: i18n-react-build ## Extract pulse dashboard strings → bowrain/apps/pulse/i18n/
	cd bowrain/apps/pulse && $(NEOKAPI_I18N_CLI) extract $(BOWRAIN_SHELL_EXTRACT_FLAGS)

emails-frontend-deps: ## Install transactional-email template dependencies
	cd $(EMAILS_DIR) && vp install

emails-extract: emails-frontend-deps i18n-react-build ## Extract transactional-email strings → bowrain/emails/i18n/
	cd $(EMAILS_DIR) && $(NEOKAPI_I18N_CLI) extract $(EMAILS_EXTRACT_FLAGS)

landing-frontend-deps: ## Install landing page dependencies
	cd $(LANDING_DIR) && vp install

landing-extract: landing-frontend-deps i18n-react-build ## Extract landing page strings → bowrain/web/landing/i18n/
	cd $(LANDING_DIR) && $(NEOKAPI_I18N_CLI) extract $(LANDING_EXTRACT_FLAGS)

kapi-i18n-generate: i18n-catalogs ## Regenerate core/i18n/builtins/metadata.json from the Go registries
	go generate ./core/i18n/...

kapi-cli-i18n-generate: i18n-catalogs ## Regenerate host/i18n/commands.json from the cobra command tree
	cd cli && go generate ./i18ngen/...

l10n-extract: kapi-desktop-extract bowrain-app-extract bowrain-ctrl-extract bowrain-pulse-extract emails-extract landing-extract kapi-i18n-generate kapi-cli-i18n-generate ## Stage 1: every SOURCE catalog the recipe declares (no target languages)
	@echo "✓ source catalogs extracted — every collection kapi.yaml declares now has content"

# Every extract surface as "<dir><TAB><flags>", one per line: the fixture guard
# reads this rather than re-deriving globs, so the thing it checks is the thing
# stage 1 runs. Single-quoted because the flag strings carry double quotes.
l10n-extract-globs: ## Print each extract surface as "<dir><TAB><extract flags>"
	@printf '%s\t%s\n' '$(KAPI_DESKTOP_FRONTEND)' '$(KAPI_DESKTOP_EXTRACT_FLAGS)'
	@printf '%s\t%s\n' '$(BOWRAIN_APP_DIR)' '$(BOWRAIN_APP_EXTRACT_FLAGS)'
	@printf '%s\t%s\n' 'bowrain/apps/ctrl' '$(BOWRAIN_SHELL_EXTRACT_FLAGS)'
	@printf '%s\t%s\n' 'bowrain/apps/pulse' '$(BOWRAIN_SHELL_EXTRACT_FLAGS)'
	@printf '%s\t%s\n' '$(EMAILS_DIR)' '$(EMAILS_EXTRACT_FLAGS)'
	@printf '%s\t%s\n' '$(LANDING_DIR)' '$(LANDING_EXTRACT_FLAGS)'

check-extract-fixtures: ## Guard: no test/story file is extracted, and each surface's make and package invocations agree
	@node scripts/check-extract-fixtures.mjs

check-vocab-packs: ## Guard: the vocabulary packs have exactly two homes (Go embed + one TS copy) and they agree
	@node scripts/format-ops/check-vocab-packs.mjs

l10n-review-export: bin/kapi ## Emit disposable TMX/CSV review views of the project store → l10n/review/
	@mkdir -p l10n/review
	./bin/kapi memory export --format tmx -o l10n/review/tm-all.tmx
	./bin/kapi terms export --format csv -s en -t nb -o l10n/review/terms-en-nb.csv
	@echo "Review views written to l10n/review/ (gitignored, read-only: wording is decided in the ledger, not in an exported view)"

# Stage 2: converge.
# One `kapi up` over the WHOLE recipe. The loop resolves the project's content
# patterns and writes each item's output from its own `target:` template, so
# every collection is covered by construction — add a collection to kapi.yaml
# and it is converged with no Makefile change.
#
# The recipe binds `flow: tm-recycle` — exact-match content-memory leverage and
# nothing else: no AI, no provider credentials, no network. So this stage is the
# committed context plus whatever the store already learned, and a fresh clone
# converges from git alone. AI convergence is the server venue's (the nightly),
# or a deliberate local `kapi run translate-ai`.
#
# The store is never wiped. `up` compiles the committed sources by content
# digest, so an unchanged bundle costs a read; and wording a venue pull brought
# home lives in the store beside what git carries. A wipe would delete exactly
# the half git does not hold.

l10n-converge: l10n-extract bin/kapi ## Stage 2: the whole recipe, one `kapi up`
	$(KAPI_LOOP_ENV) ./bin/kapi up
	@# A narration sidecar byte-identical to its source carries nothing — the
	@# harness already falls back to English. Dropping it keeps the committed
	@# set equal to the demos that actually have reviewed narration, with no
	@# allowlist for a human to remember to extend.
	@for f in harness/demos/*/demo.*.yaml; do \
		[ -e "$$f" ] || continue; \
		if cmp -s "$$f" "$${f%/*}/demo.yaml"; then rm -f "$$f"; fi; \
	done

# The qps pseudo-locale is a separate, isolated pass: it is a runtime-correctness
# probe (does the UI survive expanded, marked-up text?), not project content, so
# it is not a target language in the recipe and does not bind the project. It
# expands the extracted source catalogs mechanically, which is why its output is
# byte-gated where the loop's is not.

l10n-pseudo: bin/kapi ## Pseudo-translate every surface into the qps probe locale
	@for dir in $(L10N_KBF_PROBE_DIRS); do \
		$(KAPI_ISO_ENV) ./bin/kapi pseudo-translate $$dir/i18n --target-lang qps -o $$dir/i18n-qps -q || exit 1; \
	done
	@# -p binds the recipe, where `defaults.locales.qps` turns the markers off
	@# for the published pseudo builds. An explicit -p outranks the
	@# KAPI_NO_PROJECT in $(KAPI_ISO_ENV), so config, plugins and caches stay
	@# isolated while the recipe still supplies the tool settings.
	$(KAPI_ISO_ENV) ./bin/kapi pseudo-translate $(LANDING_DIR)/i18n --target-lang qps -p $(CURDIR)/kapi.yaml -o $(LANDING_DIR)/i18n-qps -q
	@# The shell's <head> is prose too: the browser tab and the social card.
	@# It stayed English in every locale because locale-meta.json only carried
	@# head strings if a person typed them, and typing a translation by hand is
	@# what this loop exists to avoid. kapi reads index.html like any other
	@# content, so the head comes from the same pass as the body.
	$(KAPI_ISO_ENV) ./bin/kapi pseudo-translate $(LANDING_DIR)/index.html --target-lang qps -p $(CURDIR)/kapi.yaml -o $(LANDING_DIR)/.head-qps.html -q
	@node scripts/landing-head.mjs $(LANDING_DIR)/.head-qps.html $(LANDING_DIR)/head/qps.json
	@rm -f $(LANDING_DIR)/.head-qps.html
	$(KAPI_ISO_ENV) ./bin/kapi pseudo-translate core/i18n/builtins/metadata.json --target-lang qps -f json -o core/i18n/catalogs/qps.json -q
	$(KAPI_ISO_ENV) ./bin/kapi pseudo-translate bowrain/mailer/subjects/en.json --target-lang qps -f json -o bowrain/mailer/subjects/qps.json -q
	@# The Docusaurus sites, for a LOCAL preview only. Their qps trees are
	@# gitignored and regenerated by .github/actions/pseudo-locale immediately
	@# before each site build, so nothing here is committed and there is no byte
	@# gate over it. Content tier only: --with-theme needs `docusaurus
	@# write-translations`, which needs each site's own node_modules.
	@./scripts/pseudo-docs-i18n.sh bowrain/web/docs
	@./scripts/pseudo-docs-i18n.sh web

# Stage 3: compile.
# A catalog is not what a product loads. The SPAs and the landing page load
# compiled runtime dictionaries and the transactional emails are rendered to
# per-locale HTML the server embeds — neokapi-i18n's job; the Go binaries load
# gettext MO compiled from the committed catalog JSON — the i18n-catalogs
# target's job. Both read a catalog this pipeline wrote and neither is the
# catalog itself.

# Which target locales this stage compiles. `make l10n` compiles what the loop
# has just materialized; `make l10n-build` compiles none, because a walk that
# regenerates the build tier must not rewrite an artifact the loop owns — a
# developer's tree still carries the previous run's `i18n-<lang>/`, and
# compiling from it would put loop output in the diff of a gate that has nothing
# to say about it.
L10N_COMPILE_LANGS ?= $(L10N_LANGS)

# Each locale is compiled where its catalogs are on disk, and skipped where they
# are not: a fresh checkout has no target-locale catalogs at all, and the probe
# exists only after l10n-pseudo has run.
l10n-compile: i18n-react-build ## Stage 3: catalogs → runtime dictionaries, embedded MO, rendered email templates
	@for spec in $(L10N_COMPILE_TARGETS); do \
		dir=$${spec%%:*}; out=$${spec#*:}; \
		if [ -d "$$dir/i18n-qps" ]; then \
			(cd $$dir && $(NEOKAPI_I18N_CLI) compile i18n-qps/ --out $$out) || exit 1; \
		fi; \
		for lang in $(L10N_COMPILE_LANGS); do \
			if [ -d "$$dir/i18n-$$lang" ]; then \
				(cd $$dir && $(NEOKAPI_I18N_CLI) compile i18n-$$lang/ --out $$out --locale $$lang) || exit 1; \
			fi; \
		done; \
	done
	cd $(EMAILS_DIR) && vp run build
	@# Recursive rather than a prerequisite: the Go catalogs must compile from
	@# the JSON on disk now, and a sibling prerequisite carries no ordering
	@# under `make -j`.
	@$(MAKE) --no-print-directory i18n-catalogs

landing-build-nb:## Build the nb landing variant → bowrain/web/landing/dist/nb (inline mode from the committed catalogs)
	cd $(LANDING_DIR) && vp run build:nb

# The two walks, the gate, and the reports.

# The stages are recursive submake calls rather than a prerequisite chain,
# because the order is the point and a prerequisite carries none under
# `make -j`.
#
# The collapse guard closes the loop on itself. Everything downstream treats
# what the walk wrote as the truth — the nightly delivers it, a human commits it
# — so a convergence that produced no target catalogs at all writes `{}`
# everywhere and every check agrees. Asserting it here, in the walk that
# produced the files, is the one place that can tell the difference.
l10n: l10n-converge ## Bring every multilingual surface up to date (extract → kapi up → compile)
	@$(MAKE) --no-print-directory l10n-pseudo
	@$(MAKE) --no-print-directory l10n-compile
	@$(MAKE) --no-print-directory l10n-collapse-check

# The build tier on its own — no loop, so it cannot rewrite a target-language
# artifact. This is what l10n-verify regenerates and what the l10n workflow
# rebuilds on a pull request: extraction, the qps probe, and the compilations
# that read neither the project store nor a target locale's catalogs.
l10n-build: l10n-extract ## Regenerate the build-derived tier only (no convergence)
	@$(MAKE) --no-print-directory l10n-pseudo
	@$(MAKE) --no-print-directory l10n-compile L10N_COMPILE_LANGS=

# Not a coverage bar: a catalog with fewer entries than yesterday passes. Only a
# catalog that carried entries at HEAD and regenerated to none fails, and only
# while the committed content memory still holds pairs for its locale. It reads
# the loop-owned set, because that is where a target-locale catalog is.
l10n-collapse-check: ## Guard: no committed target-locale catalog regenerated to empty
	@node scripts/check-catalog-collapse.mjs $(L10N_LANGS) -- $(L10N_LOOP_CATALOGS)

# Three path sets, one definition each, so no consumer re-derives a list.
#
#   l10n-derived-paths     the byte gate — l10n-verify and scripts/l10n-autofix.sh.
#                          Source-deterministic artifacts only.
#   l10n-loop-owned-paths  the target tier on its own: the answer to "what may
#                          this run change with no source change behind it",
#                          which is what a reviewer reading a convergence diff
#                          needs and what the gate below classifies as derived.
#   l10n-owned-paths       both — scripts/check-sync-backed.sh (which classifies
#                          a convergence run's tree) and the nightly's delivery
#                          step. A convergence run may legitimately leave either
#                          tier behind; anything else it touched is foreign.

l10n-derived-paths: ## Print the git pathspecs l10n-verify byte-gates
	@echo "$(L10N_DERIVED) $(L10N_MAIL_RENDER_SPEC)"

l10n-loop-owned-paths: ## Print the git pathspecs the loop owns (target tier, gated on content rather than bytes)
	@echo "$(L10N_LOOP_CATALOGS) $(L10N_SIDECAR_SPEC)"

l10n-owned-paths: ## Print every committed artifact this pipeline owns (both tiers)
	@echo "$(L10N_DERIVED) $(L10N_MAIL_RENDER_SPEC) $(L10N_LOOP_CATALOGS) $(L10N_SIDECAR_SPEC)"

# The byte gate. It asserts generated-vs-source consistency over the tier that is
# a function of committed source alone: the generated inventories, the English
# email renders, and the qps probe. It says nothing about translation coverage,
# and cannot — the target tier is not in its set.
#
# A new untracked file under the gated paths counts as drift too: `git diff`
# alone would miss an artifact for a surface that did not exist before.
l10n-verify: l10n-build ## CI gate: every build-derived artifact regenerates byte-identically
	@# A magic pathspec may match zero tracked files; git treats an unmatched
	@# pathspec as fatal, so diff only what exists.
	@tracked="$$(git ls-files -- $(L10N_DERIVED) $(L10N_MAIL_RENDER_SPEC_SH))"; \
	if [ -n "$$tracked" ]; then git diff --exit-code -- $$tracked; fi
	@untracked="$$(git ls-files --others --exclude-standard -- $(L10N_DERIVED) $(L10N_MAIL_RENDER_SPEC_SH))"; \
	if [ -n "$$untracked" ]; then \
		echo "l10n-verify: regeneration produced untracked files (commit them):"; \
		echo "$$untracked" | sed 's/^/  /'; \
		exit 1; \
	fi
	@echo "l10n-verify: every build-derived artifact matches its source"

# What replaced the byte gate on the target tier: coverage and placeholder
# parity per locale, reported and never gated. Coverage below a bar is pending
# work; a placeholder mismatch is a defect a human reads, not a build break the
# next source edit inherits.
#
# <target artifact>:<the document it is measured against>. The reference is the
# qps probe where a surface has one — it expands every source string and keeps
# its placeholders — and the source document otherwise.
L10N_REPORT_PAIRS = \
	core/i18n/catalogs/$(1).json:core/i18n/builtins/metadata.json \
	host/i18n/catalogs/$(1).json:host/i18n/commands.json \
	bowrain/mailer/subjects/$(1).json:bowrain/mailer/subjects/en.json \
	$(KAPI_DESKTOP_FRONTEND)/public/translations/$(1).json:$(KAPI_DESKTOP_FRONTEND)/public/translations/qps.json \
	bowrain/apps/web/public/translations/$(1).json:bowrain/apps/web/public/translations/qps.json \
	bowrain/apps/bowrain/frontend/public/translations/$(1).json:bowrain/apps/bowrain/frontend/public/translations/qps.json \
	bowrain/apps/ctrl/public/translations/$(1).json:bowrain/apps/ctrl/public/translations/qps.json \
	bowrain/apps/pulse/public/translations/$(1).json:bowrain/apps/pulse/public/translations/qps.json \
	$(LANDING_DIR)/translations/$(1).json:$(LANDING_DIR)/translations/qps.json

l10n-report: ## Report per-locale coverage and placeholder parity over the loop-owned tier (never gates)
	@$(foreach lang,$(L10N_LANGS),node scripts/l10n-loop-report.mjs $(lang) $(call L10N_REPORT_PAIRS,$(lang));)

# What a derived artifact derives from, printed one locale per line, so the
# return-leg gate and this walk cannot disagree about it. The same pairs the
# report is measured against: a coverage number and a soundness verdict are two
# readings of one relation.
l10n-content-pairs: ## Print <artifact>:<reference> for every derived artifact whose content can be read
	@$(foreach lang,$(L10N_LANGS),echo "$(lang) $(call L10N_REPORT_PAIRS,$(lang))";)

# The content question over the whole committed tier — the standing burndown of
# what is already in git. `scripts/check-sync-backed.sh` asks it of what a run
# wrote, which is where it gates; here it is a reading, run when someone wants
# the list. Not in `make l10n`: content soundness gates the return leg, never an
# ordinary build.
l10n-content-check: ## Read every committed derived artifact for parse, placeholder parity and machine identifiers
	@$(foreach lang,$(L10N_LANGS),node scripts/check-derived-content.mjs $(lang) $(call L10N_REPORT_PAIRS,$(lang));)

# Everything the loop materializes for one locale — the corpus the orphan report
# asks "did this seed entry produce anything?" against. Not the same set as
# L10N_LOOP_CATALOGS: that one names what is *committed*, and the docs pages and
# per-surface catalogs here are generated build artifacts on purpose.
L10N_TARGETS = $(foreach d,$(L10N_KBF_DIRS),$(d)/i18n-$(1)) \
	core/i18n/catalogs/$(1).json host/i18n/catalogs/$(1).json \
	bowrain/mailer/subjects/$(1).json bowrain/mailer/templates/$(1) \
	web/i18n/$(1) bowrain/web/docs/i18n/$(1) \
	$(wildcard harness/demos/*/demo.$(1).yaml)

# Standing, never gating — the committed seeds are read-only accelerants, and an
# entry whose source string is gone is kept rather than deleted. Content memory
# matches on text, though, so a kept entry is wording any surface can pick up
# again; the point of the report is that a reviewer sees which ones those are.
l10n-orphans: l10n l10n-orphans-report l10n-stale-report ## Run the loop, then report entries the source has moved away from

# The report over already-materialized targets. Split out because CI has just
# run the stages and re-running them to read their output would double the job.
l10n-orphans-report: ## Report seed entries that produced no target artifact (never gates)
	@$(foreach lang,$(L10N_LANGS),node scripts/l10n-orphan-report.mjs $(lang) $(call L10N_TARGETS,$(lang));)

# The mirror image of the orphan report, and the reason both exist. A KBF
# catalog keys on the source text, so a rewrite orphans the entry and the locale
# falls back — visible above. A scope-addressed catalog keys on the key path, so
# a rewrite leaves the old translation attached to the new sentence and nothing
# falls back. That is a wrong translation rather than a missing one, and the
# wording it was produced from lives in git.
l10n-stale-report: ## Report target entries whose source string changed under them (never gates)
	@$(foreach lang,$(L10N_LANGS),node scripts/l10n-stale-target-report.mjs $(lang) $(call L10N_REPORT_PAIRS,$(lang));)

# ── Frontend packages ────────────────────────────────────────────────────────

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

install: i18n-catalogs ## Install kapi CLI to GOPATH/bin
	cd kapi && $(GO) install $(LDFLAGS) ./cmd/kapi

# ── Coverage ─────────────────────────────────────────────────────────────────

cover: i18n-catalogs ## Run tests with coverage (merged report)
	@mkdir -p $(COVER_DIR)
	$(GOTEST_BASE) -coverprofile=$(COVER_DIR)/framework.out $(_COVMODE) ./... -count=1
	cd host && $(GOTEST_BASE) -coverprofile=../$(COVER_DIR)/host.out $(_COVMODE) ./... -count=1
	cd cli && $(GOTEST_BASE) -coverprofile=../$(COVER_DIR)/cli.out $(_COVMODE) ./... -count=1
	cd kapi && $(GOTEST_BASE) -coverprofile=../$(COVER_DIR)/kapi.out $(_COVMODE) ./... -count=1
	@$(MAKE) -C bowrain cover
	cat $(COVER_DIR)/framework.out > $(COVER_DIR)/coverage.out
	tail -n +2 $(COVER_DIR)/host.out >> $(COVER_DIR)/coverage.out
	tail -n +2 $(COVER_DIR)/cli.out >> $(COVER_DIR)/coverage.out
	tail -n +2 $(COVER_DIR)/kapi.out >> $(COVER_DIR)/coverage.out
	tail -n +2 $(COVER_DIR)/platform.out >> $(COVER_DIR)/coverage.out
	tail -n +2 $(COVER_DIR)/bowrain-plugin.out >> $(COVER_DIR)/coverage.out
	tail -n +2 $(COVER_DIR)/bowrain.out >> $(COVER_DIR)/coverage.out
	$(GO) tool cover -html=$(COVER_DIR)/coverage.out -o $(COVER_DIR)/coverage.html
	@echo "Coverage report: $(COVER_DIR)/coverage.html"

# ── E2E Tests ────────────────────────────────────────────────────────────────

# The kapi suite is tagged `e2e` and lives in ./kapi/e2e — both are load-bearing.
# These targets named ./e2e/... (a path that has not existed since the module
# reorg) and dropped the tag, so they selected nothing; `test-e2e` additionally
# swallowed the error, which is why `make test-e2e` "passed" while the nightly
# was red. Keep the tag and the path together.
test-e2e: ## Run all end-to-end tests
	$(MAKE) test-e2e-kapi
	$(MAKE) -C bowrain test-e2e-bowrain

# Not $(GOTEST): that bakes in $(GOTAGS), and a second -tags would silently
# shadow it rather than add to it.
test-e2e-kapi: i18n-catalogs ## Run kapi e2e tests
	$(GO) test -tags "fts5,e2e" ./kapi/e2e/... -count=1 -v

test-e2e-bowrain: ; $(MAKE) -C bowrain $@
test-e2e-cloud: ; $(MAKE) -C bowrain $@ ## Run cloud e2e tests against a live server
test-e2e-dev: ; $(MAKE) -C bowrain $@ ## Run cloud e2e tests against dev environment

# ── Face parity ─────────────────────────────────────────────────────────────
#
# One question, three faces: the CLI verb, the MCP tool or resource, and the
# desktop's backend method. The suites cannot share a binary — the desktop is
# its own module and links Wails — so they meet at a committed record of
# answers (host/facetest). Each leg builds the same fixture, asks its own face,
# and compares.
#
# Not $(GOTEST) with a second -tags: `go` takes the last -tags flag rather than
# unioning them, so every tag is spelled once, here.
face-parity: i18n-catalogs ## Run the CLI/MCP/desktop face parity suite
	$(GO) test -tags "fts5" ./host/ ./cli/ -run TestFaceParity -count=1
	cd $(KAPI_DESKTOP_DIR) && $(GO) test -tags "fts5" ./backend/ -run TestFaceParity -count=1

face-parity-update: i18n-catalogs ## Rewrite the face parity record from the host layer
	$(GO) test -tags "fts5" ./host/ -run TestFaceParity -count=1 -update-face-golden
	@echo "re-run 'make face-parity' and review host/facetest/testdata/answers.json before committing"

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

# ── Conversion eval ──────────────────────────────────────────────────────────
# Compares document converters on how much of a document's text each one keeps.
#
# Ground truth comes from the documents rather than from any converter: OOXML
# designates which elements carry text, so each file states its own contents.
# The corpus is the okapi-testdata tree the parity harness already downloads —
# real documents collected by another project for another purpose.
#
# Free and offline. It needs whichever converters are installed; each one that
# is absent is left out of the report rather than scored zero.
CONVERSIONEVAL_LIMIT ?= 0

conversion-eval: build ## Compare converters on text-extraction completeness → /conversion-eval
	$(GO) run ./scripts/conversioneval -limit $(CONVERSIONEVAL_LIMIT) -jobs 6
	@echo "Published conversion-eval report → web/src/pages/conversion-eval/_conversioneval.json"

# ── Authoring eval ───────────────────────────────────────────────────────────
# Measures the authoring side: the voice checks, voice-infer, and whether
# `kapi voice guide` steers writing toward its profile rather than just
# improving it.
#
# The corpus is synthesized and says so in the data. Two of the three questions
# need ground truth no repository carries — a profile a person wrote from a
# known corpus, and prose whose every violation is marked — and labelling real
# material to that standard is the eval rather than preparation for it.
#
# The checks leg is free and offline. The steering leg writes six documents
# twice with a real model, so it costs calls.
AUTHORINGEVAL_PROVIDER ?= claude-code
AUTHORINGEVAL_MODEL    ?= sonnet

# One provider across all three legs. Each leg writes its own section of the same
# dataset, so mixing providers publishes a page whose rows were measured under
# different conditions and says so nowhere.
authoring-eval: build ## Score the voice checks and the voice guide → /authoring-eval (costs calls)
	$(GO) run ./scripts/authoringeval -only checks \
	    -provider $(AUTHORINGEVAL_PROVIDER) -model $(AUTHORINGEVAL_MODEL)
	$(GO) run ./scripts/authoringeval -only infer \
	    -provider $(AUTHORINGEVAL_PROVIDER) -model $(AUTHORINGEVAL_MODEL)
	$(GO) run ./scripts/authoringeval -only steer \
	    -provider $(AUTHORINGEVAL_PROVIDER) -model $(AUTHORINGEVAL_MODEL)
	@echo "Published authoring-eval report → web/src/pages/authoring-eval/_authoringeval.json"

# The read-it-yourself half. Writes the same document three ways at two
# coordinates, across four models, and publishes the prose rather than a score:
# 24 Markdown documents under web/static/authoring-lab and a page that puts the
# arms side by side.
#
# The three arms differ only in how the governance arrives. Bare has none.
# Pushed has the guide in its system prompt. Pulled has the kapi skill and a
# project that binds the voice, nothing in its prompt, and has to go and ask —
# which is the arm that says whether the loop closes, and needs bin/kapi, hence
# the build prerequisite.
#
# It scores nothing on purpose. Nobody has yet said what a good user guide is,
# and a rubric invented here would be measuring the rubric.
AUTHORINGLAB_ARGS ?=
authoring-lab: build ## Write the same document three ways at two coordinates, across four models (spends)
	$(GO) run ./scripts/authoringlab $(AUTHORINGLAB_ARGS)

authoring-eval-checks: build ## Score the offline voice check only (free, no model)
	$(GO) run ./scripts/authoringeval -only checks -provider none -model ""

# ── Batch-size eval (issue #1227) ────────────────────────────────────────────
# Measures what batching costs, by sweeping blocks-per-call and scoring each N on
# structural integrity: does every segment come back, under the id it was sent,
# with its placeholders and inline tags intact? That is the failure batching is
# documented to produce, and it needs no reference translations.
#
# tools.MaxBlocksPerCall was set from evidence about *adjacent* tasks — nobody had
# published a quality-versus-N curve for segment translation. This is the harness
# that measured our own, and it is re-run when the models move: a ceiling checked
# once decays into folklore.
#
# The default target runs the demo stub: it proves the harness and measures
# NOTHING about batching, and says so. A real curve costs real calls:
#
#   make batch-eval BATCHEVAL_ARGS="-provider anthropic -repeat 3"
# The /coordinate dashboard: what governs each point, which prior answers the
# gate offers, and the prompt each produces — side by side.
#
# Unlike its eval siblings this costs nothing and measures nothing stochastic.
# Resolution, the version chain and the governance gate are deterministic, so
# the page states facts rather than samples, and the companion test fails when
# the committed data stops matching the code.
coordinate-report: ## Regenerate the /coordinate dashboard data (no model calls)
	$(GO) run $(GOTAGS) ./scripts/coordinatereport
	@echo "Published coordinate report → web/src/pages/coordinate/_coordinate.json"

# ── The /evals cover page ────────────────────────────────────────────────────
# The registry of every eval and the question it answers, including the ones
# nothing answers yet. A registry rather than a runner: each eval keeps its own
# harness and its own cadence, and the cheap ones would run rarely (or the
# expensive ones constantly) if they shared a command. The companion test fails
# when the committed index stops matching the cards, and when a card names a
# command, page or dataset that does not exist.
eval-index: ## Rebuild the /evals cover-page data
	$(GO) run ./scripts/evalindex

# ── Agent-skill eval ─────────────────────────────────────────────────────────
# Measures the shipped Agent Skill by driving a real agent through `claude -p`
# in a throwaway workspace per scenario.
#
# NEVER runs in CI. It needs the claude CLI, local credentials and real money,
# so the committed dataset is the only thing a build ever sees — which makes the
# date on that dataset the real currency of the numbers, and the dashboard shows
# its age for exactly that reason.
#
# Triggering is stochastic: a single pass tells you almost nothing, so the
# default is three and a scenario that fires twice in three is reported as
# `flaky` rather than rounded to a pass.
SKILLEVAL_ARGS ?=
skill-eval: ## Measure whether the Agent Skill fires on the right tasks (spends, local only)
	$(GO) run ./scripts/skilleval -mode trigger $(SKILLEVAL_ARGS)

# The expensive half: drives each positive to a green gate rather than stopping
# at activation. Needs a built kapi, and takes far longer per scenario.
skill-eval-completion: build ## Drive each positive scenario to a green gate (slow, spends, local only)
	$(GO) run ./scripts/skilleval -mode completion -repeat 1 -concurrency 3 -timeout 90m $(SKILLEVAL_ARGS)

# The other door. An MCP client holds kapi's nineteen tools in context already,
# so it cannot fail to notice kapi; it fails by picking the wrong tool. Scored
# on which tool the agent reached for, not on whether it reached kapi at all.
# Needs a built kapi, because the agent is pointed at this checkout's binary.
mcp-eval: build ## Measure whether an agent picks the right kapi MCP tool (spends, local only)
	$(GO) run ./scripts/skilleval -mode trigger -surface mcp -repeat 3 $(SKILLEVAL_ARGS)

PRIORAB_ARGS ?=
# Costs model calls. Two halves: a deterministic consistency check (does the
# approved wording survive) and a judged quality score. Only the first should be
# read as evidence until the judge's agreement with a person is measured.
prior-ab-eval: ## Measure whether a block's prior version changes what the model writes (spends)
	$(GO) run $(GOTAGS) ./scripts/priorabeval $(PRIORAB_ARGS)

BATCHEVAL_ARGS ?=
batch-eval: ## Sweep batch size and score structural integrity (demo stub unless -models given)
	$(GO) run ./scripts/batcheval $(BATCHEVAL_ARGS)

# The published sweep behind the /batch-eval dashboard. Re-run it when the models
# move: an alias like `sonnet` or `gemini-3.5-flash` points at different weights
# over time, so a ceiling measured once and never re-checked decays into folklore.
# Same-day re-runs correct their entry rather than appending a second point.
#
# claude-code runs on the local Claude subscription (no API key); gemini needs
# GEMINI_API_KEY.
BATCHEVAL_DATA   ?= web/src/pages/batch-eval/_batcheval.json
# The corpus has to be bigger than the largest batch swept, or the sweep saturates
# and reports the *corpus's* ceiling as the models'. A 30-block corpus swept to N=32
# is not testing a batch of 32; it is testing the whole document in one call, which
# is why the first curve came out flat at 100%.
BATCHEVAL_BLOCKS ?= 600
BATCHEVAL_N      ?= 8,16,32,64,128,256,600
# Current models only. Tracking a model that is being retired measures a curve
# nobody can act on, and the retirement itself is the noisiest kind of false
# finding — gemini-3-pro-preview already answers 404 ("no longer available").
BATCHEVAL_GEMINI ?= gemini:gemini-3.5-flash,gemini:gemini-3.1-flash-lite,gemini:gemini-3.1-pro-preview
BATCHEVAL_CLAUDE ?= claude-code:opus,claude-code:sonnet,claude-code:haiku
# The route the Bowrain platform actually runs on, so the one whose numbers describe
# production rather than an alternative. Needs credentials for the account that can
# call the inference profile: `aws sso login --profile bowrain-prod`.
BATCHEVAL_BEDROCK ?= bedrock:eu.anthropic.claude-sonnet-4-6
# The model catalog (providers/ai/models.json) carries lifecycle facts the vendors'
# APIs do not — introduced, superseded, retired — so it is curated, and curation
# rots. check-models is the alarm: it lists what each provider serves today and
# reports any catalogued model that is gone (should be marked retired) and any live
# model the catalog omits (a candidate). The keyless half — every /batch-eval price
# must be for a catalogued model — is also a unit test, so it runs in `make test`.
#
#   make check-models                          # catalog vs live provider lists (needs keys)
#   make check-models MODELCHECK_ARGS=-candidates   # also list live models not catalogued
#   make update-model-prices                   # refresh prices.json from the vendors' pages
#   (refresh the catalog itself: scripts/prompts/update-model-catalog.md)
check-models: ## Report catalogued models a provider no longer serves (and, with -candidates, new ones)
	$(GO) run ./scripts/modelcheck $(MODELCHECK_ARGS)

# Prices are not exposed over any API, so refreshing them means reading the vendors'
# pricing pages. That is a job for an agent with a browser, driven by a prompt that
# spells out which of the many published rates is the right one.
update-model-prices: ## Refresh scripts/batcheval/prices.json from the vendors' pricing pages
	@command -v claude >/dev/null || { echo "needs the claude CLI: brew install claude"; exit 1; }
	claude -p "$$(cat scripts/prompts/update-model-prices.md)"
# batcheval's table is canonical; contexteval embeds a copy, byte-identical
# (TestPricesMatchBatcheval gates the drift).
	cp scripts/batcheval/prices.json scripts/contexteval/prices.json

# The catalog carries lifecycle facts no API exposes, so refreshing it is a curation
# job: reconcile check-models against the live lists, retire what is gone, adopt what
# is new. Driven by an agent with the providers' model cards.
update-model-catalog: ## Refresh providers/ai/models.json against the providers' live model lists
	@command -v claude >/dev/null || { echo "needs the claude CLI: brew install claude"; exit 1; }
	claude -p "$$(cat scripts/prompts/update-model-catalog.md)"

batch-eval-publish: ## Sweep the real models → /batch-eval dashboard data (costs calls)
	$(GO) run ./scripts/batcheval -models $(BATCHEVAL_GEMINI) -blocks $(BATCHEVAL_BLOCKS) \
		-n $(BATCHEVAL_N) -repeat 2 -concurrency 4 -append $(BATCHEVAL_DATA)
	$(GO) run ./scripts/batcheval -models $(BATCHEVAL_CLAUDE) -blocks $(BATCHEVAL_BLOCKS) \
		-n $(BATCHEVAL_N) -repeat 1 -concurrency 3 -append $(BATCHEVAL_DATA)
# Bedrock rate-limits on *requests*, so the small batch sizes — which issue the most
# calls — are the ones that get throttled. Low concurrency, and a throttled N is
# recorded as unmeasured rather than as a failure the model did not have.
# The Bedrock leg needs AWS credentials (`aws sso login --profile bowrain-prod`),
# which CI does not hold — the evals-refresh workflow sets EVALS_SKIP_BEDROCK=1
# and the leg stays a desktop responsibility.
ifeq ($(strip $(EVALS_SKIP_BEDROCK)),)
	$(GO) run ./scripts/batcheval -models $(BATCHEVAL_BEDROCK) -blocks $(BATCHEVAL_BLOCKS) \
		-n $(BATCHEVAL_N) -repeat 1 -concurrency 2 -append $(BATCHEVAL_DATA)
else
	@echo "EVALS_SKIP_BEDROCK set — skipping the Bedrock batch-eval leg (no AWS credentials)"
endif
	@echo "Published batch-eval history → $(BATCHEVAL_DATA)"

# ── Context-adherence eval ───────────────────────────────────────────────────
# The batch eval's sibling: measures whether a model *follows the context kapi
# injects* — terms, brand voice, instruction — as a differential (adherence
# with context minus adherence without = lift), scored per dimension with the
# framework's own check tools over an engineered trap corpus. The headline is
# lift per 1,000 context tokens: is the brand guide earning what it costs to
# send on this model? Design note: strategy/2026-07-model-evals.md.
#
# The default target runs the demo stub: it proves the harness and measures
# NOTHING about any model, and says so.
CONTEXTEVAL_ARGS ?=
context-eval: ## Measure context-adherence lift (demo stub unless -models given)
	$(GO) run ./scripts/contexteval $(CONTEXTEVAL_ARGS)

# The published sweep behind the /context-eval dashboard. Steerability is model-
# and time-specific — an alias like `sonnet` points at different weights over
# time — so re-run it when the models move. Same-day re-runs correct their entry.
#
# Judges are cross-family by construction (self-preference bias): the Claude and
# Bedrock sweeps are judged by Gemini, the Gemini sweep by Claude. Judged scores
# stay unpublished until `-judge-validate` records agreement above the bar.
CONTEXTEVAL_DATA    ?= web/src/pages/context-eval/_contexteval.json
CONTEXTEVAL_TARGETS ?= de,fr,en-GB,nb
CONTEXTEVAL_GEMINI  ?= gemini:gemini-3.5-flash,gemini:gemini-3.1-flash-lite,gemini:gemini-3.1-pro-preview
CONTEXTEVAL_CLAUDE  ?= claude-code:opus,claude-code:sonnet,claude-code:haiku
CONTEXTEVAL_BEDROCK ?= bedrock:eu.anthropic.claude-sonnet-4-6
CONTEXTEVAL_JUDGE_FOR_CLAUDE ?= gemini:gemini-3.5-flash
CONTEXTEVAL_JUDGE_FOR_GEMINI ?= claude-code:sonnet

# ── Judge validation ─────────────────────────────────────────────────────────
# A judged score cannot be trusted above the judge's measured agreement with a
# person, and until that measurement exists the dashboard withholds the judged
# dimension. The plumbing to USE labels has been here since the judge was
# written; what was missing is the part a person can do, because nobody
# hand-writes 150 items of JSON.
#
# Three steps, and the middle one is yours:
#
#   make judge-candidates   sweep and save every scored translation (costs calls)
#   make judge-label        answer y/n per criterion, resumable, ~20 min
#   make judge-validate     measure kappa and record it in the history
#
# The loop is blind on purpose: it never shows the judge's verdict, the model,
# or whether the translation came from the steered or the bare pass. Seeing any
# of them turns this into a measurement of agreement with a hint.
CONTEXTEVAL_CANDIDATES ?= scripts/contexteval/candidates.json
CONTEXTEVAL_LABELS     ?= scripts/contexteval/labels.json

judge-candidates: ## Sweep and save translations to label (costs calls)
	$(GO) run ./scripts/contexteval -models $(CONTEXTEVAL_CLAUDE) \
	    -targets $(CONTEXTEVAL_TARGETS) -repeat 1 -concurrency 3 \
	    -save-outputs $(CONTEXTEVAL_CANDIDATES)
	@echo "Candidates → $(CONTEXTEVAL_CANDIDATES). Now: make judge-label"

judge-label: ## Label saved translations for judge validation (interactive, free)
	@$(GO) run ./scripts/contexteval -label $(CONTEXTEVAL_CANDIDATES) -labels $(CONTEXTEVAL_LABELS)

judge-validate: ## Measure judge–human agreement over the labels and record it
	$(GO) run ./scripts/contexteval -judge $(CONTEXTEVAL_JUDGE_FOR_CLAUDE) \
	    -judge-validate $(CONTEXTEVAL_LABELS) -append $(CONTEXTEVAL_DATA)

context-eval-publish: ## Sweep real models for context adherence → /context-eval dashboard data (costs calls)
	$(GO) run ./scripts/contexteval -models $(CONTEXTEVAL_GEMINI) -targets $(CONTEXTEVAL_TARGETS) \
		-repeat 2 -concurrency 4 -judge $(CONTEXTEVAL_JUDGE_FOR_GEMINI) -append $(CONTEXTEVAL_DATA)
	$(GO) run ./scripts/contexteval -models $(CONTEXTEVAL_CLAUDE) -targets $(CONTEXTEVAL_TARGETS) \
		-repeat 1 -concurrency 3 -judge $(CONTEXTEVAL_JUDGE_FOR_CLAUDE) -append $(CONTEXTEVAL_DATA)
# Bedrock rate-limits on requests; low concurrency, and a throttled run is
# recorded as unmeasured rather than as 0% adherence. The leg needs AWS
# credentials CI does not hold — evals-refresh sets EVALS_SKIP_BEDROCK=1.
ifeq ($(strip $(EVALS_SKIP_BEDROCK)),)
	$(GO) run ./scripts/contexteval -models $(CONTEXTEVAL_BEDROCK) -targets $(CONTEXTEVAL_TARGETS) \
		-repeat 1 -concurrency 2 -judge $(CONTEXTEVAL_JUDGE_FOR_CLAUDE) -append $(CONTEXTEVAL_DATA)
else
	@echo "EVALS_SKIP_BEDROCK set — skipping the Bedrock context-eval leg (no AWS credentials)"
endif
	@echo "Published context-eval history → $(CONTEXTEVAL_DATA)"

# Measure judge–human agreement on the committed Norwegian seed set and record
# it (both cross-family judges). The judged voice dimension stays unpublished
# until a judge clears kappa >= 0.6 over >= 30 verdicts — reporting an
# unvalidated judge's opinion as a model's behaviour is how adherence evals rot.
# -dump prints the per-item disagreements so a low kappa can be inspected.
CONTEXTEVAL_LABELS ?= scripts/contexteval/evaldata/nb-labels.json
context-eval-validate: ## Measure judge–human agreement on the labeled seed set → dashboard gate
	$(GO) run ./scripts/contexteval -judge $(CONTEXTEVAL_JUDGE_FOR_CLAUDE) \
		-judge-validate $(CONTEXTEVAL_LABELS) -append $(CONTEXTEVAL_DATA) $(CONTEXTEVAL_VALIDATE_ARGS)
	$(GO) run ./scripts/contexteval -judge $(CONTEXTEVAL_JUDGE_FOR_GEMINI) \
		-judge-validate $(CONTEXTEVAL_LABELS) -append $(CONTEXTEVAL_DATA) $(CONTEXTEVAL_VALIDATE_ARGS)

# ── Frontend Checks ──────────────────────────────────────────────────────────

audit-vite-alias: ## Assert the vite catalog alias tracks the vite-plus devDependency version
	bash scripts/audit-vite-alias.sh

frontend-check-all: audit-vite-alias ## Run lint, format, and typecheck across all frontend projects
	$(MAKE) -C bowrain frontend-check-all

story-coverage: ## Report frontend components >=200 lines that lack a Storybook story (non-blocking)
	node scripts/story-coverage.mjs

story-coverage-strict: ## Fail if any tracked tree has a >=200-line component without a story
	node scripts/story-coverage.mjs --strict

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
# the new mascot. Bowrain is a separate brand: see `make bowrain-logo`.
logo: ## Regenerate all neokapi logo/icon/favicon assets from the source pair
	@bash scripts/generate-neokapi-logo.sh

# Regenerate every bowrain-branded logo/icon/favicon from the canonical vector
# mark (bowrain/assets/brand/mark.svg + mark-favicon.svg). Fully scripted —
# drop in an updated mark and re-run.
bowrain-logo: ## Regenerate all bowrain logo/icon/favicon assets from the vector mark
	@bash scripts/generate-bowrain-brand-assets.sh

# ── CDN (S3 + CloudFront) asset publishing ──────────────────────────────────
# The large, desktop-produced docs assets (wasm engine, ONNX vision models,
# walkthrough videos, and screenshots) live ONLY on the S3 CDN origin served at
# $DOCS_CDN_URL (cdn.<domain>, fronted by CloudFront) — referenced by URL
# (ThemedVideo/ThemedImage, the Vision Lab), never committed to git or staged
# into the Pages artifact / PR-preview bundle. The GitHub docs-assets /
# bowrain-docs-assets releases are retired; these targets are the single publish
# path. Auth via env: CDN_BUCKET + AWS credentials (an `aws sso login` profile
# locally). See web/docs/contribute/implementation/repo/cdn-assets.md.
# wasm is also published by CI on each push-to-main docs build (versioned by
# sha). The rest are published from the desktop where the harness produces them;
# the vision model set is pulled from the pinned vision-models-v1 release.
publish-cdn-wasm: web-wasm-demo web-wasm-cli web-pdfium-wasm ## Build + sync the playground wasm → CDN (kapi/wasm/<version>/)
	@bash scripts/publish-cdn-assets.sh wasm

publish-cdn-vision-models: ## Sync the Vision Lab ONNX models → CDN (kapi/models/vision/<web/models.version>/)
	@bash scripts/publish-cdn-assets.sh vision-models

publish-cdn-videos: ## Sync kapi walkthrough videos (web/static/video) → CDN (kapi/video/)
	@bash scripts/publish-cdn-assets.sh video-kapi

publish-cdn-bowrain-videos: ## Sync bowrain walkthrough videos → CDN (bowrain/video/)
	@bash scripts/publish-cdn-assets.sh video-bowrain

publish-cdn-icu: ## Sync the ICU4X segmentation wasm (icu_capi.wasm) → CDN (kapi/icu/<ver>/)
	@bash scripts/publish-cdn-assets.sh icu

# What the skill-eval agents produced (translated .docx, rewritten .pptx, …).
# Staged by the sweep under web/static/skill-eval/artifacts (gitignored, ~50MB a
# run) and linked from the report, so a reader can open the document rather than
# read a byte count. Needs CDN_BUCKET + an aws session, like the others.
publish-cdn-eval-artifacts: ## Publish the skill-eval artefacts → CDN (needs CDN_BUCKET + aws session)
	@bash scripts/publish-cdn-assets.sh eval-artifacts

publish-cdn-images: ## Sync kapi docs images/screenshots (web/static/img) → CDN (kapi/img/)
	@bash scripts/publish-cdn-assets.sh images-kapi

publish-cdn-bowrain-images: ## Sync bowrain docs images/screenshots → CDN (bowrain/img/)
	@bash scripts/publish-cdn-assets.sh images-bowrain

# Publish every CDN asset from the local desktop working copy (where the harness
# renders videos/screenshots and fetch-vision-models stages the models). Run
# after re-recording so the live + preview sites pick up the new assets. (wasm is
# published by CI on the next docs build; not included here.)
# Deliberately not including publish-cdn-eval-artifacts: those exist only after
# a metered sweep has run, so the target would fail on a machine that has not
# run one. They publish with the sweep that produced them.
publish-cdn-all: publish-cdn-videos publish-cdn-bowrain-videos publish-cdn-images publish-cdn-bowrain-images publish-cdn-vision-models publish-cdn-icu ## Publish all desktop-produced assets → CDN (S3)
	@echo "✓ all CDN assets published to the S3 CDN."

# Tier B format corpora (docs/internals/format-maturity.md §2.5): one
# corpus-<id>.tar.gz asset per format on the lexically-latest format-corpus-vN
# release, fetched into corpus/<tag>/<id>/ (gitignored). Tests reference the
# files via the `corpus:` input scheme (core/format/spec) and skip — never
# fail — when the corpus is absent. Publishing stages new files from
# corpus-staging/<id>/ and merges per-format assets (never drops).
fetch-corpus: ## Download Tier B format corpora from the format-corpus release (FORMAT=<id> for one format)
	@FORMAT="$(FORMAT)" bash scripts/fetch-corpus.sh

publish-corpus: ## Publish corpus-staging/<id>/ to the format-corpus release (merges per-format, never drops)
	@bash scripts/publish-corpus.sh

# corpus-sweep (docs/internals/format-ops.md §3 ritual 8) — the out-of-band
# Tier B sweep: every wild file read→write→read in its OWN worker subprocess
# with wall-clock + RSS caps, classified OK/OK_ROUNDTRIP/EXPECTED_REJECT/
# ROUNDTRIP_DRIFT/CRASH/HANG/OOM (Tika ForkParser doctrine). The Go driver
# (cmd/corpus-sweep) runs the sweep and emits a report; record-sweep.mjs folds
# the per-format counts into the ledger's corpus-sweep watermarks. Until
# `make fetch-corpus` has a format-corpus-vN release, no format declares Tier B
# files, so the driver sweeps committed Tier A testdata as a smoke corpus and
# says so. Safety failures (CRASH/HANG/OOM) break the run (exit 3) and
# auto-promote into core/formats/<id>/testdata/fuzz/ regardless of tier;
# ROUNDTRIP_DRIFT is advisory (recorded for the count-delta check) and promotes
# only for Tier B wild files. Promotions are uncommitted, for the maintainer to
# review and add the suggested origin:bug manifest entry. FORMATS=json,po,...
# limits the set (default: every format with a corpus.yaml). Pass NO_PROMOTE=1
# to skip the fuzz-seed promotion.
corpus-sweep: ## Run the Tier B corpus-sweep harness + record counts to the ledger (FORMATS=<id,id,...>)
	@tmp=$$(mktemp); \
	go run ./cmd/corpus-sweep --formats "$(or $(FORMATS),all)" $(if $(NO_PROMOTE),--no-promote,) --report "$$tmp"; \
	status=$$?; \
	node scripts/format-ops/record-sweep.mjs "$$tmp"; \
	rm -f "$$tmp"; \
	exit $$status

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

# okapi-bridge plugin dir feeding the reference dataset.
#
# Bridge inclusion is OPT-IN (WITH_BRIDGE=1), not auto-detected, because the
# generated dataset is COMMITTED. Auto-detection meant the output depended on
# whether the developer happened to have a sibling okapi-bridge checkout built:
# with one, `make generate-reference-docs` wrote ~93 formats and ~156 tools
# instead of the built-in ~38/34, and `make generate-reference-pages` turned
# that into ~180 extra committed MDX pages — enough to run the docs site's
# Docusaurus build out of heap on CI, which is exactly how this was found.
#
# The committed dataset must be reproducible from this repo alone. That is the
# same principle as the kapi dogfood isolation contract above: in-repo tooling
# does not silently consume whatever the developer has installed or checked out
# next door.
BRIDGE_PLUGIN ?= $(NEOKAPI_WORKSPACE_DIR)/okapi-bridge/dist/plugin
BRIDGE_ARG     = $(if $(WITH_BRIDGE),$(if $(wildcard $(BRIDGE_PLUGIN)),-bridge $(BRIDGE_PLUGIN),$(error WITH_BRIDGE=1 but no bridge plugin at $(BRIDGE_PLUGIN))),)

generate-reference-docs: i18n-catalogs ## Generate the reference dataset from THIS repo (built-in + in-repo plugins) → packages/reference-data/data. WITH_BRIDGE=1 adds okapi-bridge (not committed).
	$(GO) run ./scripts/gen-refs $(BRIDGE_ARG)

check-reference-docs: i18n-catalogs ## Drift gate: fail if the committed reference dataset is stale vs. source (gates the built-in subset)
	$(GO) run ./scripts/gen-refs -check $(BRIDGE_ARG)

# Register gate over the authored dossiers the reference dataset is compiled
# from — the recipe's `neokapi-docs-reference` collection, held to the same bar
# as the prose an author writes by hand, at the one place a finding against it
# can be acted on. The generated pages carry a DO-NOT-EDIT banner, so a finding
# reported there would name a file nobody may edit.
#
# The explicit `-p` binds the dogfood recipe (which is what declares the
# collection, its reader config and the voice profile it is judged against);
# $(KAPI_ISO_ENV) still isolates config, caches and plugin discovery, so the gate
# reads nothing of the developer's machine. Any critical, major or minor finding
# fails it: the collection is clean, and a gate that tolerates its own findings
# teaches the reader to stop reading them.
# Stage a built plugin where the isolated kapi can discover it. The iso env
# sets KAPI_PLUGINS_DIR_ONLY, so a developer's Homebrew-installed plugins are
# deliberately invisible — which also means a gate needing one must put it here.
stage-sourcecode-plugin: build-sourcecode-plugin
	@mkdir -p $(KAPI_ISO_DIR)/plugins/sourcecode/formats/sourcecode
	@cp -f $(BIN_DIR)/kapi-sourcecode $(KAPI_ISO_DIR)/plugins/sourcecode/
	@cp -f plugins/sourcecode/manifest.json $(KAPI_ISO_DIR)/plugins/sourcecode/
	@cp -f plugins/sourcecode/formats/sourcecode/schema.json $(KAPI_ISO_DIR)/plugins/sourcecode/formats/sourcecode/

# ── The prose kapi governs, checked BY kapi ──────────────────────────────────
#
# These collections carry product copy in files no docs sweep reaches: the
# description a package manager shows before anything is installed, the one
# Windows shows in file properties, and the cask lines Homebrew prints. Each is
# declared in kapi.yaml and governed by the project's voice profile, so the rule
# is enforced by the engine rather than by a second implementation of it in
# bash.
#
# scripts/check-vocabulary.sh keeps only what kapi cannot open. When a surface
# moves under a collection it comes OUT of that script — one rule, one enforcer
# per surface.
check-governed-prose: build stage-sourcecode-plugin ## Gate: the collections holding distribution prose pass `kapi check`
	$(KAPI_ISO_ENV) $(BIN_DIR)/kapi check 'packaging/nfpm.yaml' \
		-p $(CURDIR)/kapi.yaml --max-major 0
	$(KAPI_ISO_ENV) $(BIN_DIR)/kapi check 'apps/kapi-desktop/build/windows/info.json' \
		-p $(CURDIR)/kapi.yaml --max-major 0
	@# The cask needs the sourcecode plugin, staged above. Minors are allowed
	@# here and nowhere else in this target: `caveats` embeds an aligned command
	@# sample, so the consecutive-spaces rule fires on formatting that is correct.
	$(KAPI_ISO_ENV) $(BIN_DIR)/kapi check 'deploy/homebrew/*.rb' \
		-p $(CURDIR)/kapi.yaml --max-major 0

# ── The prose kapi reads, gated on every PR ──────────────────────────────────
#
# The docs were governed by the voice profile and checked by nothing per-PR:
# dogfood-sync.yml runs `kapi up` on a SCHEDULE, so a violation merged on a
# Tuesday was found on Wednesday, by a bot, on main. scripts/check-vocabulary.sh
# was the only per-PR enforcement, and it holds a second copy of the rule.
#
# This is the gate that lets the script stop owning these surfaces. Minors are
# allowed (--max-major 0): TBX, the XLIFF Glossary module and a handful of
# concept pages name the external standards they document, and those read as
# MINOR by design. Majors and criticals fail.
#
# 379 + 71 files in under two seconds, so it is cheap enough to run on every PR.
check-docs-prose: build ## Gate: the documentation passes `kapi check` under the project's voice
	$(KAPI_ISO_ENV) $(BIN_DIR)/kapi check 'web/docs/**/*.md' 'web/docs/**/*.mdx' \
		-p $(CURDIR)/kapi.yaml --max-major 0
	$(KAPI_ISO_ENV) $(BIN_DIR)/kapi check 'bowrain/web/docs/docs/**/*.md' 'bowrain/web/docs/docs/**/*.mdx' \
		-p $(CURDIR)/kapi.yaml --max-major 0
	@# The two site taglines. They are declared in the docs collections but sit
	@# beside docs/ rather than inside it, so the globs above do not reach them.
	@# A collection that nothing gates is a declaration, not a check.
	$(KAPI_ISO_ENV) $(BIN_DIR)/kapi check 'web/brand.json' 'bowrain/web/docs/brand.json' \
		-p $(CURDIR)/kapi.yaml --max-major 0
	@# The two READMEs: prose kapi has always been able to read, that no
	@# collection declared. A reader meets the README before anything else.
	$(KAPI_ISO_ENV) $(BIN_DIR)/kapi check 'README.md' 'bowrain/README.md' \
		-p $(CURDIR)/kapi.yaml --max-major 0
	@# NOT the agent skill, and not by omission. Gating cli/skills reports 30
	@# majors, every one of them correct-by-design: @angular/localize and
	@# expo-localization are third-party identifiers, gen-l10n is a Flutter tool,
	@# and EVALS.md quotes user prompts verbatim because the skill description is
	@# intent-matching vocabulary — it must contain the words a user types.
	@# check-vocabulary.sh lists it under PENDING_SURFACES for the same reason.
	@# Deciding what a matching surface owes the vocabulary rule comes first.

check-reference-prose: build ## Register gate: the authored reference dossiers pass `kapi check` with no findings
	$(KAPI_ISO_ENV) $(BIN_DIR)/kapi check 'scripts/gen-refs/nativedocs/*/*.yaml' \
		-p $(CURDIR)/kapi.yaml --max-major 0 --max-minor 0

# Superseded by generate-reference-docs; kept as an alias for existing callers.
generate-format-docs: generate-reference-docs

generate-contract-types: ## Generate the shared TS contract + content-model types and the content JSON Schema from Go (core/schema, core/proto/content/v1)
	$(GO) run ./scripts/gen-contract-types

check-contract-types: ## Drift gate: fail if the committed contract/content types or content JSON Schema are stale vs. Go
	$(GO) run ./scripts/gen-contract-types -check

generate-translatability: ## Generate the W3C translatability table for the Go readers from packages/i18n-react (TS is the single definition)
	node --no-warnings --experimental-strip-types scripts/gen-translatability.ts

check-translatability: ## Drift gate: fail if core/translatability/data/w3c.json is stale vs. the TypeScript table
	node --no-warnings --experimental-strip-types scripts/gen-translatability.ts -check

generate-reference-pages: i18n-catalogs ## Generate static per-entry reference MDX pages (R4, #673) → web/docs/reference/{commands,formats,tools}
	cd web && node --no-warnings --experimental-strip-types scripts/gen-reference-pages.ts

# ── Documentation Site ──────────────────────────────────────────────────────

docs-deps: ; cd web && vp install --frozen-lockfile
docs-dev: docs-wasm ; cd web && vp run start
docs-build: ; cd web && vp run build
docs-serve: ; cd web && vp run serve

# Stage the Vision Lab models same-origin (web/static/models/vision) so the
# browser can fetch them without CORS (GitHub release URLs are CORS-blocked for
# fetch()). The docs deploy git-pushes the built site, so files must stay under
# the 100 MB git limit: OCR models (det/rec/dict) ship whole; the ~132 MB layout
# model is split into <100 MB parts plus a "<name>.json" manifest, which the
# browser reassembles before inference (visionBridge.fetchModel). Staged at
# build, never committed (web/.gitignore covers static/models).
VISION_MODELS_REL := https://github.com/neokapi/neokapi/releases/download/vision-models-v1
fetch-vision-models: ## Stage Vision Lab models (OCR whole + layout chunked) → web/static/models/vision
	@mkdir -p web/static/models/vision
	@for f in ppocrv5_det.onnx ppocrv5_rec.onnx ppocrv5_dict.txt; do \
	  if [ ! -f web/static/models/vision/$$f ]; then \
	    gh release download vision-models-v1 -p $$f -O web/static/models/vision/$$f 2>/dev/null \
	      || curl -sSfL -o web/static/models/vision/$$f "$(VISION_MODELS_REL)/$$f"; \
	  fi; \
	done
	@if [ ! -f web/static/models/vision/ppdoclayoutv3.onnx.json ]; then \
	  tmp=$$(mktemp); \
	  gh release download vision-models-v1 -p ppdoclayoutv3.onnx -O $$tmp 2>/dev/null \
	    || curl -sSfL -o $$tmp "$(VISION_MODELS_REL)/ppdoclayoutv3.onnx"; \
	  bytes=$$(wc -c < $$tmp | tr -d ' '); \
	  ( cd web/static/models/vision && rm -f ppdoclayoutv3.onnx.part-* \
	    && split -b 94371840 "$$tmp" ppdoclayoutv3.onnx.part- ); \
	  parts=$$(cd web/static/models/vision && ls ppdoclayoutv3.onnx.part-* | sort | sed 's/.*/"&"/' | paste -sd, -); \
	  printf '{"parts":[%s],"bytes":%s}\n' "$$parts" "$$bytes" > web/static/models/vision/ppdoclayoutv3.onnx.json; \
	  rm -f $$tmp; \
	fi
	@echo "Staged Vision Lab models → web/static/models/vision"

# Output dir for the in-browser playground (gitignored; built locally or in CI).
WASM_DEMO_DIR := web/static/wasm

web-wasm-demo: i18n-catalogs ## Build the in-browser playground wasm + JS glue → web/static/wasm/
	@mkdir -p $(WASM_DEMO_DIR)
	GOOS=js GOARCH=wasm $(GO) build -o $(WASM_DEMO_DIR)/kapi.wasm ./cmd/kapi-wasm
	@cp "$$($(GO) env GOROOT)/lib/wasm/wasm_exec.js" $(WASM_DEMO_DIR)/wasm_exec.js
	@ls -lh $(WASM_DEMO_DIR)/kapi.wasm | awk '{print "  built",$$NF,$$5}'

# Self-host @embedpdf/pdfium's wasm next to the kapi wasm so the browser PDF
# reader's bridge (packages/kapi-playground/src/pdfiumBridge.ts) can fetch it
# locally — no CDN, no Content-Type/streaming-compile constraints. Requires
# `vp install` to have populated node_modules. Best-effort: warns if absent so a
# non-PDF docs build still succeeds (PDF inspection then surfaces a clear error).
PDFIUM_WASM_SRC := node_modules/@embedpdf/pdfium/dist/pdfium.wasm
web-pdfium-wasm: ## Stage @embedpdf/pdfium wasm → web/static/wasm/pdfium.wasm
	@mkdir -p $(WASM_DEMO_DIR)
	@if [ -f "$(PDFIUM_WASM_SRC)" ]; then \
		cp "$(PDFIUM_WASM_SRC)" $(WASM_DEMO_DIR)/pdfium.wasm; \
		ls -lh $(WASM_DEMO_DIR)/pdfium.wasm | awk '{print "  staged",$$NF,$$5}'; \
	else \
		echo "  warning: $(PDFIUM_WASM_SRC) not found — run 'vp install'; browser PDF disabled"; \
	fi

web-wasm-cli: web-pdfium-wasm i18n-catalogs ## Build the in-browser kapi CLI (wasm) → web/static/wasm/kapi-cli.wasm
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
WASM_SRC_DIRS := core cli kapi providers memory terms cmd
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

kbf-smoke: web-wasm-cli ## Verify KBF Go(wasm)↔TS parity for the docs Tests page (serialization, preview, anchors, validation)
	node --experimental-strip-types scripts/verify-snippets/kbf-smoke.ts

kpz-smoke: build ## Verify the resumable .kpz workspace lifecycle (open→step→finish == one-shot; pack stable)
	bash scripts/kpz-smoke.sh $(BIN_DIR)/kapi

kpz-wasm-smoke: web-wasm-cli ## Verify .kpz workspace + .kapi project run in the browser WASM engine (JSON + Office)
	GOROOT="$$($(GO) env GOROOT)" node --experimental-strip-types scripts/kpz-wasm-smoke.ts

wasm-surface-smoke: web-wasm-cli ## Verify no browser verb answers "unknown command", gaps explain themselves, and the labs' own argv still runs
	node --experimental-strip-types scripts/verify-snippets/command-surface-smoke.ts

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

landing-build: ## Build the bowrain landing page (en + nb variants) with its production base URL → bowrain/web/landing/dist
	cd bowrain/web/landing && VITE_BASE=$(BOWRAIN_LANDING_BASE) vp run build
	cd bowrain/web/landing && VITE_BASE=$(BOWRAIN_LANDING_BASE) vp run build:nb

docs-build-prod: web-wasm-demo web-wasm-cli ## Build the kapi docs+landing site with the production base (set DOCS_CDN_URL so videos/models/images resolve from R2) → web/build
	cd web && DOCS_BASE_URL=$(NEOKAPI_DOCS_BASE) vp run build

bowrain-docs-build-prod: ## Build the standalone bowrain docs site with the production base → bowrain/web/docs/build
# bowrain/web/docs carries its own pnpm-workspace.yaml (packages: []), so it is
# already its own workspace root and needs no --ignore-workspace to stay out of
# the repo workspace. The flag is actively harmful here: it makes pnpm skip that
# file, including its packageManagerRegistries pin, and the package-manager
# bootstrap then fails on any machine whose default registry is a mirror.
	cd bowrain/web/docs && corepack pnpm install
	cd bowrain/web/docs && DOCS_BASE_URL=$(BOWRAIN_DOCS_BASE) vpx docusaurus build

publish-landing: landing-build ## Build + deploy the bowrain landing page to neokapi.github.io (PAGES_PUBLISH_YES=1 to skip prompt)
	@bash scripts/publish-pages.sh bowrain-landing

publish-website: docs-build-prod bowrain-docs-build-prod ## Build + deploy the kapi & bowrain docs sites to neokapi.github.io (set DOCS_CDN_URL so assets resolve from R2)
	@bash scripts/publish-pages.sh neokapi-docs bowrain-docs

# ── Tools ────────────────────────────────────────────────────────────────────

tools: ## Install development tools
	$(GO) install google.golang.org/protobuf/cmd/protoc-gen-go@latest
	$(GO) install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
	$(GO) install github.com/golangci/golangci-lint/cmd/golangci-lint@latest

setup-remote: ## Install dependencies for cloud environments
	CLAUDE_CODE_REMOTE=true bash scripts/setup-remote.sh

doctor: ## Check this machine can build and test neokapi; report what is missing
	@bash scripts/doctor.sh

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

.PHONY: release release-windows release-winget release-bowrain release-bowrain-windows release-coordinated

release: ## Tag + push a kapi release (v=1.3.4 → tag v1.3.4); CI builds & publishes the rest
	@[ -n "$(strip $(v))" ] || { echo "usage: make release v=1.3.4"; exit 1; }
	@echo "$(VER)" | grep -Eq '^[0-9]+\.[0-9]+\.[0-9]+' || { echo "✗ version must look like 1.3.4 (got '$(v)')"; exit 1; }
	@test -z "$$(git status --porcelain)" || { echo "✗ working tree not clean"; exit 1; }
	@test "$$(git rev-parse --abbrev-ref HEAD)" = "main" || { echo "✗ not on main"; exit 1; }
	@git fetch --quiet origin main
	@test "$$(git rev-parse HEAD)" = "$$(git rev-parse origin/main)" || { echo "✗ local main is not in sync with origin/main"; exit 1; }
	@git rev-parse "$(TAG)" >/dev/null 2>&1 && { echo "✗ tag $(TAG) already exists"; exit 1; } || true
	@git ls-remote --exit-code --tags origin "$(BTAG)" >/dev/null 2>&1 || { \
	  printf '\n⚠️  No %s release found. The kapi CLI container image (ghcr.io/neokapi/kapi:%s)\n' "$(BTAG)" "$(VER)"; \
	  printf '    bundles the kapi-bowrain plugin and is SKIPPED unless a matching %s release\n' "$(BTAG)"; \
	  printf '    publishes its archives — samples and GitLab CI pull that image. Cut them together:\n'; \
	  printf '        make release-coordinated   (or run make release-bowrain v=%s right after this).\n\n' "$(VER)"; \
	}
	@printf "Tag and push %s at %s? [y/N] " "$(TAG)" "$$(git rev-parse --short HEAD)"; read ok; [ "$$ok" = "y" ] || { echo aborted; exit 1; }
	git tag -a "$(TAG)" -m "Release $(TAG)"
	git push origin "$(TAG)"
	@echo ""
	@echo "Pushed $(TAG). Follow CI with:  gh run watch"
	@echo "After CI finishes, sign Windows (with SimplySign Desktop logged in):"
	@echo "    make release-windows v=$(VER)"

release-windows: ## Sign the Windows artifacts, finalize the release, and dispatch winget (after CI; SimplySign logged in)
	@[ -n "$(strip $(v))" ] || { echo "usage: make release-windows v=1.3.4"; exit 1; }
	JSIGN_KEYSTORE="$${JSIGN_KEYSTORE:-$$HOME/simplysign-pkcs11.cfg}" \
		./scripts/publish-windows-signed.sh "$(TAG)"
	@echo "(winget.yml is dispatched automatically once the signed assets are up; SKIP_WINGET=1 to opt out)"

release-winget: ## Re-dispatch the winget update (release-windows already does this; needs WINGET_TOKEN + `komac new` bootstrap)
	@[ -n "$(strip $(v))" ] || { echo "usage: make release-winget v=1.3.4"; exit 1; }
	gh workflow run winget.yml --repo neokapi/neokapi -f tag="$(TAG)"

# ── Bowrain release track (independent of kapi; tag bowrain-vX.Y.Z) ───────────
# The kapi-bowrain plugin, Bowrain Desktop, and bowrain-server images release on
# their own cadence via release-bowrain.yml. Cut bowrain from a commit whose
# kapi-bowrain plugin matches a released kapi (the two share the framework + cli
# modules); they need not be the same commit, but keep them close.
BTAG := bowrain-v$(VER)

release-bowrain: ## Tag + push a bowrain release (v=2.1.0 → tag bowrain-v2.1.0); CI builds & publishes the rest
	@[ -n "$(strip $(v))" ] || { echo "usage: make release-bowrain v=2.1.0"; exit 1; }
	@echo "$(VER)" | grep -Eq '^[0-9]+\.[0-9]+\.[0-9]+' || { echo "✗ version must look like 2.1.0 (got '$(v)')"; exit 1; }
	@test -z "$$(git status --porcelain)" || { echo "✗ working tree not clean"; exit 1; }
	@test "$$(git rev-parse --abbrev-ref HEAD)" = "main" || { echo "✗ not on main"; exit 1; }
	@git fetch --quiet origin main
	@test "$$(git rev-parse HEAD)" = "$$(git rev-parse origin/main)" || { echo "✗ local main is not in sync with origin/main"; exit 1; }
	@git rev-parse "$(BTAG)" >/dev/null 2>&1 && { echo "✗ tag $(BTAG) already exists"; exit 1; } || true
	@printf "Tag and push %s at %s? [y/N] " "$(BTAG)" "$$(git rev-parse --short HEAD)"; read ok; [ "$$ok" = "y" ] || { echo aborted; exit 1; }
	git tag -a "$(BTAG)" -m "Release $(BTAG)"
	git push origin "$(BTAG)"
	@echo ""
	@echo "Pushed $(BTAG). Follow CI with:  gh run watch"
	@echo "After CI finishes, sign Windows (with SimplySign Desktop logged in):"
	@echo "    make release-bowrain-windows v=$(VER)"

release-bowrain-windows: ## Sign the Bowrain Windows artifacts + publish its update feed (after CI; SimplySign logged in)
	@[ -n "$(strip $(v))" ] || { echo "usage: make release-bowrain-windows v=2.1.0"; exit 1; }
	JSIGN_KEYSTORE="$${JSIGN_KEYSTORE:-$$HOME/simplysign-pkcs11.cfg}" \
		./scripts/publish-windows-signed.sh "$(BTAG)"
	@echo "(winget is kapi-only and is skipped for bowrain tags; the Windows update feed is dispatched automatically)"

# ── Coordinated release (kapi + bowrain land in the package managers together) ─
# For a joint launch only — routine ships still use `make release` /
# `make release-bowrain` independently. Dispatches release-coordinated.yml, which
# builds both tracks and holds their tap/registry publishes behind the
# `coordinated-release` GitHub Environment; approve both pending deployments at
# once (Actions UI) and they publish within seconds of each other. Leave a
# version blank to gate only the other track. Requires the environment to have
# required reviewers configured.
release-coordinated: ## Joint launch: kapi=1.3.4 bowrain=2.1.0 → dispatch + manual-approve both publishes together
	@[ -n "$(strip $(kapi))$(strip $(bowrain))" ] || { echo "usage: make release-coordinated kapi=1.3.4 bowrain=2.1.0 (either may be blank)"; exit 1; }
	gh workflow run release-coordinated.yml --repo neokapi/neokapi --ref main \
		-f kapi_version="$(strip $(kapi))" -f bowrain_version="$(strip $(bowrain))"
	@echo ""
	@echo "Dispatched. Both tracks build, then wait at the 'coordinated-release' gate."
	@echo "Approve both pending deployments together:  gh run watch  (or the Actions UI)"

# ── Clean ────────────────────────────────────────────────────────────────────

clean: ## Remove all build artifacts
	rm -rf bin coverage
	@$(MAKE) -C bowrain clean

# ── Help ─────────────────────────────────────────────────────────────────────

help: ## Show this help
	@awk '/^# ── / { \
		gsub(/# ── /, ""); gsub(/ ─+$$/, ""); category = $$0; next \
	} \
	/^[a-zA-Z0-9_-]+:.*## / { \
		match($$0, /## (.*)/); desc = substr($$0, RSTART+3); \
		match($$0, /^[a-zA-Z0-9_-]+/); target = substr($$0, RSTART, RLENGTH); \
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
        parity-sandbox parity-test parity-publish parity-clean regen-okapi-fixtures check-eval batch-eval batch-eval-publish context-eval context-eval-publish context-eval-validate check-models update-model-prices update-model-catalog \
        contract-audit contract-audit-all contract-audit-clean okapi-failsafe-reports \
        fmt vet lint check check-framework check-bowrain check-abs-paths check-vocabulary check-desktop-interchange check-comment-history check-run-projection check-locale-display check-sidebar-ids check-lockfile-idempotent check-package-licenses check-archive-licenses check-plugin-licenses check-tracked-binaries check-gofmt workspace-paths test-parallel \
        test-framework test-cli test-kapi test-platform test-bowrain-plugin test-bowrain \
        test-plugins test-sat-plugin test-check-plugin test-vision-plugin test-asr-plugin test-pdfium-plugin \
        bowrain-desktop-test \
        ci-test-framework ci-test-cli ci-test-kapi ci-test-platform \
        ci-test-bowrain ci-test-kapi-desktop ci-test-bowrain-desktop ci-test-all \
        ci-frontend ci-kapi-desktop-frontend ci-bowrain-desktop-frontend ci-i18n-react ci-build ci-tidy \
        audit-modules audit-vite-alias \
        build build-all build-server build-worker build-kapi-bowrain-plugin build-bowrain-plugin build-bowrain build-headless \
        plugin-bundle dev-skills \
        install install-kapi-bowrain-plugin \
        frontend-check-all \
        build-kapi-desktop kapi-desktop-dev kapi-desktop-test regen-kapimart-sample \
        kapi-desktop-frontend-deps kapi-desktop-frontend-dev kapi-desktop-frontend-build \
        kapi-desktop-frontend-test kapi-desktop-frontend-check kapi-desktop-extract \
        kapi-desktop-bindings bowrain-desktop-bindings wails-bindings check-wails-bindings \
        check-kapi-desktop-bindings check-bowrain-desktop-bindings wails3-cli \
        bowrain-app-extract bowrain-ctrl-extract bowrain-pulse-extract \
        kapi-i18n-generate kapi-cli-i18n-generate i18n-react-build i18n-catalogs \
        l10n l10n-build l10n-extract l10n-converge l10n-pseudo l10n-compile \
        l10n-verify l10n-derived-paths l10n-loop-owned-paths l10n-owned-paths \
        l10n-extract-globs l10n-review-export \
        l10n-collapse-check l10n-report l10n-content-pairs l10n-content-check \
        l10n-orphans l10n-orphans-report l10n-stale-report \
        check-extract-fixtures \
        flow-editor-deps flow-editor-check flow-editor-test \
        kapi-storybook kapi-storybook-build bowrain-storybook bowrain-storybook-build \
        cover test-e2e test-e2e-kapi test-e2e-bowrain test-e2e-cloud test-e2e-dev \
        face-parity face-parity-update \
        bench bench-build bench-run bench-run-full bench-stress \
        logo harness-deps harness-videos \
        harness-seed harness-record harness-narrate harness-package harness-videos-all harness-videos-staged \
        publish-cdn-wasm publish-cdn-vision-models publish-cdn-videos publish-cdn-bowrain-videos \
        publish-cdn-images publish-cdn-bowrain-images publish-cdn-all \
        fetch-corpus publish-corpus corpus-sweep \
        generate-format-docs generate-reference-docs check-reference-docs check-reference-prose generate-reference-pages \
        generate-contract-types check-contract-types \
        generate-translatability check-translatability \
        docs-deps docs-dev docs-wasm docs-build docs-serve docs-verify-snippets \
        kbf-smoke kpz-smoke kpz-wasm-smoke wasm-surface-smoke \
        landing-build landing-build-nb docs-build-prod bowrain-docs-build-prod publish-landing publish-website \
        emails-frontend-deps emails-extract \
        landing-frontend-deps landing-extract \
        tools setup-remote doctor gha-lint clean \
        _fw-fmt _fw-test _fw-test-fast _fw-test-unit _fw-test-race _fw-test-verbose _fw-test-integration \
        _fw-vet _fw-lint _fw-proto _fw-deps _fw-deps-update
