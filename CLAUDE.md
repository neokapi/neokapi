# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

neokapi is an AI-native reimagining of the [Okapi Framework](https://okapiframework.org/) in Go. It provides format-aware document parsing, channel-based concurrent processing flows, and pluggable tools for localization and translation.

The repository is a **multi-module monorepo** with seven Go modules:

- **Framework** (`github.com/neokapi/neokapi`) — the open-source localization engine: content model, format readers/writers, processing tools, pipeline executor, plugin system, SQLite-backed TM and termbase (`sievepen/`, `termbase/`), shared SQLite infrastructure (`core/storage/`), `.kapi` project file format (`core/project/`), AI providers (`providers/ai/`, package `aiprovider`), MT providers (`providers/mt/`, package `mtprovider`). Framework packages live under `core/`, `sievepen/`, `termbase/`, and `providers/`. No bowrain dependencies (no Wails, Echo, Cobra, OIDC).
- **CLI** (`github.com/neokapi/neokapi/cli`) — shared CLI base used by both kapi and bowrain: App struct, command factories (formats, plugins, tools, flows, presets, termbase, tm, version), output formatting, Viper-based app config, OS keychain credential store (`cli/credentials/`). Uses framework's SQLite TM/termbase from `sievepen/` and `termbase/`. Depends on framework only. No bowrain dependency.
- **Bowrain Core** (`github.com/neokapi/neokapi/bowrain/core`) — shared platform types and interfaces: project model, auth types, connector interfaces, REST client (incl. the Merkle-diff push client), store interfaces, event types, plus the low sync packages the push client needs — `bowrain/core/sync` (pure model↔protobuf converters + content-hash helpers) and `bowrain/core/proto/sync/v1` (the sync-protocol protobuf messages). Depends on framework only (+ `bowrain/plugin/schema`); no CLI dependency (no Cobra, Viper) and **no dependency on the main `bowrain` module** — the redis/store-backed sync engine stays in `bowrain/sync`.
- **Kapi** (`github.com/neokapi/neokapi/kapi`) — primary CLI binary (Apache-2.0). Contains zero vendor-plugin code. Plugins (bowrain, okapi-bridge, …) are discovered at runtime via the unified manifest model (#438) and dispatched as subprocesses. Depends on framework + CLI only.
- **Kapi Desktop** (`github.com/neokapi/neokapi/kapi-desktop`) — Wails v3 desktop app for visual localization workflows. Blank-imports `bowrain/plugin/schema` so it validates bowrain recipes on open. Depends on framework + CLI + bowrain/plugin/schema.
- **Bowrain CLI** (`github.com/neokapi/neokapi/bowrain/cli`) — produces the `kapi-bowrain` manifest-driven plugin binary (in `cmd/kapi-bowrain/`). The plugin is dispatched to by kapi under the unified plugin model. The legacy standalone `bowrain` binary has been retired; all bowrain commands flow through `kapi <command>` once `bowrain-cli` is brew-installed.
- **Bowrain Plugin** (`github.com/neokapi/neokapi/bowrain/plugin`) — Go packages implementing bowrain's behavior: `schema/` (recipe extension decoders, registers via `init()` against `core/project.RegisterExtension`), `commands/` (push, pull, sync, status, init, auth, …, registers via `cli.RegisterCommandFactory`), `connector/` (BowrainSourceConnector), `mcp/` (bowrain MCP tools, registers via `cli.RegisterMCPToolFactory`). These are blank-imported into the `kapi-bowrain` plugin binary; they are no longer imported by the default `kapi` binary. The `schema/` sub-package has its own go.mod so kapi-desktop can blank-import it cheaply.
- **Bowrain Core** (`github.com/neokapi/neokapi/bowrain/core`) — shared bowrain platform types: Recipe wrapper around the framework's `KapiProject` (with type aliases re-exported from `bowrain/plugin/schema`), Project facade, sync converters + content-hash helpers (`bowrain/core/sync`) and the sync-protocol protobuf (`bowrain/core/proto/sync/v1`), REST client, auth, store interfaces, event types. The redis-backed sync hash cache and the store-backed diff engine live in the main bowrain module (`bowrain/sync`), which re-exports the pure converters so server-side callers keep one import.
- **Bowrain** (`github.com/neokapi/neokapi/bowrain`) — the full-stack localization platform: REST server, desktop app, web app, connectors, persistent SQLite/PostgreSQL storage. Depends on framework + bowrain/core.
- **SaT plugin** (`github.com/neokapi/neokapi/plugins/sat`) — the `kapi-sat` segmenter plugin: runs wtpsplit *Segment any Text* ONNX models in-process (cgo onnxruntime + XLM-RoBERTa tokenizer, gated behind `-tags onnx`) and speaks a line-delimited JSON segmentation protocol on stdin/stdout. Isolated so its native ML stack never enters the `kapi` binary; the CLI's `sat` segment engine (`cli/segment_sat.go`) discovers and drives it. The pure-Go `satproto` package + protocol/algorithm tests build with no native deps.

Both **kapi** and **bowrain** binaries share a common base in `cli/`. The base provides core commands (run, extract, merge, flows, tools, formats, plugins, presets, termbase, tm, mcp, version) plus four plugin registries: `cli.RegisterCommandFactory`, `cli.RegisterAppInitializer`, `cli.RegisterMCPToolFactory` (CLI-side), and `core/project.RegisterExtension` (framework, for recipe schema). Plugins blank-imported into a binary contribute commands, MCP tools, and recipe schema via `init()` registration. See [Note: Plugin model](web/docs/contribute/notes-internal/plugin-model.md).

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

> **Note:** A single root pnpm workspace (`pnpm-workspace.yaml`) coordinates all frontend
> packages (`packages/ui`, `packages/flow-editor`,
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

**Bowrain web app (SaaS UI served by bowrain-server):**

```bash
make -C bowrain web-deps                      # vp install for the web app
make -C bowrain web-build                     # Build web app → bowrain/apps/web/dist/
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
cd web && vp run start           # Dev server with hot reload
cd web && vp run build           # Production build → web/build/
```

## Build Conventions

Always prefer `make` targets over raw `go build` / `go test` commands. The Makefile handles prerequisites (e.g. `make proto` regenerates gRPC code before a build that needs it) and places binaries in `bin/` rather than the repo root. Use direct `go test` only when targeting a specific package or test function.

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

## Dogfooding kapi (in-repo isolation contract)

This repo dogfoods kapi: a `*.kapi` recipe lives at the repo root and is driven
by the **system/user-installed** kapi + plugins (the real `kapi-bowrain` plugin,
real keychain auth, real server). That recipe is auto-discovered by a git-style
**upward walk** from any cwd inside the tree (`core/project.ResolveLayout` →
`cli.ResolveProjectPath`), so the dogfood project must never leak into the
project's own tests, scripts, or docs recorders.

**The contract: every in-repo kapi invocation that is _not_ the dogfood
workflow must isolate itself.** Set, on the kapi process environment:

- `KAPI_NO_PROJECT=1` — opt out of project discovery (an explicit `-p` still
  wins). **Note:** `KAPI_PROJECT=""` does *not* disable discovery; only a
  non-empty `KAPI_NO_PROJECT` does.
- `KAPI_CONFIG_DIR`, `XDG_DATA_HOME`, `XDG_CACHE_HOME` → throwaway dirs, so kapi
  can't read the developer's `~/.config/kapi`, user-installed plugins, or caches.
- `KAPI_PLUGINS_DIR_ONLY=1` — discover plugins *only* from `$KAPI_PLUGINS_DIR`
  (empty by default → none), skipping the user (XDG) **and** system (Homebrew,
  `/usr/share`) plugin roots. `XDG_DATA_HOME` alone only isolates the user root,
  so without this an in-repo kapi still picks up Homebrew-installed plugins.
  Point `KAPI_PLUGINS_DIR` at a repo-local dir when a dogfood scenario needs one.

Where this is already wired:

- **Makefile** — use the shared `$(KAPI_ISO_ENV)` (defined near the top) to
  prefix any in-repo `bin/kapi` call (e.g. the `kapi-*-pseudo-translate`
  targets): it applies config isolation and adds `KAPI_NO_PROJECT=1` for
  invocations that don't own a `*.kapi` fixture (those that do keep discovery on
  and rely on nearest-recipe-wins).
- **`kapi/e2e`** — `TestMain` builds with `-tags fts5` and pins an isolated
  config/data/cache home with `KAPI_NO_PROJECT=1` (see `isoEnv`).
- **harness/** — already safe: its sandboxes live in `os.tmpdir()` (outside the
  repo) and it sets `XDG_DATA_HOME` / `KAPI_CONFIG_DIR` via `kapiIsolationEnv()`.

When adding a new in-repo kapi invocation, follow this contract or it may
silently bind to (and act on) the dogfood project.

## Architecture

### Multi-Module Structure

```
neokapi/
├── go.work                # Workspace: framework, cli, kapi, apps/kapi-desktop, the bowrain/* modules (incl. plugin & plugin/schema), and the scripts/* tooling modules
├── go.mod                 # module github.com/neokapi/neokapi (framework, Apache-2.0)
│
│   ── Framework Module (repo root) ──────
├── core/
│   ├── model/             # Content model (Part, Block, Run, Target, Overlay, Layer)
│   ├── format/            # DataFormatReader/Writer interfaces
│   ├── tool/              # Tool interface
│   ├── flow/              # Executor, pipeline orchestration
│   ├── registry/          # Format and tool registries
│   ├── encoding/          # Character encoding detection/conversion
│   ├── locale/            # BCP-47 locale utilities
│   ├── editor/            # Block index serialization and preview generation
│   ├── version/           # Version info (set via ldflags)
│   ├── formats/           # Built-in format implementations (one package per format)
│   ├── storage/           # Shared SQLite DB infrastructure (Open, Migrate)
│   ├── project/           # .kapi project file format (Load, Save, Validate)
│   ├── tools/             # Built-in utility tools
│   ├── plugin/            # go-plugin + gRPC plugin system + Java bridge
│   └── internal/testutil/ # Shared test helpers (RawDocFromString, CollectBlocks, …)
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
│   └── preset/            # Built-in preset definitions
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
├── package.json           # Root package.json; workspace members live in pnpm-workspace.yaml
├── .npmrc                 # pnpm registry/auth config (behavioral settings live in pnpm-workspace.yaml)
├── storybook/             # Kapi Storybook config (port 6007, aggregates packages/ui + flow-editor + kapi-desktop)
├── packages/
│   ├── ui/                # @neokapi/ui-primitives — shadcn/ui primitives consumed by kapi-desktop and bowrain apps
│   ├── flow-editor/       # @neokapi/flow-editor — shared React flow editor component library
│   └── storybook-config/  # @neokapi/storybook-config — shared Storybook preview/main factories
│
│   ── Non-Go Assets ─────────────────────
├── docs/                  # Architecture decisions, implementation notes
├── web/               # Docusaurus site
└── Makefile               # Multi-module build targets
```

### Bowrain Project Model (`.kapi` Recipe + State Dir)

Bowrain CLI uses the framework's unified `.kapi` project model — a `<dir-name>.kapi` recipe at the project root with a `server:` block, plus a sibling `.kapi/` state directory ([Bowrain AD-010](bowrain/web/docs/docs/architecture-decisions/010-bowrain-cli-and-project-model.md)):

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
│       ├── sync-cache.json  # kapi push/pull state
│       ├── extractions/
│       └── collections/
├── src/
│   └── locales/
│       ├── en-US.json
│       └── fr-FR.json
```

A bowrain project is just a kapi project whose recipe declares a `server:` block (compound URL, optional `stream`). Top-level recipe fields cover `defaults`, `content`, `plugins` (map form), `flows`, `hooks`, `automations`, `assets`, `brand_voice`. Auth tokens live in the OS keychain (`bowrain-auth:<server-url>`, `bowrain-refresh:<server-url>`); non-secret metadata sits at `~/.config/bowrain/auth.json`. `BOWRAIN_AUTH_TOKEN` env var works in CI.

**Key bowrain plugin commands (run via `kapi` once the `kapi-bowrain` plugin is installed):**

```bash
kapi init                       # Write <dir-name>.kapi + .kapi/ state dir
kapi status                     # Show sync state (like git status)
kapi pull                       # Fetch from Bowrain Server → update local files
kapi push                       # Send local files → update Bowrain Server
kapi run <flow-name>            # Execute flow (inline on recipe or .kapi/flows/)
```

**All bowrain plugin commands require a `.kapi` project with a `server:` block.** The CLI searches upward from the current directory (like git) to find the recipe.

**Key kapi CLI commands (standalone, no project needed):**

```bash
kapi ai-translate -i file.xliff --target-lang fr    # Translate with AI
kapi pseudo-translate file.json # Pseudo-translate for testing
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

`.kapi` files are portable YAML documents — see [AD-008](web/docs/contribute/architecture/008-project-model.md). They work with both kapi CLI (`-p` flag) and Kapi (open/edit/save as documents).

**Role Separation:**

- **Kapi** = standalone file-processing tool, demonstrates neokapi's power as open-source toolchain
- **Kapi** = GUI companion for kapi — visual flow editor, runner, plugin manager, credential vault
- **kapi-bowrain plugin** (manifest-driven, dispatched via `kapi`) = project sync companion CLI, focuses on DX and project simplicity for Bowrain
- **Shared CLI base** (`cli/`) = common commands (run, flows, tools, formats, plugins, presets, termbase, version) and top-level tool commands used by both kapi and bowrain
- **Bowrain Server** = integration platform (CMS connectors, automation, ContentStore)
- **Bowrain desktop app** (`bowrain/apps/bowrain/`) = a real-time **working copy of the server**, not a local-file/project authoring tool. Its local footprint is cache and speed only — a content cache, an offline edit queue, and TM/termbase mirrors — and is never a source of truth. It offers only **remote/CMS connectors** (wordpress, figma, hubspot); the local-filesystem connectors (file, git) are registered **server-side only** (`bowrain/connector.RegisterAll` for the server/worker vs `RegisterRemote` for the desktop). Sourcing from a filesystem or git checkout is a server-side concern.

**Product boundary (canonical):** kapi owns local files + project configuration — the `.kapi` recipe (content/flows/plugins/languages/brand + `server:` block) is authored and versioned locally with kapi, including configuring projects pushed to Bowrain via `kapi push` / `kapi sync`. Bowrain's local footprint is cache/speed/implementation only — never source of truth.

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
- **Block** — translatable content: a flat `Source []Run`, `Targets map[VariantKey]*Target` (variant = locale + optional tone/channel), and stand-off `Overlays` (segmentation, terms, entities, QA, alignment) anchored to run-index ranges. There is no structural `Segment` type — segmentation is an opt-in overlay (AD-002).
- **Run** — the inline unit: a discriminated union (Text, Ph, PcOpen/PcClose, Sub, Plural, Select). Inline markup lives in runs, not in the text.
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
| TextFragment                    | Run sequence (`[]Run`)     |
| Code                            | Run                        |
| StartSubDocument/StartSubFilter | Child Layer                |

## Implementing a New Format

Create a package under `core/formats/` with reader.go, writer.go, config.go. The reader must implement `format.DataFormatReader` (embed `format.BaseFormatReader`). The writer must implement `format.DataFormatWriter` (embed `format.BaseFormatWriter`). Register both in `core/formats/register.go` via `init()`. Format packages live in the framework module at the repo root.

## Implementing a New Tool

Create a type embedding `tool.BaseTool`. For Blocks, set exactly one capability-typed handler — `Annotate(BlockView)` (read-only; writes overlays/annotations/properties), `Translate(TargetView)` (writes target), or `Transform(SourceView)` (rewrites source) — the view type bounds what the tool may write (immutability model, AD-006). Other Part types use the untyped `HandleDataFn` / `HandleMediaFn` / `Handle{Layer,Group}{Start,End}Fn` fields. Parts you don't handle pass through unchanged. A tool that needs batching, 1→N fan-out, or stream control overrides `Process` instead. Register in the tool registry. Source-transform (`Transform`) tools belong in a flow's leading source-transform stage, which settles the source before annotation/translation.

## Testing

Tests use `github.com/stretchr/testify` (assert/require). Table-driven tests are the standard pattern. Format tests typically do roundtrip validation (read → write → compare). Test files colocate with implementation (`*_test.go`).

## Documentation Assets (Walkthrough Videos)

Walkthrough videos serve as documentation and are embedded on the website. **Whenever UI- or CLI-surface code changes, re-record the affected walkthrough videos** as part of the verification process before committing.

Videos are produced by the **harness** (`harness/`): each demo is an authored `demo.yaml` — a real kapi/bowrain command sequence or a UI flow — that the harness drives against real infrastructure, screencasts, narrates (TTS), and renders with Remotion into theme-matched light + dark `.webm` files. Published videos land under `web/static/video/` (kapi) and `bowrain/web/docs/static/video/` (bowrain); the MDX wires them in with `ThemedVideo` / `KapiPlayground` embeds. (The interactive in-browser explorers are a separate system — `{id}.scene.yaml` specs driving the WASM engine — not videos.)

### How to regenerate

```bash
make harness-videos-staged        # full pass: stack up → seed → record → narrate → package (light + dark)
make harness-videos               # render the kapi demo videos from existing captures
make publish-docs-assets          # publish web/static/{img,video} → docs-assets release (merges, never drops)
make publish-bowrain-docs-assets  # publish bowrain/web/docs/static/{img,video} → bowrain-docs-assets release
make fetch-docs-assets            # download already-built assets from the docs-assets release
```

See `harness/` (and its Makefile) for the phased seed → record → narrate → package pipeline; bring the stack up once and re-render freely.

### In CI

The docs build workflows (`.github/workflows/docs-kapi.yml`, `docs-bowrain.yml`) **stage** the `.webm` assets from the `docs-assets` / `bowrain-docs-assets` GitHub releases rather than recording in CI — recording happens on the desktop and is pushed to a release via the `publish-*-docs-assets` targets. Assets are not stored in git.

### Real systems, not mocks

All screenshots and recordings must run against real neokapi infrastructure. Specifically:

- **Authentication & identity**: Use the real Keycloak OIDC provider via `compose.yaml`. Never mock the auth flow.
- **bowrain-server**: Use the real server binary (locally built). Never use a mock API server.
- **Database & storage**: Use a real SQLite database (bowrain-server creates one automatically).
- **External integrations** outside the scope of this project (e.g. third-party MT providers, external LLM APIs) may be mocked if needed for isolation.

### Verification checklist for UI changes

Before committing any UI-related change:

1. TypeScript checks pass for the frontend projects (`packages/ui`, `bowrain/apps/web`, `bowrain/apps/bowrain/frontend`, `apps/kapi-desktop/frontend`)
2. All unit tests pass (`cd packages/ui && vp test`)
3. All frontend production builds succeed
4. Affected walkthrough scenes re-recorded (see the walkthrough/scenes engine below) and assets land under `web/static/`
5. Go build succeeds (`make build build-server`)

## Writing & Brand Communication

When writing or editing user-facing prose (docs site, landing pages, READMEs,
release notes, CLI help, UI copy), follow
[docs/internals/brand-communication.md](docs/internals/brand-communication.md).
In short: use an academic, restrained register (no marketing superlatives or
emoji); never hardcode counts that the code controls (formats, tools,
providers, filters) — name categories and link to generated references; state
each topic once and cross-link rather than duplicate; and verify every command,
flag, import path, and flow name against the code before publishing.

### Diagrams in docs: real React, not ASCII

Documentation diagrams must be **real React diagram-kit components**, never ASCII
art in a code fence. The themed, light/dark SVG kit lives in
`@neokapi/docs-shared` (`packages/docs-shared/src/diagram/`): `PipelineDiagram`,
`StreamDiagram`, `PhaseFlow`, `RoundTripDiagram`, `LanesDiagram`,
`SwimlaneDiagram`, `ArchitectureDiagram`, and `RedactionDiagram` (censor-bar
blackout). Import the component into the `.md`/`.mdx` page and pass the data as
props. Every component has a story under **Diagrams** in the Kapi Storybook
(`packages/docs-shared/src/diagram/*.stories.tsx`) — add or reuse one there when
you introduce a diagram, and check it renders in both themes. ASCII code fences
are only for *code*: CLI output, file/directory trees, and config snippets — not
for flows, sequences, or relationships.

## Architecture Decisions

ADs live in `web/docs/contribute/architecture/`. They are organized by architectural concern (content model, plugin system, Java bridge, etc.), not by chronological order. Each AD should describe the current state of its subsystem as a self-contained document. When a subsystem evolves, update the existing AD in place rather than appending a new one. Only create a new AD when a genuinely new architectural concern is introduced.

Implementation notes live in `web/docs/contribute/notes-internal/`. These contain tactical details (SQL schemas, API routes, algorithm pseudocode) extracted from ADs to keep decisions focused on the WHY and WHAT.
