# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

gokapi is an AI-native reimagining of the [Okapi Framework](https://okapiframework.org/) in Go. It provides format-aware document parsing, channel-based concurrent processing flows, and pluggable tools for localization and translation.

The repository is a **multi-module monorepo** with six Go modules:

- **Framework** (`github.com/gokapi/gokapi`) — the open-source localization engine: content model, format readers/writers, processing tools, pipeline executor, plugin system. All framework Go packages live under `core/`. Zero platform dependencies (no SQLite, Wails, Echo, Cobra, OIDC).
- **CLI** (`github.com/gokapi/gokapi/cli`) — shared CLI base used by both kapi and bowrain: App struct, command factories (formats, plugins, tools, flows, presets, termbase, tm, version), output formatting, Viper-based app config, SQLite-backed termbase and TM storage (`cli/storage/`). Depends on framework only. No platform dependency.
- **Platform** (`github.com/gokapi/gokapi/platform`) — shared platform types and interfaces: project model, auth types, connector interfaces, REST client. Depends on framework only. No CLI dependency (no Cobra, Viper).
- **Kapi** (`github.com/gokapi/gokapi/kapi`) — standalone CLI tool for local file processing: format conversion, pseudo-translation, quality checks, etc. Depends on framework + CLI. No platform dependency, no heavy dependencies (no Wails, Echo, OIDC, keyring). SQLite is provided via the CLI module for persistent termbase and TM storage.
- **Bowrain CLI** (`github.com/gokapi/gokapi/bowrain-cli`) — project sync companion CLI: manages `.bowrain/` projects, syncs with Bowrain Server (init, push, pull, auth, status). Depends on framework + CLI + platform.
- **Bowrain** (`github.com/gokapi/gokapi/bowrain`) — the full-stack localization platform: REST server, desktop app, connectors, authentication, persistent SQLite/PostgreSQL storage. Depends on framework + platform. No CLI dependency.

Both **kapi** and **bowrain** CLIs share a common base in `cli/`. The shared base provides command factories for formats, plugins, tools, flows, presets, termbase, and version. Each CLI selects which commands to register and can extend them with CLI-specific behavior (e.g., bowrain adds project flow support via a `RegistryResolver` hook).

A `go.work` file at the root coordinates the six modules for local development. CLI and platform have zero cross-dependency. Kapi and bowrain have no dependency on each other.

## Build & Test Commands

```bash
make build              # Build kapi CLI → bin/kapi
make build-bowrain      # Build Bowrain CLI → bin/bowrain
make build-server       # Build REST server → bin/bowrain-server
make build-all          # Build all Go binaries
make test               # Run all tests (all modules)
make test-framework     # Run framework tests only
make test-cli           # Run cli module tests only
make test-platform      # Run platform module tests only
make test-kapi          # Run kapi CLI tests only
make test-bowrain-cli   # Run Bowrain CLI tests only
make test-bowrain       # Run bowrain tests only
make test-unit          # Unit tests only (-short flag)
make test-race          # Tests with race detector
make test-verbose       # Verbose test output
make cover              # Coverage report → coverage/coverage.html
make fmt                # Format Go source (gofmt -w -s)
make vet                # Run go vet (all modules)
make lint               # Run golangci-lint (all modules)
make check              # fmt + vet + lint
make deps               # Download and tidy Go modules (all modules)
make proto              # Generate gRPC code from protobuf definitions
```

Run a single test: `go test ./core/flow/ -run TestExecutorCancellation -v`

**Web UI (embedded in kapi serve):**
```bash
make web-deps                                 # npm install for web UI
make web-build                                # Build web UI → bowrain/apps/web/dist/
```

**Bowrain (desktop GUI):**
```bash
cd bowrain/apps/bowrain && wails3 build       # Build native macOS/Linux/Windows app
cd bowrain/apps/bowrain && wails3 dev         # Dev mode with hot reload
make frontend-deps                            # npm install for frontend
make frontend-build                           # Production frontend build
```

**Documentation site:**
```bash
cd website && npm start              # Dev server with hot reload
cd website && npm run build          # Production build → website/build/
```

## Build Conventions

Always prefer `make` targets over raw `go build` / `go test` commands. The Makefile handles prerequisites (e.g. `make build` requires `make web-build` first for the embedded web UI) and places binaries in `bin/` rather than the repo root. Use direct `go test` only when targeting a specific package or test function.

For the multi-module structure:
- Framework packages build from the root: `go build ./...`
- CLI packages: `cd cli && go build ./...`
- Platform packages: `cd platform && go build ./...`
- Kapi CLI: `cd kapi && go build ./...`
- Bowrain CLI: `cd bowrain-cli && go build ./...`
- Bowrain packages: `cd bowrain && go build ./...`
- With `go.work`, all modules resolve cross-module imports automatically
- `GOWORK=off go build ./...` verifies framework module isolation
- `GOWORK=off bash -c "cd cli && go build ./..."` verifies cli isolation (no platform dep)
- `GOWORK=off bash -c "cd platform && go build ./..."` verifies platform isolation (no cli dep)
- `GOWORK=off bash -c "cd kapi && go build ./..."` verifies kapi isolation (no platform dep)

## Architecture

### Multi-Module Structure

```
gokapi/
├── go.work                # Workspace: use . ./cli ./platform ./kapi ./bowrain-cli ./bowrain
├── go.mod                 # module github.com/gokapi/gokapi (framework)
│
│   ── Framework Module ──────────────────
├── core/
│   ├── model/             # Content model (Part, Block, Fragment, Span, Layer)
│   ├── format/            # DataFormatReader/Writer interfaces
│   ├── tool/              # Tool interface
│   ├── flow/              # FlowExecutor, pipeline orchestration
│   ├── registry/          # Format and tool registries
│   ├── encoding/          # Character encoding detection/conversion
│   ├── locale/            # BCP-47 locale utilities
│   ├── editor/            # Block index serialization and preview generation
│   ├── version/           # Version info (set via ldflags)
│   ├── formats/           # 15 built-in format implementations
│   ├── ai/                # LLM providers + AI tools
│   ├── mt/                # MT providers + MT tools
│   ├── sievepen/          # Translation memory (interface + in-memory + matching)
│   ├── termbase/          # Terminology (interface + in-memory + import)
│   ├── tools/             # Built-in utility tools
│   ├── plugin/            # go-plugin + gRPC plugin system + Java bridge
│   └── testutil/          # Shared test helpers
├── examples/              # Plugin examples
│
│   ── CLI Module ────────────────────────
├── cli/
│   ├── go.mod             # module github.com/gokapi/gokapi/cli (framework only)
│   ├── config/            # Viper-based app configuration (~/.config/kapi/)
│   ├── output/            # Shared output formatting + types (used by kapi & bowrain)
│   └── storage/           # SQLite-backed termbase and TM for CLI workflows
│
│   ── Platform Module ───────────────────
├── platform/
│   ├── go.mod             # module github.com/gokapi/gokapi/platform (framework only)
│   ├── project/           # .bowrain/ project model (types, config, sync cache)
│   ├── auth/              # Auth types, JWT, device flow client
│   ├── connector/         # Connector interfaces + base types
│   ├── client/            # REST client for bowrain API
│   ├── config/            # Auth persistence (StoredAuth, LoadAuth, SaveAuth)
│   ├── store/             # ContentStore interface + domain types
│   ├── event/             # Event types + bus interface
│   └── credentials/       # Provider credential management
│
│   ── Kapi Module ───────────────────────
├── kapi/
│   ├── go.mod             # module github.com/gokapi/gokapi/kapi (framework + cli)
│   ├── cmd/kapi/          # Thin root cmd wiring shared CLI commands
│   └── apps/
│       └── kapi-web/      # kapi serve web UI
│
│   ── Bowrain CLI Module ──────────────────
├── bowrain-cli/
│   ├── go.mod             # module github.com/gokapi/gokapi/bowrain-cli (framework + cli + platform)
│   └── cmd/bowrain/       # Bowrain CLI (project cmds + shared CLI base)
│       └── output/        # Bowrain CLI-specific output types
│
│   ── Bowrain Module ────────────────────
├── bowrain/
│   ├── go.mod             # module github.com/gokapi/gokapi/bowrain (framework + platform)
│   ├── auth/              # OIDC, AuthStore, SQLite + PostgreSQL auth (server-specific)
│   ├── connector/         # Concrete connector implementations (File, Git, etc.)
│   ├── store/             # SQLite + PostgreSQL ContentStore implementations
│   ├── storage/           # SQLite + PostgreSQL migration utilities
│   ├── server/            # HTTP/gRPC server handlers
│   ├── service/           # Auth, project, connector, flow services
│   ├── event/             # Event bus implementation + automation
│   ├── credentials/       # Keyring-backed credentials
│   ├── sievepen/          # SQLite + PostgreSQL TM implementation
│   ├── termbase/          # SQLite + PostgreSQL TermBase implementation
│   ├── proto/v1/          # gRPC protobuf definitions
│   ├── cmd/bowrain-server/ # Echo v4 REST API server
│   └── apps/
│       ├── bowrain/       # Wails v3 desktop app (Go + React/TS)
│       └── web/           # SaaS web UI
│
│   ── Shared Frontend ───────────────────
├── packages/
│   └── ui/                # Shared React component library
│
│   ── Non-Go Assets ─────────────────────
├── docs/                  # Architecture decisions, implementation notes
├── website/               # Docusaurus site
├── e2e/                   # E2E test infra
├── deploy/                # Deployment configs
└── Makefile               # Multi-module build targets
```

### Bowrain Project Model (.bowrain/ Directories)

Bowrain CLI uses a git-like project model with `.bowrain/` directories ([AD-016](docs/ad/016-kapi-project-model.md)):

```
my-app/
├── .bowrain/
│   ├── config.yaml      # Project configuration
│   ├── flows/           # Flow definitions (YAML)
│   │   └── pseudo.yaml
│   └── .sync-cache      # Sync cache (gitignored)
├── src/
│   └── locales/
│       ├── en-US.json
│       └── fr-FR.json
```

**Key Bowrain CLI commands:**
```bash
bowrain init                    # Create .bowrain/ project
bowrain status                  # Show sync state (like git status)
bowrain pull                    # Fetch from Bowrain Server → update local files
bowrain push                    # Send local files → update Bowrain Server
bowrain flow run <flow-name>    # Execute flow from .bowrain/flows/
bowrain serve                   # Start local dashboard (web UI)
```

**All `bowrain` commands require a `.bowrain/` project.** The CLI searches upward from the current directory (like git) to find the project root.

**Key kapi CLI commands (standalone, no project needed):**
```bash
kapi formats list             # List available formats
kapi flow run pseudo-translate -i file.json --target-lang qps   # Process files directly
kapi plugins list             # List installed plugins
kapi presets list             # List available presets
```

**Role Separation:**
- **Kapi** = standalone file-processing tool, demonstrates gokapi's power as open-source toolchain
- **Bowrain CLI** (`bowrain` binary) = project sync companion CLI, focuses on DX and project simplicity for Bowrain
- **Shared CLI base** (`cli/`) = common commands (formats, plugins, tools, flows, presets, termbase, version) used by both kapi and bowrain
- **Bowrain Server** = integration platform (CMS connectors, automation, ContentStore)

### Streaming Pipeline

Documents flow through a channel-based concurrent pipeline:

```
RawDocument → DataFormatReader → [Tool 1] → [Tool 2] → ... → DataFormatWriter → Output
                                    ↕            ↕
                              chan *Part    chan *Part
```

Each tool runs in its own goroutine. Buffered channels (default 64) provide backpressure. `errgroup.Group` coordinates error handling. Context cancellation propagates to all stages.

### Content Model (core/model/)

The Part is the fundamental streaming unit, carrying a PartType discriminator and a Resource:

- **Layer** — structural grouping (document, section, embedded content). Layers nest: embedded content (HTML inside JSON) becomes a child Layer with its own DataFormat.
- **Block** — translatable content with Source segments and Target segments per locale
- **Fragment** — text with inline Spans using coded text (Unicode private use area markers replace inline markup)
- **Data** — non-translatable structure
- **Media** — binary content

### Key Interfaces

- `format.DataFormatReader` — `Open(ctx, doc)` then `Read(ctx) <-chan PartResult`
- `format.DataFormatWriter` — `SetOutput(path)`, `Write(ctx, <-chan *Part)`
- `tool.Tool` — `Process(ctx, in <-chan *Part, out chan<- *Part) error`
- `flow.FlowExecutor` — orchestrates tool chains with goroutines and channels
- `registry.FormatRegistry` — factory registry for readers/writers with format detection
- `ai/provider.LLMProvider` — interface for Anthropic, OpenAI, Ollama backends

### Terminology Mapping from Okapi

| Okapi (Java) | gokapi (Go) |
|---|---|
| Filter | DataFormat (Reader/Writer) |
| Step | Tool |
| Pipeline | Flow |
| PipelineDriver | FlowExecutor |
| Event | Part |
| TextUnit | Block |
| TextFragment | Fragment |
| Code | Span |
| StartSubDocument/StartSubFilter | Child Layer |

## Implementing a New Format

Create a package under `core/formats/` with reader.go, writer.go, config.go. The reader must implement `format.DataFormatReader` (embed `format.BaseFormatReader`). The writer must implement `format.DataFormatWriter` (embed `format.BaseFormatWriter`). Register both in `core/formats/register.go` via `init()`.

## Implementing a New Tool

Create a type embedding `tool.BaseTool` and set `HandleBlockFn` / `HandleDataFn` / `HandleMediaFn` function fields for the part types you want to process. Parts you don't handle pass through unchanged. Register in the tool registry.

## Testing

Tests use `github.com/stretchr/testify` (assert/require). Table-driven tests are the standard pattern. Format tests typically do roundtrip validation (read → write → compare). Test files colocate with implementation (`*_test.go`).

## Screenshots, Recordings & Screencasts

Screenshots and video recordings serve as documentation and are embedded on the website. **Whenever UI-related code changes, all screenshots and recordings must be regenerated** as part of the verification process before committing.

### Screenshot systems

Screenshots are captured via Playwright and written directly to `website/static/img/`:

1. **Bowrain (desktop GUI)** — 9 screenshots x 2 themes in `bowrain/apps/bowrain/frontend/e2e/screenshots.spec.ts`. Self-contained (auto-starts a Vite dev server). Output: `website/static/img/bowrain/{dark,light}/`.
2. **Web app** — 6 test suites (multiple captures each) x 2 themes in `bowrain/apps/web/e2e/screenshots.spec.ts`. Requires a running bowrain-server with Keycloak OIDC. Output: `website/static/img/web-app/{dark,light}/`.

### Recording systems

There are four independent video recording pipelines:

1. **Bowrain (desktop GUI)** — 13 scenarios x 2 themes (dark + light) in `bowrain/apps/bowrain/frontend/e2e/recordings.spec.ts`. Uses real bowrain-server via Wails dev mode for recordings/screenshots. Mocks (`mock-backend.ts`) are used for e2e unit tests only.
2. **Web app** — 8 scenarios x 2 themes (dark + light) in `bowrain/apps/web/e2e/recordings.spec.ts`. Requires a running bowrain-server with Keycloak OIDC.
3. **Kapi CLI** — VHS terminal recordings from `.tape` files in `website/tapes/` (3 standalone kapi demos). No server required.
4. **Bowrain CLI** — VHS terminal recordings from `.tape` files in `bowrain/e2e/tapes/` (10 bowrain demos, some need server).

### How to regenerate

**Locally:**
```bash
# 1. Bowrain screenshots + recordings (self-contained)
make screenshots                 # screenshots → website/static/img/bowrain/{dark,light}/
make recordings                  # recordings → website/static/video/bowrain/{dark,light}/

# 2. Web app screenshots + recordings (needs Keycloak + local server)
docker compose up -d --wait   # starts Keycloak + Mailpit
make dev-server               # builds + starts bowrain-server locally
cd bowrain/apps/web && npm run e2e:screenshots
cd bowrain/apps/web && npm run e2e:recordings
THEME=dark  bash bowrain/apps/web/scripts/copy-recordings.sh
THEME=light bash bowrain/apps/web/scripts/copy-recordings.sh
# Ctrl-C the server, then:
docker compose down -v

# 3. Kapi CLI recordings (no server needed)
make kapi-recordings             # runs tapes + copies to website/static/video/kapi/

# 4. Bowrain CLI recordings (needs VHS + server)
make bowrain-cli-recordings      # runs tapes + copies to website/static/video/bowrain-cli/

# Or generate everything at once:
make docs-assets                 # screenshots + recordings + cli-recordings
```

**Fetching pre-built assets (no local regeneration needed):**
```bash
make fetch-docs-assets           # downloads tarball from docs-assets GitHub release
```

**In CI:**
```bash
# Automated via GitHub Actions (.github/workflows/screenshots-recordings.yml)
# - On-demand: workflow_dispatch
# - On release: automatically triggered by version tags
# - Nightly: scheduled at 2 AM UTC
#
# All four systems (Bowrain, Web app, Kapi CLI, Bowrain CLI) run in parallel jobs.
# A publish-assets job creates a tarball and uploads it to the "docs-assets"
# GitHub release. The docs deploy workflow fetches this tarball before building.
# Assets are NOT stored in git.
```

### Real systems, not mocks

All screenshots and recordings must run against real gokapi infrastructure. Specifically:

- **Authentication & identity**: Use the real Keycloak OIDC provider via `compose.yaml`. Never mock the auth flow.
- **bowrain-server**: Use the real server binary (locally built). Never use a mock API server.
- **Database & storage**: Use a real SQLite database (bowrain-server creates one automatically).
- **External integrations** outside the scope of this project (e.g. third-party MT providers, external LLM APIs) may be mocked if needed for isolation.

### Verification checklist for UI changes

Before committing any UI-related change:

1. TypeScript checks pass for all 4 projects (`packages/ui`, `bowrain/apps/web`, `kapi/apps/kapi-web`, `bowrain/apps/bowrain/frontend`)
2. All unit tests pass (`cd packages/ui && npm test`)
3. All 3 frontend production builds succeed
4. All screenshots regenerated to `website/static/img/`
5. All recordings regenerated and copied to `website/static/video/`
6. Go build succeeds (`make build build-server`)

## Architecture Decisions

ADs live in `docs/ad/`. They are organized by architectural concern (content model, plugin system, Java bridge, etc.), not by chronological order. Each AD should describe the current state of its subsystem as a self-contained document. When a subsystem evolves, update the existing AD in place rather than appending a new one. Only create a new AD when a genuinely new architectural concern is introduced.

Implementation notes live in `docs/notes/`. These contain tactical details (SQL schemas, API routes, algorithm pseudocode) extracted from ADs to keep decisions focused on the WHY and WHAT.
