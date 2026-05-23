# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

neokapi is an AI-native reimagining of the [Okapi Framework](https://okapiframework.org/) in Go. It provides format-aware document parsing, channel-based concurrent processing flows, and pluggable tools for localization and translation.

The repository is a **multi-module monorepo** with seven Go modules:

- **Framework** (`github.com/neokapi/neokapi`) — the open-source localization engine: content model, format readers/writers, processing tools, pipeline executor, plugin system, SQLite-backed TM and termbase (`sievepen/`, `termbase/`), shared SQLite infrastructure (`core/storage/`), `.kapi` project file format (`core/project/`), AI providers (`providers/ai/`, package `aiprovider`), MT providers (`providers/mt/`, package `mtprovider`). Framework packages live under `core/`, `sievepen/`, `termbase/`, and `providers/`. No bowrain dependencies (no Wails, Echo, Cobra, OIDC).
- **CLI** (`github.com/neokapi/neokapi/cli`) — shared CLI base used by both kapi and bowrain: App struct, command factories (formats, plugins, tools, flows, presets, termbase, tm, version), output formatting, Viper-based app config, OS keychain credential store (`cli/credentials/`). Uses framework's SQLite TM/termbase from `sievepen/` and `termbase/`. Depends on framework only. No bowrain dependency.
- **Bowrain Core** (`github.com/neokapi/neokapi/bowrain/core`) — shared platform types and interfaces: project model, auth types, connector interfaces, REST client, store interfaces, event types. Depends on framework only. No CLI dependency (no Cobra, Viper).
- **Kapi** (`github.com/neokapi/neokapi/kapi`) — primary CLI binary (Apache-2.0). Contains zero vendor-plugin code. Plugins (bowrain, okapi-bridge, …) are discovered at runtime via the unified manifest model (#438) and dispatched as subprocesses. Depends on framework + CLI only.
- **Kapi Desktop** (`github.com/neokapi/neokapi/kapi-desktop`) — Wails v3 desktop app for visual localization workflows. Blank-imports `bowrain/plugin/schema` so it validates bowrain recipes on open. Depends on framework + CLI + bowrain/plugin/schema.
- **Bowrain CLI** (`github.com/neokapi/neokapi/bowrain/cli`) — produces the `kapi-bowrain` manifest-driven plugin binary (in `cmd/kapi-bowrain/`). The plugin is dispatched to by kapi under the unified plugin model. The legacy standalone `bowrain` binary has been retired; all bowrain commands flow through `kapi <command>` once `bowrain-cli` is brew-installed.
- **Bowrain Plugin** (`github.com/neokapi/neokapi/bowrain/plugin`) — Go packages implementing bowrain's behavior: `schema/` (recipe extension decoders, registers via `init()` against `core/project.RegisterExtension`), `commands/` (push, pull, sync, status, init, auth, …, registers via `cli.RegisterCommandFactory`), `connector/` (BowrainSourceConnector), `mcp/` (bowrain MCP tools, registers via `cli.RegisterMCPToolFactory`). These are blank-imported into the `kapi-bowrain` plugin binary; they are no longer imported by the default `kapi` binary. The `schema/` sub-package has its own go.mod so kapi-desktop can blank-import it cheaply.
- **Bowrain Core** (`github.com/neokapi/neokapi/bowrain/core`) — shared bowrain platform types: Recipe wrapper around the framework's `KapiProject` (with type aliases re-exported from `bowrain/plugin/schema`), Project facade, sync cache helpers, REST client, auth, store interfaces, event types.
- **Bowrain** (`github.com/neokapi/neokapi/bowrain`) — the full-stack localization platform: REST server, desktop app, web app, connectors, persistent SQLite/PostgreSQL storage. Depends on framework + bowrain/core.

Both **kapi** and **bowrain** binaries share a common base in `cli/`. The base provides core commands (run, extract, merge, flows, tools, formats, plugins, presets, termbase, tm, mcp, version) plus four plugin registries: `cli.RegisterCommandFactory`, `cli.RegisterAppInitializer`, `cli.RegisterMCPToolFactory` (CLI-side), and `core/project.RegisterExtension` (framework, for recipe schema). Plugins blank-imported into a binary contribute commands, MCP tools, and recipe schema via `init()` registration. See [Note: Plugin model](web/docs/docs/notes-internal/plugin-model.md).

A `go.work` file at the root coordinates the modules for local development. The framework (`core/`) stays platform-agnostic — bowrain attaches via the extension mechanism and the CLI plugin registries, not via direct imports from `core/` to bowrain.

## Build & Test Commands

```bash
make build              # Build kapi CLI → bin/kapi
make build-all          # Build all Go binaries
make test               # Run all tests (framework + bowrain)
make test-unit          # Unit tests only (-short flag)
make test-race          # Tests with race detector
make test-verbose       # Verbose test output
make cover              # Coverage report → coverage/coverage.html
make fmt                # Format Go source (gofmt -w -s)
make vet                # Run go vet (all modules)
make lint               # Run golangci-lint (all modules)
make check              # fmt + vet + lint
make pre-push           # Run checks relevant to your changes (mirrors CI)
make pre-push-all       # Run all checks regardless of changes
make frontend-check-all # Lint + format + typecheck all frontend projects
make kapi-desktop-frontend-check  # Lint + format + typecheck Kapi Desktop
make flow-editor-check  # Lint + format + typecheck flow-editor
make deps               # Download and tidy Go modules (all modules)
make proto              # Generate gRPC code from protobuf definitions
make -C bowrain build-server   # Build bowrain server
vp install              # Install all frontend workspace members (run at repo root)
```

> **Note:** A single root `package.json` npm workspace coordinates all frontend
> packages (`packages/ui`, `packages/flow-editor`, `kapi/apps/kapi-web`,
> `apps/kapi-desktop/frontend`, `bowrain/apps/bowrain/frontend`,
> `bowrain/apps/web`, `bowrain/apps/ctrl`, `website`). Run `vp install` at the
> repo root — no per-directory installs are needed.

Run a single test: `go test ./core/flow/ -run TestExecutorCancellation -v`

**Kapi:**

```bash
make kapi-desktop-test              # Run Go backend tests
make kapi-desktop-frontend-deps     # Install frontend dependencies
make kapi-desktop-frontend-test     # Run frontend vitest tests
make kapi-desktop-frontend-check    # Lint + format + typecheck
make kapi-storybook                 # Run Kapi Storybook on port 6007
make kapi-storybook-build           # Build Kapi Storybook static site
make bowrain-storybook              # Run Bowrain Storybook on port 6006
make bowrain-storybook-build        # Build Bowrain Storybook static site
```

**Web UI (embedded in kapi serve):**

```bash
make web-deps                                 # vp install for web UI
make web-build                                # Build web UI → bowrain/apps/web/dist/
```

**Bowrain (desktop GUI):**

```bash
cd bowrain/apps/bowrain && wails3 build       # Build native macOS/Linux/Windows app
cd bowrain/apps/bowrain && wails3 dev         # Dev mode with hot reload
make frontend-deps                            # vp install for frontend
make frontend-build                           # Production frontend build
```

**Documentation site:**

```bash
cd web/docs && vp run start           # Dev server with hot reload
cd web/docs && vp run build           # Production build → web/docs/build/
```

## Build Conventions

Always prefer `make` targets over raw `go build` / `go test` commands. The Makefile handles prerequisites (e.g. `make build` requires `make web-build` first for the embedded web UI) and places binaries in `bin/` rather than the repo root. Use direct `go test` only when targeting a specific package or test function.

For the multi-module structure:

- Framework packages build from the root: `go build ./...`
- CLI packages: `cd cli && go build ./...`
- Bowrain Core packages: `cd bowrain/core && go build ./...`
- Kapi CLI: `cd kapi && go build ./...`
- Bowrain CLI: `cd bowrain/cli && go build ./...`
- Bowrain packages: `cd bowrain && go build ./...`
- Kapi Desktop: `cd apps/kapi-desktop && go build ./...`
- With `go.work`, all modules resolve cross-module imports automatically
- `GOWORK=off go build ./...` verifies framework module isolation
- `GOWORK=off bash -c "cd cli && go build ./..."` verifies cli isolation (no bowrain dep)
- `GOWORK=off bash -c "cd bowrain/core && go build ./..."` verifies bowrain/core isolation (no cli dep)
- `GOWORK=off bash -c "cd kapi && go build ./..."` verifies kapi isolation (no bowrain dep)
- `GOWORK=off bash -c "cd apps/kapi-desktop && go build ./..."` verifies kapi-desktop isolation (no bowrain dep)

## Architecture

### Multi-Module Structure

```
neokapi/
├── go.work                # Workspace: use . ./cli ./kapi ./apps/kapi-desktop ./bowrain/core ./bowrain/cli ./bowrain
├── go.mod                 # module github.com/neokapi/neokapi (framework, Apache-2.0)
│
│   ── Framework Module (repo root) ──────
├── core/
│   ├── model/             # Content model (Part, Block, Fragment, Span, Layer)
│   ├── format/            # DataFormatReader/Writer interfaces
│   ├── tool/              # Tool interface
│   ├── flow/              # Executor, pipeline orchestration
│   ├── registry/          # Format and tool registries
│   ├── encoding/          # Character encoding detection/conversion
│   ├── locale/            # BCP-47 locale utilities
│   ├── editor/            # Block index serialization and preview generation
│   ├── version/           # Version info (set via ldflags)
│   ├── formats/           # 15 built-in format implementations
│   ├── storage/           # Shared SQLite DB infrastructure (Open, Migrate)
│   ├── project/           # .kapi project file format (Load, Save, Validate)
│   ├── tools/             # Built-in utility tools
│   ├── plugin/            # go-plugin + gRPC plugin system + Java bridge
│   └── testutil/          # Shared test helpers
├── sievepen/              # Translation memory (interface + in-memory + SQLite + matching)
├── termbase/              # Terminology (interface + in-memory + SQLite + import)
├── providers/
│   ├── ai/                # package aiprovider — LLM providers + AI tools
│   └── mt/                # package mtprovider — MT providers + MT tools
├── bench/                 # Benchmarks
├── examples/              # Plugin examples
│
│   ── CLI Module ────────────────────────
├── cli/
│   ├── go.mod             # module github.com/neokapi/neokapi/cli (framework only)
│   ├── config/            # Viper-based app configuration (~/.config/kapi/)
│   ├── output/            # Shared output formatting + types (used by kapi & bowrain)
│   └── storage/           # SQLite-backed termbase and TM for CLI workflows
│
│   ── Kapi Module ───────────────────────
├── kapi/
│   ├── go.mod             # module github.com/neokapi/neokapi/kapi (framework + cli)
│   ├── cmd/kapi/          # Thin root cmd wiring shared CLI commands
│   └── apps/
│       └── kapi-web/      # kapi serve web UI
│
│   ── Kapi Desktop Module ───────────────
├── apps/
│   └── kapi-desktop/      # Wails v3 desktop app (Go + React/TS)
│       ├── go.mod         # module github.com/neokapi/neokapi/kapi-desktop (framework + cli)
│       ├── main.go        # Wails v3 entry point
│       ├── backend/       # Go backend: project, flows, runner, credentials, plugins
│       ├── frontend/      # React 19 + Vite + TailwindCSS
│       └── build/         # Wails build config + platform-specific settings
│
│   ── Bowrain (ALL AGPL-3.0 CODE) ──────
├── bowrain/
│   ├── go.mod             # module github.com/neokapi/neokapi/bowrain (framework + bowrain/core)
│   ├── Makefile           # Bowrain-specific build targets
│   │
│   │   ── Bowrain Core Module ───────────
│   ├── core/              # module github.com/neokapi/neokapi/bowrain/core (framework only)
│   │   └── auth/ store/ connector/ project/ event/ agent/ client/ config/
│   │
│   │   ── Bowrain CLI Module ────────────
│   ├── cli/               # module github.com/neokapi/neokapi/bowrain/cli (framework + cli + bowrain/core)
│   │   └── cmd/kapi-bowrain/   # Manifest-driven kapi-bowrain plugin binary (Mode A/B/C)
│   │
│   ├── auth/              # OIDC, AuthStore, SQLite + PostgreSQL auth (server-specific)
│   ├── connector/         # Concrete connector implementations (File, Git, etc.)
│   ├── store/             # SQLite + PostgreSQL ContentStore implementations
│   ├── storage/           # SQLite + PostgreSQL migration utilities
│   ├── server/            # HTTP/gRPC server handlers
│   ├── service/           # Auth, project, connector, flow services
│   ├── event/             # Event bus implementation + automation
│   ├── billing/           # Billing and subscription management
│   ├── jobs/              # Background job processing
│   ├── brand/             # Brand management
│   ├── graph/             # Graph data structures
│   ├── analytics/         # Analytics and reporting
│   ├── sievepen/          # SQLite + PostgreSQL TM implementation
│   ├── termbase/          # SQLite + PostgreSQL TermBase implementation
│   ├── proto/             # gRPC protobuf definitions
│   ├── cmd/bowrain-server/ # Echo v4 REST API server
│   ├── cmd/bowrain-worker/ # Background worker
│   ├── apps/
│   │   ├── bowrain/       # Wails v3 desktop app (Go + React/TS)
│   │   ├── web/           # SaaS web UI
│   │   ├── ctrl/          # Admin control panel
│   │   ├── pulse/         # Real-time dashboard
│   │   └── keycloak-theme/ # Custom Keycloak theme
│   ├── packages/ui/       # @neokapi/ui (AGPL)
│   ├── storybook/         # Bowrain Storybook config (port 6006, aggregates Kapi + Bowrain stories)
│   ├── docker/            # Docker configurations
│   ├── deploy/            # Deployment configs
│   ├── e2e/               # End-to-end tests
│   ├── emails/            # Email templates
│   ├── compose.yaml
│   └── compose.override.yaml
│
│   ── Shared Frontend (Apache-2.0) ─────
├── package.json           # Root npm workspace coordinating all frontend packages
├── .npmrc                 # install-strategy=hoisted (npm 11)
├── storybook/             # Kapi Storybook config (port 6007, aggregates packages/ui + flow-editor + kapi-desktop)
├── packages/
│   ├── ui/                # @neokapi/ui-primitives — shadcn/ui primitives consumed by kapi-desktop and bowrain apps
│   ├── flow-editor/       # @neokapi/flow-editor — shared React flow editor component library
│   └── storybook-config/  # @neokapi/storybook-config — shared Storybook preview/main factories
│
│   ── Non-Go Assets ─────────────────────
├── docs/                  # Architecture decisions, implementation notes
├── web/docs/               # Docusaurus site
└── Makefile               # Multi-module build targets
```

### Bowrain Project Model (`.kapi` Recipe + State Dir)

Bowrain CLI uses the framework's unified `.kapi` project model — a `<dir-name>.kapi` recipe at the project root with a `server:` block, plus a sibling `.kapi/` state directory ([Bowrain AD-010](bowrain/docs/architecture-decisions/010-bowrain-cli-and-project-model.md)):

```
my-app/
├── my-app.kapi             # Recipe (committed) — directory-named YAML, includes server: block
├── .kapi/                  # State (gitignored)
│   ├── manifest.yaml
│   ├── tm.db               # authoritative project TM
│   ├── termbase.db         # authoritative project termbase
│   ├── flows/              # optional file-per-flow definitions (committed)
│   │   └── pseudo.yaml
│   └── cache/              # all regenerable caches under one roof
│       ├── blocks.db        # block store
│       ├── sync-cache.json  # bowrain push/pull state
│       ├── extractions/
│       └── collections/
├── src/
│   └── locales/
│       ├── en-US.json
│       └── fr-FR.json
```

A bowrain project is just a kapi project whose recipe declares a `server:` block (compound URL, optional `stream`). Top-level recipe fields cover `defaults`, `content`, `plugins` (map form), `flows`, `hooks`, `automations`, `assets`, `brand_voice`. Auth tokens live in the OS keychain (`bowrain-auth:<server-url>`, `bowrain-refresh:<server-url>`); non-secret metadata sits at `~/.config/bowrain/auth.json`. `BOWRAIN_AUTH_TOKEN` env var works in CI.

**Key Bowrain CLI commands:**

```bash
bowrain init                    # Write <dir-name>.kapi + .kapi/ state dir
bowrain status                  # Show sync state (like git status)
bowrain pull                    # Fetch from Bowrain Server → update local files
bowrain push                    # Send local files → update Bowrain Server
bowrain run <flow-name>         # Execute flow (inline on recipe or .kapi/flows/)
bowrain serve                   # Start local dashboard (web UI)
```

**All `bowrain` commands require a `.kapi` project with a `server:` block.** The CLI searches upward from the current directory (like git) to find the recipe.

**Key kapi CLI commands (standalone, no project needed):**

```bash
kapi ai-translate -i file.xliff --target-lang fr    # Translate with AI
kapi pseudo-translate -i file.json --target-lang qps # Pseudo-translate for testing
kapi word-count file.json                            # Count words
kapi run ai-translate-qa -i file.xliff --target-lang fr  # Run composed flow
kapi formats list             # List available formats
kapi tools                    # List available tools
kapi flows                    # List available flows
kapi plugins list             # List installed plugins
```

**Kapi with .kapi project files:**

```bash
# Use a .kapi project file for saved workflow defaults
kapi run translate -p myproject.kapi
kapi run translate-and-qa -p myproject.kapi --target-lang de
```

`.kapi` files are portable YAML documents — see [AD-008](docs/architecture-decisions/008-project-model.md). They work with both kapi CLI (`-p` flag) and Kapi (open/edit/save as documents).

**Role Separation:**

- **Kapi** = standalone file-processing tool, demonstrates neokapi's power as open-source toolchain
- **Kapi** = GUI companion for kapi — visual flow editor, runner, plugin manager, credential vault
- **kapi-bowrain plugin** (manifest-driven, dispatched via `kapi`) = project sync companion CLI, focuses on DX and project simplicity for Bowrain
- **Shared CLI base** (`cli/`) = common commands (run, flows, tools, formats, plugins, presets, termbase, version) and top-level tool commands used by both kapi and bowrain
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
- `flow.Executor` — orchestrates tool chains with goroutines and channels
- `registry.FormatRegistry` — factory registry for readers/writers with format detection
- `aiprovider.LLMProvider` — interface for Anthropic, OpenAI, Azure OpenAI, Ollama, Gemini backends (`providers/ai/`)
- `aiprovider.StreamingLLMProvider` — optional extension of LLMProvider with `ChatStream`/`ChatStructuredStream` for live thinking progress (streaming events: thinking, content, done)

### Terminology Mapping from Okapi

| Okapi (Java)                    | neokapi (Go)               |
| ------------------------------- | -------------------------- |
| Filter                          | DataFormat (Reader/Writer) |
| Step                            | Tool                       |
| Pipeline                        | Flow                       |
| PipelineDriver                  | Executor                   |
| Event                           | Part                       |
| TextUnit                        | Block                      |
| TextFragment                    | Fragment                   |
| Code                            | Span                       |
| StartSubDocument/StartSubFilter | Child Layer                |

## Implementing a New Format

Create a package under `core/formats/` with reader.go, writer.go, config.go. The reader must implement `format.DataFormatReader` (embed `format.BaseFormatReader`). The writer must implement `format.DataFormatWriter` (embed `format.BaseFormatWriter`). Register both in `core/formats/register.go` via `init()`. Format packages live in the framework module at the repo root.

## Implementing a New Tool

Create a type embedding `tool.BaseTool` and set `HandleBlockFn` / `HandleDataFn` / `HandleMediaFn` function fields for the part types you want to process. Parts you don't handle pass through unchanged. Register in the tool registry.

## Testing

Tests use `github.com/stretchr/testify` (assert/require). Table-driven tests are the standard pattern. Format tests typically do roundtrip validation (read → write → compare). Test files colocate with implementation (`*_test.go`).

## Screenshots, Recordings & Screencasts

Screenshots and video recordings serve as documentation and are embedded on the website. **Whenever UI-related code changes, all screenshots and recordings must be regenerated** as part of the verification process before committing.

### Screenshot systems

Screenshots are captured via Playwright and written directly to `web/docs/static/img/`:

1. **Bowrain (desktop GUI)** — 9 screenshots x 2 themes in `bowrain/apps/bowrain/frontend/e2e/screenshots.spec.ts`. Self-contained (auto-starts a Vite dev server). Output: `web/docs/static/img/bowrain/{dark,light}/`.
2. **Web app** — 6 test suites (multiple captures each) x 2 themes in `bowrain/apps/web/e2e/screenshots.spec.ts`. Requires a running bowrain-server with Keycloak OIDC. Output: `web/docs/static/img/web-app/{dark,light}/`.

### Recording systems

There are four independent video recording pipelines:

1. **Bowrain (desktop GUI)** — 13 scenarios x 2 themes (dark + light) in `bowrain/apps/bowrain/frontend/e2e/recordings.spec.ts`. Uses real bowrain-server via Wails dev mode for recordings/screenshots. Mocks (`mock-backend.ts`) are used for e2e unit tests only.
2. **Web app** — 8 scenarios x 2 themes (dark + light) in `bowrain/apps/web/e2e/recordings.spec.ts`. Requires a running bowrain-server with Keycloak OIDC.
3. **Kapi CLI** — VHS terminal recordings from `.tape` files in `web/docs/tapes/` (3 standalone kapi demos). No server required.
4. **Bowrain CLI** — VHS terminal recordings from `.tape` files in `bowrain/e2e/tapes/` (10 bowrain demos, some need server).

### How to regenerate

**Locally:**

```bash
# 1. Bowrain screenshots + recordings (self-contained)
make screenshots                 # screenshots → web/docs/static/img/bowrain/{dark,light}/
make recordings                  # recordings → web/docs/static/video/bowrain/{dark,light}/

# 2. Web app screenshots + recordings (needs Keycloak + local server)
docker compose up -d --wait   # starts Keycloak + Mailpit
make dev-server               # builds + starts bowrain-server locally
cd bowrain/apps/web && vp run e2e:screenshots
cd bowrain/apps/web && vp run e2e:recordings
THEME=dark  bash bowrain/apps/web/scripts/copy-recordings.sh
THEME=light bash bowrain/apps/web/scripts/copy-recordings.sh
# Ctrl-C the server, then:
docker compose down -v

# 3. Kapi CLI recordings (no server needed)
make kapi-recordings             # runs tapes + copies to web/docs/static/video/kapi/

# 4. Bowrain CLI recordings (needs VHS + server)
# bowrain-cli-recordings retired with the standalone bowrain binary; the kapi-bowrain plugin
# is exercised via the same tapes through the kapi binary now.

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

All screenshots and recordings must run against real neokapi infrastructure. Specifically:

- **Authentication & identity**: Use the real Keycloak OIDC provider via `compose.yaml`. Never mock the auth flow.
- **bowrain-server**: Use the real server binary (locally built). Never use a mock API server.
- **Database & storage**: Use a real SQLite database (bowrain-server creates one automatically).
- **External integrations** outside the scope of this project (e.g. third-party MT providers, external LLM APIs) may be mocked if needed for isolation.

### Verification checklist for UI changes

Before committing any UI-related change:

1. TypeScript checks pass for all 4 projects (`packages/ui`, `bowrain/apps/web`, `kapi/apps/kapi-web`, `bowrain/apps/bowrain/frontend`)
2. All unit tests pass (`cd packages/ui && vp test`)
3. All 3 frontend production builds succeed
4. All screenshots regenerated to `web/docs/static/img/`
5. All recordings regenerated and copied to `web/docs/static/video/`
6. Go build succeeds (`make build build-server`)

## Writing & Brand Communication

When writing or editing user-facing prose (docs site, landing pages, READMEs,
release notes, CLI help, UI copy), follow
[docs/internals/brand-communication.md](docs/internals/brand-communication.md).
In short: use an academic, restrained register (no marketing superlatives or
emoji); never hardcode counts that the code controls (formats, tools,
providers, filters) — name categories and link to generated references; state
each topic once and cross-link rather than duplicate; and verify every command,
flag, import path, and flow name against the code before publishing.

## Architecture Decisions

ADs live in `docs/ad/`. They are organized by architectural concern (content model, plugin system, Java bridge, etc.), not by chronological order. Each AD should describe the current state of its subsystem as a self-contained document. When a subsystem evolves, update the existing AD in place rather than appending a new one. Only create a new AD when a genuinely new architectural concern is introduced.

Implementation notes live in `docs/notes/`. These contain tactical details (SQL schemas, API routes, algorithm pseudocode) extracted from ADs to keep decisions focused on the WHY and WHAT.
