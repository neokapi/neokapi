---
id: s-02-kapi-desktop
sidebar_position: 2
title: "S-02: Kapi Desktop"
description: "Kapi Desktop is a Wails v3 application over the same host runtime the CLI uses — one Go service bound to a React frontend, several projects open as tabs, live run events, and no Cobra anywhere in its dependency graph."
keywords: [neokapi, architecture decision, Kapi Desktop, Wails, desktop app, module isolation, flow runner, review, credential vault]
---

import { SwimlaneDiagram } from "@neokapi/docs-shared";

# S-02: Kapi Desktop

## Summary

Kapi Desktop is a Wails v3 application at `apps/kapi-desktop/` (module
`github.com/neokapi/neokapi/kapi-desktop`). It opens projects by their folder —
the one holding a `kapi.yaml` recipe — several at a time as tabs, and gives them
a visual surface: content collections, a flow editor and live runner, the
catch-up loop, a review queue, block-level inspection, plugin management, and an
OS-keychain credential vault. It depends on the framework and `host` only. It
links neither Cobra nor the `cli` module, which `make audit-modules` asserts by
inspecting the transitive package list of `./backend/...`.

## Context

Everything the desktop does is available from the CLI. What it adds is the
affordance: a form driven by a tool's JSON Schema instead of a remembered flag,
a graph you drag instead of a YAML block you type, a progress view instead of a
log tail, and a place to paste an API key that is not shell history.

Nothing about that needs a second implementation. `host` is deliberately
cobra-free — command threading is the `host.Command` interface, which
`*cobra.Command` satisfies natively and which `host.EnvCommand` satisfies for
embedded runs — so the desktop calls the same functions the CLI does, with a
different thing on the other end of them.

The constraint that shapes the module boundary is the reverse direction: Wails
brings a large native toolchain, and the CLI must never acquire it. So the
desktop is its own Go module, and the boundary is asserted rather than trusted.

## Decision

### Stack and module boundary

- **Backend** — Go, with Wails v3 generating TypeScript bindings from the
  exported service methods.
- **Frontend** — React 19, Vite, Tailwind CSS 4, shadcn-based primitives.
- **License** — Apache-2.0, matching the framework.

The module's direct dependencies are the framework, `host`, Wails, and one
schema-only module contributed by the platform layer. That last one exists
precisely so the desktop can validate a recipe carrying platform extensions
without linking the platform: it holds recipe vocabulary and extension decoders
and nothing else, and it is Apache-2.0, so the desktop binary stays free of the
platform's copyleft code. Keeping that import schema-only is a license boundary,
not a size optimisation.

```
apps/kapi-desktop/
├── go.mod                   # module github.com/neokapi/neokapi/kapi-desktop
├── main.go                  # Wails entry point, application menu, embedded assets
├── backend/                 # one flat Go package — the bound service
├── frontend/                # React + Vite; bindings/ is Wails-generated
└── build/                   # per-platform build configuration
```

Three assertions run in `make audit-modules`: an isolated `GOWORK=off` build of
`./backend/...` (so the module resolves against its own `go.mod`, not the
workspace overlay), a tidiness check that fails on any stale or
boundary-crossing requirement, and an import-level check that the transitive
package list contains no Cobra and no `cli` package.

### One bound service, many open projects

`backend.App` is the single Wails service. The frontend calls its exported
methods; there is no second RPC layer and no hand-written binding code. Within
that one service the concerns are split across files: project lifecycle, flow
CRUD, the runner, the catch-up loop, checks, the review queue, inspection,
content memory, terms, media, outputs, plugins, credentials and model detection,
locales, settings and recents, the file watcher, and the in-app updater.

Projects open as **tabs**. Every project-scoped method takes a `tabID`, so the
open set is real state in the backend — each tab carries its own loaded recipe,
project context, and resolved stores, and closing one releases them. A file
watcher on the recipe keeps an open tab honest when the file changes underneath
it, which is the normal case when the same project is also being driven from the
CLI or from git.

<SwimlaneDiagram
  actors={[
    { label: "Frontend", sub: "React", role: "io" },
    { label: "backend.App", sub: "Wails service" },
    { label: "Executor", sub: "host + framework", role: "translate" },
  ]}
  messages={[
    { from: 0, to: 1, label: "RunFlow(tabID, flow, inputs, locales)" },
    { from: 1, to: 2, label: "execute", detail: "flow spec + project context" },
    { from: 2, to: 1, label: "trace records", detail: "step, block, log, error" },
    { from: 1, to: 0, label: "flow:event", detail: "emitted + buffered for reconnect" },
    { from: 0, to: 1, label: "CancelRun()" },
    { from: 1, to: 2, label: "context cancel" },
  ]}
  caption="A run is one call in and a stream of events out. The backend buffers the stream so a reloaded window replays the run rather than losing it."
/>

Buffering matters more than it looks. A desktop window can reload mid-run, and a
run that only existed as a live event stream would vanish. The backend keeps the
events, collapsing consecutive metric snapshots so a long run does not grow the
buffer at the sampling rate.

### The frontend consumes the workspace packages

The desktop is one consumer of the monorepo's shared frontend packages rather
than the owner of any of them: the shadcn primitives and the preview kit from
`@neokapi/ui-primitives` ([S-06](s-06-visual-editor.md)), the xyflow-based
`@neokapi/flow-editor`, the lab explorers in `@neokapi/kapi-lab`, plus the
contract types, reference data, status views, concept views, and grid editor
packages. All resolve through the single root pnpm workspace.

It also consumes `@neokapi/i18n-react` ([S-05](s-05-i18n-runtime.md)) for its
own interface languages — the desktop's UI strings go through the same
extraction and runtime the framework offers to any React application. The
target-language catalogs are build artefacts, regenerated rather than authored.

The flow editor's graph edits **composition** only. Its two ends are endpoint
pickers — file, store, interchange, or none — not draggable reader and writer
nodes, because the binding is a property of the run rather than of the flow
([E-04](../engine/e-04-flows-and-io-binding.md)).

### Configuration lives in two roots

Provider configuration and installed plugins live under the kapi config home,
shared with the CLI: `providers.json` there, API keys in the OS keychain under
the `kapi` service name, plugins under `plugins/` unless `KAPI_PLUGIN_DIR` says
otherwise ([S-01](s-01-kapi-cli.md)).

The desktop's own preferences — theme, interface language, hidden and custom
locales, recent projects — live in a *separate* root, `<UserConfigDir>/kapi-desktop`,
overridable with `KAPI_DESKTOP_CONFIG_DIR`. The split is deliberate: a preference
about a window is not a setting the CLI should read, and resetting one should not
disturb the other.

### It reuses framework primitives rather than re-deriving them

Tool listings come from the same registry the CLI reads, and each tool's
configuration form is generated from its JSON Schema
([E-03](../engine/e-03-tool-system.md)) — so a new tool appears in the desktop
with no desktop change beyond registration. Formats, plugins, providers, block
stores, and the executor are likewise the framework's, consumed rather than
mirrored. A project declaring a different block store opens without the desktop
knowing which one: resolution goes through the `BlockStore` interface in the
project machinery.

Differences from the CLI are presentational — dynamic forms, event streaming,
live progress, tabbed state. There is no desktop fork of framework behaviour.

### Distribution

The release workflow builds the desktop across a platform matrix covering macOS,
Windows, and Linux on both common architectures where the toolchain allows it.
macOS ships a signed application distributed as a Homebrew cask, Windows an
Authenticode-signed ZIP, Linux a tarball, all attached to the GitHub release. A
signed appcast feeds the in-app updater, so an installed desktop can update
itself without going back through the package manager.

## Consequences

- The desktop reaches every framework capability through `host`, so it can never
  drift from the CLI on behaviour — only on presentation.
- Wails and the native toolchain stay out of the CLI module, which keeps the CLI
  cross-compilable and small.
- The cobra-free assertion is mechanical rather than a convention: a
  reintroduced `cli` import fails `make audit-modules`, not a review.
- The schema-only platform import keeps the desktop able to open a
  platform-extended recipe while staying Apache-2.0 end to end.
- Tabs make the desktop honest about a real working pattern — several projects
  open at once — at the cost of every project-scoped method carrying a `tabID`.
- `kapi.yaml` recipes stay shareable workflow documents: open, edit, save,
  commit. No hidden state travels with the recipe.
- Any new tool, format, or provider registered in the framework appears in the
  desktop with no backend change.

## Related

- [F-01: The framework and its modules](../foundations/f-01-framework-and-modules.md) — the module isolation contract and the license boundary
- [E-01: The processing engine](../engine/e-01-processing-engine.md) — the executor and its trace events
- [E-03: The tool system](../engine/e-03-tool-system.md) — the registry and schemas the forms are generated from
- [E-04: Flows and I/O binding](../engine/e-04-flows-and-io-binding.md) — why the graph's ends are endpoint pickers
- [E-05: The plugin system](../engine/e-05-plugin-system.md) — the plugin manager's model
- [C-01: The project model](../context/c-01-project-model.md) — the recipe and the `.kapi/` state a tab loads
- [C-04: Unit state and decisions](../context/c-04-unit-state-and-decisions.md) — what the review queue records
- [S-01: The kapi CLI](s-01-kapi-cli.md) — the shared credential store and config home
- [S-05: The i18n runtime for React](s-05-i18n-runtime.md) — the runtime the desktop's own interface uses
- [S-06: The visual editor data model](s-06-visual-editor.md) — the preview kit the desktop hosts
- [Kapi Desktop overview](/kapi/desktop/overview) — the user-facing guide
