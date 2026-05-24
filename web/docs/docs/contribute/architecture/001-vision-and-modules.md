---
id: 001-vision-and-modules
sidebar_position: 1
title: "AD-001: Vision and Module Architecture"
description: "Architecture decision: neokapi is an open, AI-native Go localization framework distributed as four independent modules — framework, CLI, kapi CLI, and desktop — coordinated by a go.work workspace and enforced by GOWORK=off CI builds."
keywords: [neokapi, architecture decision, Go modules, go.work, multi-module, Apache-2.0]
---

# AD-001: Vision and Module Architecture

## Summary

neokapi is an open, AI-native localization framework in Go, distributed under
Apache-2.0. It ships as four independent Go modules — framework, shared CLI
base, standalone CLI (kapi), and desktop app (kapi-desktop) — coordinated by a
`go.work` file. Each module has an explicitly declared dependency footprint,
enforced by CI with `GOWORK=off` builds.

## Context

A localization framework has to serve several distinct deployment targets from
one codebase: a standalone file-processing CLI for engineers, a visual desktop
app for localization specialists, and a library that larger platforms can
embed. Each target has a different dependency profile — Wails for the desktop
app, keychain access for credentials, SQLite for local stores — and forcing
every binary to pull every dependency produces slow builds and bloated
artifacts.

At the same time, the framework is the Apache-licensed core of a broader
ecosystem. Platforms layered on top of it (such as the AGPL-3.0 bowrain
platform) must build on framework interfaces without polluting the framework
with platform-specific dependencies. The license boundary is structural, not
conventional: framework code never depends on AGPL code, and CI enforces it.

## Decision

### Identity

neokapi is an open-source localization framework in Go, licensed under
Apache-2.0. It provides format-aware document parsing, a channel-based
concurrent processing engine, and composable tools for translation, quality
assurance, terminology management, and review. Everything is a library and a
toolkit — no database, no server, no authentication. The framework is the
primary vehicle for driving open localization innovation, including format
support, processing tools, and AI integration.

Design principles:

1. **Streaming concurrency.** Documents flow through a pipeline of channels;
   each tool runs in its own goroutine. See
   [AD-004: Processing Engine](004-processing-engine.md).
2. **Content-addressable blocks.** Translatable units are identified by the
   hash of their normalized content plus surrounding context, enabling
   deduplication across sources and incremental processing. See
   [AD-002: Content Model](002-content-model.md) and
   [AD-003: Identity](003-identity.md).
3. **Progressive complexity.** A single CLI command on day one, a YAML flow on
   day two, an integrated project on day ten. The same content model and tool
   chain work at every scale.
4. **AI as first-class pipeline tools.** LLM-powered translation, quality
   assurance, terminology extraction, and review are ordinary pipeline tools
   that participate in the same flow execution model as format-aware tools.
5. **Single-binary distribution.** Go compiles to static binaries. The shared
   codebase produces the `kapi` CLI and the `kapi-desktop` desktop app with no
   JVM, no Node.js runtime, and no container required for basic usage.

### Framework Modules

Four Go modules make up the framework, coordinated by a single `go.work` file
at the repository root:

| Module       | Import path                               | Directory            | Role                                                                                  |
| ------------ | ----------------------------------------- | -------------------- | ------------------------------------------------------------------------------------- |
| Framework    | `github.com/neokapi/neokapi`              | `.` (repo root)      | Content model, formats, tools, pipeline, plugin system, TM, termbase, AI/MT providers |
| CLI          | `github.com/neokapi/neokapi/cli`          | `cli/`               | Shared CLI base: App struct, command factories, output formatting, app config         |
| Kapi         | `github.com/neokapi/neokapi/kapi`         | `kapi/`              | Standalone CLI tool for local file processing                                         |
| Kapi Desktop | `github.com/neokapi/neokapi/kapi-desktop` | `apps/kapi-desktop/` | Wails v3 desktop app for visual localization workflows                                |

The dependency graph is strictly hierarchical:

```
framework
    ↑
    └── cli          (depends on framework only)
         ↑
         ├── kapi    (depends on framework + cli)
         └── kapi-desktop  (depends on framework + cli)
```

### Dependency rules

The following invariants are enforced by CI:

- **Framework** has zero platform dependencies. No SQLite, Wails, Echo, Cobra,
  Viper, OIDC, or keyring imports. The framework is pure library code.
- **CLI** depends only on the framework. No Wails, Echo, OIDC, keyring. CLI
  uses Cobra for command parsing and Viper for config.
- **Kapi** depends on framework + CLI. No heavy dependencies (no Wails, no
  keyring, no OIDC).
- **Kapi Desktop** depends on framework + CLI. The Wails v3 and keyring
  dependencies justify a separate module so that Kapi CLI builds stay small.
- **No framework module depends on any AGPL code.** The framework is
  Apache-2.0 end-to-end.

These rules are verified in CI with `GOWORK=off` builds per module:

```bash
GOWORK=off go build ./...                                    # framework
GOWORK=off bash -c "cd cli && go build ./..."                # cli
GOWORK=off bash -c "cd kapi && go build ./..."               # kapi
GOWORK=off bash -c "cd apps/kapi-desktop && go build ./..."  # kapi-desktop
```

A successful `GOWORK=off` build per module proves that each module's imports
resolve without the workspace — meaning every cross-module import is a real
dependency declared in the module's `go.mod`.

### License boundary

All four framework modules are Apache-2.0. The broader repository also hosts
the AGPL-3.0 bowrain platform, which builds on framework interfaces (content
model, tools, flows, formats), but no framework module ever imports bowrain
code. The license gradient is one-directional:

```
Apache-2.0 framework  →  (consumed by)  →  AGPL-3.0 bowrain platform
```

This is a structural property, not a convention. Because the framework
modules never import bowrain packages, accidental upward coupling is a
compile error.

### Framework package layout

Within the framework module (repo root), packages are grouped by
responsibility:

```
core/
    model/            Content model types (Part, Block, Layer, Fragment, Span)
    format/           DataFormatReader/Writer interfaces, detection
    tool/             Tool interface, BaseTool dispatch
    flow/             Executor, Builder, FlowDefinition
    registry/         FormatRegistry, ToolRegistry
    encoding/         Text encoding utilities
    locale/           BCP-47 locale handling
    editor/           Block index serialization and preview generation
    id/               Short base62 ID generation
    version/          Build version info
    formats/          Built-in format implementations
    tools/            Built-in utility tools
    plugin/           Plugin system: gRPC host, Java bridge, loader, registry
    storage/          Shared SQLite DB infrastructure
    project/          .kapi project file format
    testutil/         Shared test helpers
sievepen/             Translation memory (interface + in-memory + SQLite + matching)
termbase/             Terminology (interface + in-memory + SQLite + import)
providers/
    ai/               package aiprovider — LLM providers + AI tools
    mt/               package mtprovider — MT providers + MT tools
bench/                Benchmarks
examples/             Plugin examples
```

### Workspace and versioning

The root `go.work` file coordinates all four modules for local development:

```
use (
    .
    ./cli
    ./kapi
    ./apps/kapi-desktop
)
```

With the workspace active, changes to framework code are visible to the CLI,
kapi, and kapi-desktop without publishing. `go mod tidy` does not respect
`go.work`, so each child module's `go.mod` carries a `replace` directive
pointing to `../` for the parent modules it depends on.

Modules are tagged independently using Go's module version conventions:

```
v0.16.0              → framework
cli/v0.1.0           → cli
kapi/v0.1.0          → kapi
kapi-desktop/v0.1.0  → kapi-desktop
```

All modules target Go 1.26+. The root Makefile provides per-module build,
test, vet, and lint targets.

### Configuration

The CLI module uses [Viper](https://github.com/spf13/viper) for layered
config, with the following precedence (highest wins):

1. **CLI flags** (via Cobra) — one-off overrides
2. **Environment variables** (`KAPI_*` prefix) — CI/CD and Docker
3. **Project config** (`.kapi` project files) — workflow defaults
4. **User config** (`~/.config/kapi/kapi.yaml`) — personal defaults
5. **Code defaults** — sensible zero-config behavior

Both kapi and kapi-desktop use [Cobra](https://github.com/spf13/cobra) (kapi
directly, kapi-desktop indirectly through the shared CLI base) for
hierarchical subcommands. Kapi operates directly on files (`kapi
pseudo-translate -i file.json`); kapi-desktop wraps the same commands behind
a GUI.

### Locale handling

`model.LocaleID` is a `string` typedef holding BCP-47 tags in canonical form
(`en`, `fr`, `pt-BR`). The `core/locale` package provides validation,
normalization, and display-name resolution:

```go
func Parse(s string) (model.LocaleID, error)
func MustParse(s string) model.LocaleID
func DisplayName(id model.LocaleID) string
func WellKnownLocales() []LocaleInfo
```

BCP-47 validation delegates to `golang.org/x/text/language`, which handles
subtag parsing, script inference, and canonicalization. All subsystems
(format readers, TM entries, terminology, CLI flags) validate locale codes at
their boundaries so invalid codes never propagate silently.

## Consequences

- Kapi CLI binary has no Wails, keyring, SQLite server, or OIDC dependencies;
  it stays small and fast to build.
- CLI module evolves independently of consumer modules — CLI changes do not
  force kapi or kapi-desktop rebuilds of unrelated code.
- Framework packages are organized under `core/`, `sievepen/`, `termbase/`,
  `providers/`, giving a clean separation of concerns at the directory level.
- Four `go.mod` files need maintenance, but `go.work` resolves cross-module
  imports during daily development and GoReleaser handles multi-module release
  builds.
- License-clean: Apache-2.0 framework modules never accidentally pull AGPL
  code, enforced by import-path topology.
- The shared CLI base lets kapi and kapi-desktop expose identical commands
  without duplicating command logic.
- Progressive scalability: the same content model and tool chain works for a
  solo developer running `kapi pseudo-translate` on local files and for a
  team using kapi-desktop with a rich flow editor and plugin manager.

## Related

- [AD-002: Content Model](002-content-model.md) — the Part/Block/Fragment/Span types every module shares
- [AD-003: Identity](003-identity.md) — the short-ID scheme and dual block identity
- [AD-004: Processing Engine](004-processing-engine.md) — the streaming pipeline
- [AD-005: Format System](005-format-system.md) — DataFormatReader/Writer and format detection
- [AD-006: Tool System](006-tool-system.md) — tool interface, parameter schemas, IO contracts
- [AD-007: Plugin System and Okapi Bridge](007-plugin-system.md) — gRPC plugins and the Java bridge
