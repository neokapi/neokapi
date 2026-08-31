---
id: f-01-framework-and-modules
sidebar_position: 1
title: "F-01: The framework and its modules"
description: "neokapi is an Apache-2.0 Go content and language intelligence framework shipped as independent modules (the framework, the cobra-free host runtime, the Cobra shell, the kapi CLI, the desktop app, and the out-of-process plugins), with each module's dependency footprint declared in its own go.mod and enforced by GOWORK=off builds."
keywords: [neokapi, architecture decision, Go modules, go.work, multi-module, module boundary, Apache-2.0]
---

import { PipelineDiagram } from "@neokapi/docs-shared";

# F-01: The framework and its modules

## Summary

neokapi is an open content and language intelligence framework in Go,
distributed under Apache-2.0. It ships as independent Go modules: the framework
itself, the cobra-free host runtime, the thin Cobra shell, the `kapi` binary, the
desktop app, and one module per out-of-process plugin. The first five are
coordinated by a `go.work` file, together with the build-support modules under
`scripts/` and the separately-licensed modules that build on the framework; the
plugin modules sit outside it. Every module declares its own dependency
footprint, and a `GOWORK=off` build of each module proves the declaration is
real.

## Context

One codebase has to serve several deployment targets: a standalone
file-processing CLI for engineers, a visual desktop app for content and language
specialists, and a library that larger systems embed. Each target has a different
dependency profile (Wails for the desktop app, keychain access for credentials,
SQLite for local stores, ONNX runtimes for ML plugins), and forcing every binary
to link every dependency produces slow builds and bloated artifacts.

The framework is also the Apache-2.0 base of a broader ecosystem.
Separately-licensed layers built on top of it must consume framework interfaces
without pushing their dependencies down into the framework. That boundary is
enforced by the build rather than by convention.

## Decision

### Identity

neokapi provides format-aware document parsing into one content model, a
channel-based concurrent processing engine, faithful write-back, and composable
tools that edit, check, and translate the content inside. Everything is a library
and a toolkit: it runs no server and holds no accounts. The framework is the
vehicle for open extension in format support, processing tools, and AI
integration.

Five design principles shape the module layout:

1. **Streaming concurrency.** Documents flow through a pipeline of channels; each
   tool runs in its own goroutine. See
   [E-01: The processing engine](../engine/e-01-processing-engine.md).
2. **Content-addressable blocks.** Units are identified by hashes of their
   normalized content and their context, enabling deduplication across sources
   and incremental processing. See [F-02: The content model](f-02-content-model.md)
   and [F-03: Identity](f-03-identity.md).
3. **Progressive complexity.** A single command on day one, a YAML flow on day
   two, an integrated project on day ten. The same content model and tool chain
   work at every scale.
4. **AI as ordinary pipeline tools.** Translation, verification, term extraction,
   and review by model are tools that participate in the same flow execution
   model as format-aware tools.
5. **Single-binary distribution.** Go compiles to static binaries. The shared
   codebase produces the `kapi` CLI and the desktop app with no JVM, no Node.js
   runtime, and no container required for basic usage.

### The modules

| Module | Import path | Directory | Role |
| --- | --- | --- | --- |
| Framework | `github.com/neokapi/neokapi` | `.` | Content model, formats, tools, flows, the plugin manifest and protocol, content memory, terms, model providers |
| Host | `github.com/neokapi/neokapi/host` | `host/` | Cobra-free application runtime and services: app config, credentials, the plugin host (discovery, install, dispatch, daemon pool), flow / convergence / check services |
| CLI | `github.com/neokapi/neokapi/cli` | `cli/` | Thin Cobra shell over host: command factories, flag registration, dispatch |
| Kapi | `github.com/neokapi/neokapi/kapi` | `kapi/` | The `kapi` binary |
| Kapi Desktop | `github.com/neokapi/neokapi/kapi-desktop` | `apps/kapi-desktop/` | Wails v3 desktop app |
| Plugins | `github.com/neokapi/neokapi/plugins/<name>` | `plugins/<name>/` | One module per out-of-process plugin binary |

The dependency graph is strictly hierarchical, and the desktop's position in it
is the reason the runtime and the command shell are separate modules at all:

<PipelineDiagram
  channelLabel=""
  caption="kapi builds on framework + host + cli; the desktop app builds on framework + host and links no Cobra. CI enforces both boundaries."
  stages={[
    { label: "framework", sub: "core/ · library only" },
    { label: "host", sub: "runtime + services · no cobra" },
    {
      lanes: [
        { label: "kapi", sub: "framework + host + cli" },
        { label: "kapi-desktop", sub: "framework + host · no cli, no cobra" },
      ],
      parallelLabel: "build on framework + host",
    },
  ]}
/>

### Dependency rules

- **Framework** is library code: no UI toolkit, no HTTP server, no command
  framework, no identity protocol. Nothing here imports Wails, an HTTP server
  framework, Cobra, Viper, or OIDC. It *does* link SQLite and the OS keychain,
  because the local stores and credential storage are framework concerns rather
  than application ones.
- **Host** depends only on the framework, and **must not import Cobra**. Command
  threading is the `host.Command` interface (a context, a `*pflag.FlagSet`, and
  the three IO streams), which `*cobra.Command` satisfies natively. Embedded runs
  (the desktop, MCP tools, internal orchestrators) pass a `host.EnvCommand`
  instead.
- **CLI** depends on framework + host. Cobra lives here and nowhere below.
- **Kapi** depends on framework + host + CLI.
- **Kapi Desktop** depends on framework + host, and on neither the cli module
  nor Cobra. Recipe vocabulary for an extension reaches it through host: a
  package there registers the extension's recipe keys with the framework's
  project registry, which is how the desktop reads a recipe's declared run venue
  without depending on the extension's implementation. The desktop is a separate
  module so that Wails v3 stays out of the CLI build.
- **Plugin modules** depend on the framework only. Each builds a standalone
  binary that kapi discovers at runtime and dispatches as a subprocess, so no
  plugin's dependencies (ONNX runtimes, cgo raster libraries, media codecs)
  reach the portable `kapi` binary. See
  [E-05: The plugin system](../engine/e-05-plugin-system.md).

### How the rules are enforced

`make audit-modules`, run by `make pre-push`, builds every workspace module
except the build-support ones with `GOWORK=off`, so each module resolves against
its own `go.mod` rather than the workspace overlay that would otherwise hide a
boundary violation:

```bash
GOWORK=off go build ./...                                            # framework
GOWORK=off bash -c "cd host && go build ./..."                       # host
GOWORK=off bash -c "cd cli && go build ./..."                        # cli
GOWORK=off bash -c "cd kapi && go build ./..."                       # kapi
GOWORK=off bash -c "cd apps/kapi-desktop && go build ./backend/..."  # kapi-desktop
```

A successful `GOWORK=off` build proves every cross-module import is a real
dependency declared in that module's `go.mod`. The desktop build is scoped to
`./backend/...` because the module's top-level package embeds `frontend/dist`,
which does not exist until the frontend has been built; an unscoped `./...` fails
on the missing embed before it ever typechecks. `go mod tidy` still resolves the
whole module graph (embeds do not affect dependency resolution), so the boundary
contract holds.

Three further assertions run on top of the builds, and CI runs them on every
change through two targets: `make check-module-boundaries` (the
`module-boundaries` job) carries the import assertions, and `make ci-tidy` (the
tidy-check job) carries tidiness.

- `go mod tidy` must be a no-op per module. A stale require, a missing one, or a
  require that pulls in a forbidden module all leave a diff.
- The desktop backend's transitive package list (`go list -deps ./backend/...`)
  must contain neither Cobra nor the cli module. Matching on *packages* rather
  than modules means a transitive dependency cannot dodge it.
- No Apache-2.0 module may reach a package under a separately-licensed tree.
  There is no exception, and adding one is the failure mode this assertion
  exists to prevent: a type both sides need belongs below the line, not on an
  allowlist above it.

The plugin modules are outside `go.work`, which means no workspace build would
notice them drifting. `make ci-tidy` therefore tidies them alongside the
workspace modules, so a module no ordinary job builds still cannot rot.

### License posture

The framework and every binary built from it (framework, host, cli, kapi,
kapi-desktop, and the plugin modules under `plugins/`) are Apache-2.0 end to end
and import no code licensed otherwise.
Separately-licensed layers attach through the extension mechanism and the plugin
registries: they consume framework interfaces (content model, tools, flows,
formats) and are discovered at runtime as subprocesses, never linked in. The
gradient is one-directional, and because it is expressed as import topology, an
accidental upward edge is a build failure rather than a review finding.

A source file carries no license header: its license is a function of the
nearest `LICENSE` file above it in the tree, and of nothing else. Two things
follow. Moving a file between subtrees *is* relicensing it, with no metadata to
keep in step. And nothing in a file says what license it is under, so the import
assertions above are the only thing standing between that property and an
accident.

### Framework package layout

Within the framework module, packages group by responsibility. These are the
primary groups rather than an exhaustive listing; the source tree is the
authoritative set:

```
core/
    model/            Content model types (Part, Block, Layer, Run, Target, Overlay)
    format/           DataFormatReader/Writer interfaces, detection, skeleton
    tool/             Tool interface, BaseTool dispatch
    flow/             Executor, Builder, flow definitions
    registry/         Format and tool registries
    engine/           Generic registry of named factories
    formats/          Built-in format implementations
    tools/            Built-in tools
    ai/               Model-backed pipeline tools, prompts, NER
    mt/               Translation-provider pipeline tools
    plugin/           Plugin manifest types, protocol, protoconvert, conformance suite
    proto/            Canonical content-model, engine, and sync protobuf schemas
    blockstore/       Block-addressed, append-only overlay store
    projectdb/        The project's local store handle
    project/          Recipe file format and layout resolution
    state/            Workflow-state model
    convergence/      Convergence model over project state
    gate/             Ship gates
    check/            Deterministic and model-backed content verification
    contextgraph/     Identity vocabulary of the context graph
    occurrence/       Where a term is actually used
    reconcile/        Matching a fresh read against the previous one
    ref/              The composite freshness ref
    venue/            Sync-wire conversion and the ref components a venue folds
    segment/          Segmentation engines and masking
    profile/          Voice-profile model
    redaction/        Span redaction and restoration
    structure/        Document-structure inference
    structrec/        The universal structural record
    projection/       Format-neutral render AST
    editor/           Block index, content tree, preview generation
    kbf/              Kapi Bundle Format block serialization
    locale/           BCP-47 locale handling
    id/               Short base62 ID generation
    its/              W3C ITS metadata
    vision/ asr/ av/  Framework seams for recognition and media
    storage/          Shared SQLite infrastructure
    safeio/           Resource-bounding parse primitives
memory/               Content memory (interface, in-memory, SQLite, matching)
terms/                Terms (interface, in-memory, SQLite, import/export)
kpz/                  The .kpz project package: recipe sanitisation, packing
providers/
    ai/               package aiprovider: model providers
    mt/               package mtprovider: translation providers
```

### Workspace and versioning

The root `go.work` file coordinates local development across the framework, host,
cli, kapi, and desktop modules, plus build-support modules under `scripts/`. With
the workspace active, a change to framework code is visible to its consumers
without publishing. `go mod tidy` does not respect `go.work`, so each child
module's `go.mod` carries a `replace` directive pointing at the parent modules it
depends on.

The framework module is tagged with flat semver tags (`vX.Y.Z`). Host, cli,
kapi and the desktop are never tagged separately: the workspace relies on
`go.work` plus `replace` directives rather than published per-module versions.
Each plugin module releases on its own prefixed tag (`<plugin>-vX.Y.Z`), which
drives that plugin's release workflow ([E-05](../engine/e-05-plugin-system.md)).
All modules target Go 1.26 or later.

`make help` is the authoritative catalog of build, test, vet, and lint targets;
it is self-documenting and current.

### Configuration

Layered application configuration lives in `host/config`, built on
[Viper](https://github.com/spf13/viper). Precedence, highest first:

1. **Command flags**: one-off overrides.
2. **Environment variables** (`KAPI_` prefix, dots replaced by underscores, so
   `plugins.directory` reads from `KAPI_PLUGINS_DIRECTORY`).
3. **The global config file**, pinned to one path rather than resolved through a
   search path. `kapi config path` prints the resolved location.
4. **Legacy config locations** (`$HOME/.config/kapi/kapi.yaml`,
   `/etc/kapi/kapi.yaml`), merged underneath so an existing installation keeps
   working. Precedence between these layers is per top-level block, not per leaf.
5. **Code defaults**: zero-config behavior.

A project recipe is **not** an application-config layer. `kapi.yaml` in a working
directory is a recipe describing what to converge, resolved by an upward walk
from the current directory, and it is absent from the config search
path: the two share a filename and nothing else. See
[C-01: The project model](../context/c-01-project-model.md).

### Locale handling

`model.LocaleID` is a `string` typedef holding BCP-47 tags in canonical form
(`en`, `fr`, `pt-BR`). `core/locale` provides validation, normalization, and
display-name resolution:

```go
func Canonical(s string) (model.LocaleID, error)
func CanonicalAll(in []model.LocaleID) ([]model.LocaleID, error)
func Parse(s string) (model.LocaleID, error)
func MustParse(s string) model.LocaleID
func DisplayName(id model.LocaleID) string
func WellKnownLocales() []LocaleInfo
```

`Canonical` is the one normalization every locale crosses on its way in. It
accepts what people and file formats write (POSIX separators such as `nb_NO`, a
codeset or modifier suffix such as `en_US.UTF-8`, any case) and returns the
canonical BCP-47 form; it rejects a tag whose primary subtag names no language,
and keeps a well-formed tag whose other subtags CLDR does not know (`qps-Ploc`)
whole rather than truncating it. `Parse` is stricter about CLDR membership and
serves code that already holds a canonical tag. Both delegate to
`golang.org/x/text/language` for subtag parsing, script inference, and
canonicalization. Format readers, content-memory entries, terms, recipe fields
and command flags all call `Canonical` at their boundaries, so an invalid code
never propagates silently and the rest of the system compares locales as plain
strings.

## Consequences

- The `kapi` binary links no UI toolkit, no HTTP server, and no ML runtime; it
  stays small and fast to build, and the heavy dependencies live in plugin
  binaries that are only spawned when used.
- The CLI module evolves independently of its consumers: a command-shell change
  does not force unrelated rebuilds in kapi or the desktop app.
- A `go.mod` per module is more bookkeeping than one would be, but `go.work`
  resolves cross-module imports during daily development and the release workflow
  handles multi-module builds.
- The license gradient is enforced by import topology rather than by convention,
  so a violation is a build failure.
- The shared host runtime lets the CLI and the desktop expose the same
  operations without duplicating command logic.
- The same content model and tool chain serve a solo developer running one
  command on local files and a team driving a full project through the desktop
  app.

## See also

- [F-02: The content model](f-02-content-model.md): the Part/Block/Run types every module shares
- [F-03: Identity](f-03-identity.md): hashes, reconciliation, and the store key
- [F-04: The content-model wire schema](f-04-wire-schema.md): the one canonical serialization
- [E-01: The processing engine](../engine/e-01-processing-engine.md): the streaming pipeline
- [E-02: The format system](../engine/e-02-format-system.md): readers, writers, and detection
- [E-03: The tool system](../engine/e-03-tool-system.md): the tool interface and IO contracts
- [E-05: The plugin system](../engine/e-05-plugin-system.md): how out-of-process modules attach
- [S-01: The kapi CLI](../surfaces/s-01-kapi-cli.md): the command surface over host
- [S-02: Kapi Desktop](../surfaces/s-02-kapi-desktop.md): the Wails app over host
