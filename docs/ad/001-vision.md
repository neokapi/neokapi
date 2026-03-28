---
id: 001-vision
sidebar_position: 1
title: "AD-001: Vision — The Open Localization Platform"
---
# AD-001: Vision — The Open Localization Platform

## Vision

**neokapi** is an open-source localization framework in Go, licensed under
Apache 2.0. It provides format-aware document parsing, a channel-based
concurrent processing engine, and composable tools for translation, quality
assurance, terminology management, and review. neokapi is the foundation — a
library and toolkit with zero platform dependencies. As the Apache-licensed
core, neokapi is the primary vehicle for driving open localization innovation,
including format support, processing tools, and AI integration.

**Bowrain** is a localization platform built on neokapi. It adds a versioned
content store, bidirectional connectors to live systems (CMS, design tools,
code repositories, marketing platforms), event-driven automation, multi-user
workspaces, and collaborative editing. Content flows into the store, gets
processed by neokapi's composable tools, and flows back to its source system.
Bowrain is dual-licensed under AGPL and a commercial license, available as a
SaaS offering or for open-source self-hosting.

**The boundary:** neokapi handles content models, format parsing, tool
execution, and pipeline orchestration. Bowrain handles persistence, sync,
connectors, authentication, and multi-user collaboration. neokapi has no
database, no server, no authentication. Bowrain depends on neokapi but not
the reverse.

Two CLIs demonstrate this separation:
- **Kapi** (`kapi` binary) — standalone file-processing tool that demonstrates
  neokapi's capabilities. No project directory, no server, no sync. Processes
  files directly.
- **Bowrain CLI** (`bowrain` binary) — project-centric sync companion for
  Bowrain Server. Manages `.bowrain/` project directories, syncs with the
  server via push/pull, runs flows within a project context.

### Architectural Principles

1. **Connector-first** — Connectors to live systems (CMS, design tools, code
   repos, marketing platforms) are the primary integration mechanism. File
   formats are one connector type (`FileConnector`), not the whole story.
   Each connector implements bidirectional sync: pull content into the store,
   push translations back.

2. **Content-addressable** — Blocks are identified by content hash, enabling
   deduplication across sources, efficient diffing between versions, and
   incremental sync that only processes what changed.

3. **Progressive complexity** — Import a CSV glossary or run a single CLI
   command on day one. Grow into flows, automation rules, and team
   collaboration as needs evolve. The same content model and tool chain
   works at every scale.

4. **Single binary** — Go compiles to static binaries. The shared codebase
   produces the `kapi` CLI, the `bowrain` CLI, the `bowrain-server`
   REST/gRPC server, and the Bowrain desktop app (via Wails v3). No JVM,
   no Node.js runtime, no container required for basic usage.

5. **AI-native** — LLM-powered translation, quality assurance, terminology
   extraction, and review are composable pipeline tools
   ([AD-008](./008-ai-integration.md)), not bolted-on services. They
   participate in the same flow execution model as every other tool
   ([AD-004](./004-processing-engine.md)).

6. **Open and extensible** — Plugins via gRPC
   ([AD-007](./007-plugin-system.md)), the Okapi bridge for 40+ additional
   filters, and community connectors. The content model is public; anyone
   can build integrations.

### Why Go

- **Single static binary** — `go build` produces a self-contained executable
  with no runtime dependencies. This enables `curl | install` distribution,
  minimal Docker images, and desktop apps without bundling a JVM or Node.js.

- **Goroutines and channels** — The streaming pipeline
  ([AD-004](./004-processing-engine.md)) runs each tool in its own
  goroutine, connected by buffered channels. This maps directly to Go's
  concurrency primitives, providing natural backpressure and cancellation
  propagation via `context.Context`.

- **Strong typing with interfaces** — Clean abstraction boundaries between
  formats, tools, flows, and connectors. Interface satisfaction is implicit,
  keeping implementations decoupled.

- **Cross-compilation** — A single `GOOS`/`GOARCH` flag produces binaries for
  macOS, Linux, and Windows. CI builds all three from one pipeline.

### Content Model Concepts

The content model ([AD-002](./002-content-model.md)) uses the following
core types:

| Concept | Description |
|---|---|
| Part | Fundamental streaming unit with a PartType discriminator |
| Layer | Structural grouping (document, section, embedded content) |
| Block | Translatable content with source and target segments per locale |
| Fragment | Text with inline Spans using coded text markers |
| Data | Non-translatable structure |
| Media | Binary content |

Parts flow through a channel-based pipeline
([AD-004](./004-processing-engine.md)) where each tool runs in its own
goroutine.

### Package Layout

The project is a multi-module monorepo with six Go modules coordinated by
`go.work`:

- **Framework** (`github.com/neokapi/neokapi`) — the open-source localization engine
- **CLI** (`github.com/neokapi/neokapi/cli`) — shared CLI base for kapi and bowrain
- **Platform** (`github.com/neokapi/neokapi/platform`) — shared platform types (project model, auth, connectors)
- **Kapi** (`github.com/neokapi/neokapi/kapi`) — standalone CLI for local file processing
- **Bowrain CLI** (`github.com/neokapi/neokapi/bowrain-cli`) — project sync companion CLI
- **Bowrain** (`github.com/neokapi/neokapi/bowrain`) — full-stack localization platform

```
neokapi/                    ── Framework Module (open-source, Apache 2.0) ──
  core/
    model/          content model types (Part, Block, Layer, Fragment, Span)
    format/         DataFormatReader/Writer interfaces, detection
    tool/           Tool interface, BaseTool dispatch
    flow/           FlowExecutor, FlowBuilder, FlowDefinition
    registry/       FormatRegistry, ToolRegistry
    encoding/       text encoding utilities
    locale/         BCP-47 locale handling
    editor/         block index serialization and preview generation
    version/        build version info
    formats/        built-in format implementations (15 formats)
    ai/             LLM providers + AI tools (translate, QA, terminology, review, entity extraction)
    mt/             MT providers + MT translate tool
    sievepen/       translation memory interface + in-memory impl
    termbase/       concept-oriented terminology interface + in-memory impl
    tools/          utility tools (wordcount, pseudo-translation, term lookup, etc.)
    plugin/         plugin system: gRPC host, Java bridge, loader, registry
    testutil/       shared test helpers

cli/                       ── CLI Module ──
  config/           Viper-based app configuration (~/.config/kapi/)
  output/           shared output formatting + types

platform/                  ── Platform Module ──
  project/          .bowrain/ project model (types, config, sync cache)
  auth/             auth types, JWT, device flow client
  connector/        connector interfaces + base types
  client/           REST client for Bowrain API
  config/           auth persistence
  store/            ContentStore interface + domain types
  event/            event types + bus interface
  credentials/      provider credential management

kapi/                      ── Kapi Module (standalone CLI) ──
  cmd/kapi/         thin root cmd wiring shared CLI commands
  apps/kapi-web/    kapi serve web UI

bowrain-cli/               ── Bowrain CLI Module ──
  cmd/bowrain/      project commands + shared CLI base

bowrain/                   ── Bowrain Module (platform) ──
  auth/             OIDC, AuthStore, SQLite + PostgreSQL auth
  connector/        concrete connector implementations (file, git, etc.)
  store/            SQLite + PostgreSQL ContentStore implementations
  server/           HTTP/gRPC server handlers
  service/          auth, project, connector, flow services
  event/            event bus implementation + automation
  credentials/      keyring-backed credentials
  sievepen/         SQLite + PostgreSQL TM implementation
  termbase/         SQLite + PostgreSQL TermBase implementation
  cmd/bowrain-server/  Echo v4 REST API server + gRPC services
  apps/bowrain/     Wails v3 desktop app (Go + React/TypeScript)
  apps/web/         SaaS web UI

packages/ui/       shared React component library
website/           Docusaurus 3 documentation site
```

**Dependency boundaries:**
- Framework has zero platform dependencies (no SQLite, Wails, Echo, Cobra, OIDC)
- CLI depends on framework only
- Platform depends on framework only (no CLI dependency)
- Kapi depends on framework + CLI (no platform, no heavy dependencies)
- Bowrain CLI depends on framework + CLI + platform
- Bowrain depends on framework + platform (no CLI dependency)

### Configuration

Configuration uses [Viper](https://github.com/spf13/viper) for layered
merging with the following precedence (highest to lowest):

1. **CLI flags** (via Cobra) — one-off overrides
2. **Environment variables** (`KAPI_*` / `BOWRAIN_*` prefix) — CI/CD and Docker
3. **Project config** (`.bowrain/config.yaml`, Bowrain CLI only) — team-shared, committed to repo
4. **User config** (`~/.config/kapi/kapi.yaml`) — personal defaults
5. **Code defaults** — sensible zero-config behavior

Both CLIs use [Cobra](https://github.com/spf13/cobra) for hierarchical
subcommands. Kapi operates directly on files (`kapi pseudo-translate
-i file.json`). Bowrain CLI operates within a project context
(`bowrain pseudo-translate`). Viper's automatic env binding means
`BOWRAIN_TOOLS_AI_TRANSLATION_MODEL` overrides the nested YAML path
`tools.ai-translation.model`.

### Locale Handling

`model.LocaleID` is a `string` typedef holding BCP-47 tags in canonical
form (e.g., `en`, `fr`, `pt-BR`). The `locale` package provides
validation, normalization, and display name resolution:

```go
func Parse(s string) (model.LocaleID, error)   // validate + normalize
func MustParse(s string) model.LocaleID         // panics on invalid
func DisplayName(id model.LocaleID) string      // "French", "German"
func WellKnownLocales() []LocaleInfo            // curated list for UI
```

BCP-47 validation delegates to `golang.org/x/text/language`, which handles
subtag parsing, script inference, and canonicalization. `WellKnownLocales()`
returns a curated list of ~50 common tags sorted by display name, powering
Bowrain's searchable locale selector dropdowns.

All subsystems validate locale codes at their boundaries:

| Subsystem | Validation point |
|---|---|
| CLI flags (`--source-lang`, `--target-lang`) | Cobra argument parsing |
| Project config ([AD-016](./016-kapi-project-model.md)) | Project initialization |
| TM entries ([AD-009](./009-translation-memory.md)) | Entry insert/query |
| Terminology ([AD-010](./010-terminology.md)) | Term creation |
| Bowrain project creation ([AD-012](./012-bowrain.md)) | Locale selector component |
| Format readers/writers | Source/target language properties |

## Alternatives Considered

- **Electron-based desktop app** — Heavy runtime (~200 MB) conflicts with the
  single-binary principle. Wails v3 uses the OS native webview, producing a
  ~15 MB binary with native performance.

- **TOML or JSON for configuration** — YAML supports comments (critical for
  team-shared config files) and is more human-friendly for nested structures.
  Viper handles the complexity of layered merging across multiple sources.

- **Custom locale system** — BCP-47 is the industry standard. The
  `golang.org/x/text/language` package handles the full complexity of subtag
  parsing and canonicalization; reimplementing it would be error-prone and
  wasteful.

- **Microservices architecture** — Premature for the current stage. The
  single-binary approach lets the platform grow from local CLI to team server
  without architectural changes.

## Consequences

- **Single-binary distribution** — `go build` produces CLI, server, and
  desktop app from one codebase. No runtime dependencies for end users.

- **Connectors are the primary integration** — Content flows from live systems
  through the platform and back. File-based workflows are supported but not
  privileged.

- **Content-addressable storage** — Deduplication across sources, efficient
  version diffing, and incremental processing that skips unchanged content.

- **AI as first-class pipeline tools** — Translation, QA, terminology
  extraction, and review participate in the same flow execution model as
  format-aware tools.

- **Layered configuration** — Works in CLI scripts, CI/CD pipelines, and
  team settings via YAML files with environment variable overrides.

- **Validated locale codes** — BCP-47 tags are normalized early; Bowrain
  shows friendly display names. No silent propagation of invalid codes.

- **Progressive scalability** — The same content model and tool chain works
  for a solo developer running `kapi pseudo-translate` on local files and a team
  using Bowrain with connectors, automation, and collaborative editing.
