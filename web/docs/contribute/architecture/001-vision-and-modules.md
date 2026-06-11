---
id: 001-vision-and-modules
sidebar_position: 1
title: "AD-001: Vision and Module Architecture"
description: "Architecture decision: neokapi is an open, AI-native Go localization framework distributed as four independent modules — framework, CLI, kapi CLI, and desktop — coordinated by a go.work workspace and enforced by GOWORK=off CI builds."
keywords: [neokapi, architecture decision, Go modules, go.work, multi-module, Apache-2.0]
---

import { PipelineDiagram } from "@neokapi/docs-shared";

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
ecosystem. Separately-licensed platforms layered on top of it must build on
framework interfaces without polluting the framework with platform-specific
dependencies. The license boundary is structural, not conventional: framework
code never depends on separately-licensed platform code, and CI enforces it.

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

<PipelineDiagram
  channelLabel=""
  caption="Each module depends only on those to its left; CI enforces the boundaries."
  stages={[
    { label: "framework", sub: "core/ · no platform deps", role: "io" },
    { label: "cli", sub: "shared CLI base" },
    {
      lanes: [{ label: "kapi" }, { label: "kapi-desktop" }],
      parallelLabel: "depend on framework + cli",
    },
  ]}
/>

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
- **No framework module depends on any separately-licensed platform code.**
  The framework is Apache-2.0 end-to-end.

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
a separately-licensed platform, which builds on framework interfaces (content
model, tools, flows, formats), but no framework module ever imports platform
code. The license gradient is one-directional:

<PipelineDiagram
  channelLabel="consumed by"
  stages={[
    { label: "Apache-2.0 framework", sub: "core · cli · kapi · desktop", role: "io" },
    { label: "Separately-licensed platform" },
  ]}
/>

This is a structural property, not a convention. Because the framework
modules never import platform packages, accidental upward coupling is a
compile error.

### Framework package layout

Within the framework module (repo root), packages are grouped by
responsibility:

```
core/
    model/            Content model types (Part, Block, Layer, Run, Target, Overlay)
    format/           DataFormatReader/Writer interfaces, detection, skeleton
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
    ai/               AI pipeline tools (tools/), prompts (prompt/), and NER (ner/)
    mt/               MT pipeline tools (tools/)
    plugin/           Plugin system: gRPC host, Java bridge, loader, registry
    storage/          Shared SQLite DB infrastructure
    blockstore/       Block store Session/Store interfaces
    project/          .kapi project file format
    preset/           Built-in preset definitions
    schema/           JSON-schema generation for tool/component parameters
    segment/          Segmentation primitives and masking
    brand/            Brand-voice model
    graph/            Graph data structures
    i18n/             Localization of component schemas and metadata
    its/              W3C ITS metadata
    klf/              KLF localization exchange format
    redaction/        Span redaction/restoration
    ignore/           .kapiignore pattern matching
    httputil/         HTTP client helpers
    set/              Generic set container
    internal/
        testutil/     Shared test helpers
sievepen/             Translation memory (interface + in-memory + SQLite + matching)
termbase/             Terminology (interface + in-memory + SQLite + import)
providers/
    ai/               package aiprovider — LLM providers
    mt/               package mtprovider — MT providers
bench/                Benchmarks
examples/             Plugin examples
```

The list above names the framework's primary package groups rather than
enumerating every directory; consult the source tree for the authoritative set.

### Workspace and versioning

The root `go.work` file coordinates the workspace for local development. It
lists every module in the repository — the framework modules below plus the
separately-licensed platform modules and build-support modules under
`scripts/`. The four framework modules are:

```
use (
    .                   # framework
    ./cli               # shared CLI base
    ./kapi              # kapi CLI
    ./apps/kapi-desktop # kapi-desktop
    # … plus the platform modules (under bowrain/) and scripts/ helpers
)
```

With the workspace active, changes to framework code are visible to the CLI,
kapi, and kapi-desktop without publishing. `go mod tidy` does not respect
`go.work`, so each child module's `go.mod` carries a `replace` directive
pointing to `../` for the parent modules it depends on.

Only the framework module is tagged today, using flat semver tags (`vX.Y.Z`).
The child modules (`cli`, `kapi`, `kapi-desktop`, and the platform modules) are
not independently tagged. Go's module-version conventions would allow
per-module tags (e.g. `cli/v0.1.0`), but the workspace currently relies on
`go.work` plus `replace` directives rather than published per-module versions.

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
pseudo-translate file.json`); kapi-desktop wraps the same commands behind
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
- License-clean: Apache-2.0 framework modules never accidentally pull
  separately-licensed platform code, enforced by import-path topology.
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
