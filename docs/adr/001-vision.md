---
id: 001-vision
sidebar_position: 1
title: "ADR-001: Vision — The Open Localization Platform"
---
# ADR-001: Vision — The Open Localization Platform

## Vision

Gokapi is an open platform for localization. It connects CMS platforms, design
tools, code repositories, and marketing systems through bidirectional
connectors. Content flows into a versioned store, gets processed by composable
tools (translation, QA, terminology enforcement), and flows back to its source
system.

File-based workflows are fully supported — the `FileConnector` treats local
files and format filters as just another integration path — but the platform
is designed around the assumption that most production content lives in systems,
not in files on disk.

The architecture draws inspiration from
[Speckle](https://speckle.systems/) — the open data platform for AEC — where
bidirectional connectors pull data from native tools into a versioned object
graph, and collaboration, automation, and diffing emerge naturally from the
data model.

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

4. **Single binary** — Go compiles to one static binary. The same codebase
   produces the `kapi` CLI, the `bowrain-server` REST/gRPC server, and the
   Bowrain desktop app (via Wails v3). No JVM, no Node.js runtime, no
   container required for basic usage.

5. **AI-native** — LLM-powered translation, quality assurance, terminology
   extraction, and review are composable pipeline tools
   ([ADR-008](./008-ai-integration.md)), not bolted-on services. They
   participate in the same flow execution model as every other tool
   ([ADR-004](./004-processing-engine.md)).

6. **Open and extensible** — Plugins via gRPC
   ([ADR-007](./007-plugin-system.md)), a Java bridge for Okapi filters,
   and community connectors. The content model is public; anyone can build
   integrations.

### Why Go

- **Single static binary** — `go build` produces a self-contained executable
  with no runtime dependencies. This enables `curl | install` distribution,
  minimal Docker images, and desktop apps without bundling a JVM or Node.js.

- **Goroutines and channels** — The streaming pipeline
  ([ADR-004](./004-processing-engine.md)) runs each tool in its own
  goroutine, connected by buffered channels. This maps directly to Go's
  concurrency primitives, providing natural backpressure and cancellation
  propagation via `context.Context`.

- **Strong typing with interfaces** — Clean abstraction boundaries between
  formats, tools, flows, and connectors. Interface satisfaction is implicit,
  keeping implementations decoupled.

- **Cross-compilation** — A single `GOOS`/`GOARCH` flag produces binaries for
  macOS, Linux, and Windows. CI builds all three from one pipeline.

### Content Model Concepts

The content model ([ADR-002](./002-content-model.md)) uses the following
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
([ADR-004](./004-processing-engine.md)) where each tool runs in its own
goroutine.

### Package Layout

```
gokapi/
  core/           model types, format/tool/flow interfaces, registry,
                  config, encoding, locale, store
  formats/        built-in format implementations (15 formats: html, xml,
                  xliff, xliff2, json, yaml, po, properties, plaintext,
                  markdown, csv, srt, vtt, tmx, etc.)
  ai/             LLM provider interface (Anthropic, OpenAI, Ollama) and
                  AI-powered tools (translate, QA, terminology, review)
  connectors/     bidirectional system connectors (CMS, design, code,
                  marketing, file, translation service)
  lib/sievepen/   translation memory with Levenshtein fuzzy matching,
                  TMX import/export
  lib/termbase/   concept-oriented terminology management, TBX-inspired
  lib/tools/      utility tools (wordcount, charcount, pseudo-translation,
                  search/replace, term lookup, term enforce)
  plugin/         plugin system: gRPC host, Java bridge, loader, registry
  cmd/kapi/       Cobra CLI (convert, translate, extract, merge, flow,
                  formats, tools, plugins, termbase)
  cmd/bowrain-server/  Echo v4 REST API server + gRPC services
  apps/bowrain/   Wails v3 desktop app (Go backend + React 19/TypeScript/
                  Vite frontend)
  website/        Docusaurus 3 documentation site
```

### Configuration

Configuration uses [Viper](https://github.com/spf13/viper) for layered
merging with the following precedence (highest to lowest):

1. **CLI flags** (via Cobra) — one-off overrides
2. **Environment variables** (`KAPI_*` prefix) — CI/CD and Docker
3. **Project config** (`./kapi.yaml`) — team-shared, committed to repo
4. **User config** (`~/.config/kapi/kapi.yaml`) — personal defaults
5. **System config** (`/etc/kapi/kapi.yaml`) — organization defaults
6. **Code defaults** — sensible zero-config behavior

```yaml
source_lang: en
target_langs: [fr, de, ja]

formats:
  html:
    preserveWhitespace: false
    extractAltText: true
  json:
    extractStringsOnly: true

tools:
  ai-translation:
    provider: anthropic
    model: claude-sonnet-4-5-20250929
  term-enforce:
    action: warn  # warn | reject | fix

connectors:
  wordpress:
    url: https://example.com/wp-json
    auth: env:WORDPRESS_TOKEN
```

The CLI uses [Cobra](https://github.com/spf13/cobra) for hierarchical
subcommands (`kapi convert`, `kapi flow run`, `kapi plugins install`,
`kapi termbase import`). Viper's automatic env binding means
`BOWRAIN_TOOLS_AI_TRANSLATION_MODEL` overrides the nested YAML path
`tools.ai-translation.model`.

### Locale Handling

`model.LocaleID` is a `string` typedef holding BCP-47 tags in canonical
form (e.g., `en`, `fr`, `pt-BR`). The `core/locale` package provides
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
| KAZ manifest ([ADR-003](./003-content-store.md)) | Archive open/create |
| TM entries ([ADR-009](./009-translation-memory.md)) | Entry insert/query |
| Terminology ([ADR-010](./010-terminology.md)) | Term creation |
| Bowrain project creation ([ADR-012](./012-bowrain.md)) | Locale selector component |
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
  for a solo translator running `kapi translate` and a team running the
  server with connectors, automation, and collaborative editing.
