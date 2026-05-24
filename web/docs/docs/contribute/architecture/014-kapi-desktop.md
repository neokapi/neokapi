---
id: 014-kapi-desktop
sidebar_position: 14
title: "AD-014: Kapi Desktop"
description: "Architecture decision: Kapi Desktop is a Wails v3 app that provides a visual companion to the kapi CLI — opening .kapi project files, editing flows visually, running them with live progress, and managing plugins and credentials."
keywords: [Kapi Desktop, Wails v3, desktop app, flow editor, visual companion, architecture decision, neokapi]
---

# AD-014: Kapi Desktop

## Summary

Kapi Desktop is a Wails v3 desktop application at `apps/desktop/` that
provides a visual companion to the kapi CLI. It opens `.kapi` project files
as documents, offers a flow editor with live runner progress, manages
plugins, and surfaces the OS-keychain credential vault. The module depends
reuses `@neokapi/ui-primitives` and `@neokapi/flow-editor` from the
monorepo's npm workspaces.

## Context

Engineers run localization workflows from the kapi CLI. Localization
specialists and translators benefit from a visual interface: drag-and-drop
flow construction, live progress while tools run, clickable plugin
installation, and a GUI for entering API keys.

The framework exposes all the primitives needed for a desktop app — flow
executor, tool registry, format registry, plugin system, AI/MT providers,
block store. A desktop app can wrap these in a native window without
pulling in server dependencies, authentication flows, or sync protocols.

The constraint is module isolation ([AD-001: Vision and Modules](001-vision-and-modules.md)):
Wails v3 and `go-keyring` bring heavy dependencies that must not leak into
the kapi CLI module. Kapi Desktop lives in a separate module that depends
on framework and shared CLI only.

## Decision

### Technology stack

- **Backend** — Go with Wails v3 auto-generated TypeScript bindings.
- **Frontend** — React 19, Vite, TailwindCSS 4, shadcn/ui components.
- **Shared UI** — `@neokapi/ui-primitives` (shadcn primitives) and
  `@neokapi/flow-editor` (React flow editor) via npm workspace symlinks.
- **State** — Wails-generated bindings on the frontend, plain Go services
  on the backend.
- **License** — Apache-2.0, consistent with the framework.

### Module layout

```
apps/desktop/
├── go.mod                   # module github.com/neokapi/neokapi/kapi-desktop
├── main.go                  # Wails v3 entry point
├── backend/                 # Go backend services
│   ├── app.go               # service registry exposed to the frontend
│   ├── project/             # .kapi open/save/edit
│   ├── flows/               # flow CRUD
│   ├── runner/              # flow execution with event streaming
│   ├── credentials/         # OS keychain wrapper
│   └── plugins/             # plugin search, install, update
├── frontend/                # React 19 + Vite + TailwindCSS
│   ├── src/
│   │   ├── views/           # Welcome, Project, FlowEditor, FlowRunner, Plugins, Credentials, Settings
│   │   ├── components/      # leaf components
│   │   └── bindings/        # Wails-generated TypeScript
│   └── vite.config.ts
└── build/                   # Wails build configs + per-platform settings
```

The module depends on:

- `github.com/neokapi/neokapi` — framework (flow, format, tool, plugin,
  registry, project).
- `github.com/neokapi/neokapi/cli` — shared CLI base (credentials, config).
- `github.com/wailsapp/wails/v3` — desktop framework.
- `github.com/zalando/go-keyring` — OS keychain.

Module isolation is verified by `GOWORK=off bash -c "cd apps/kapi-desktop && go build ./..."`.

### Backend services

`backend/app.go` exposes a service struct to the frontend via Wails
bindings. The main groups:

- **Project operations** — `NewProject`, `OpenProject`, `SaveProject`,
  `SaveProjectAs`. Loads and writes `.kapi` recipes
  ([AD-008: Kapi Project Model](008-project-model.md)).
- **Flow CRUD** — add, remove, reorder, and configure flows within the
  open `.kapi` project. Validates step schemas against the tool registry.
- **Flow execution** — runs a flow through the framework's Executor and
  streams `TraceRecorder` events (step start/finish, block progress,
  error/warning, tool log output) to the frontend over Wails events.
- **Tool inspection** — lists registered tools with JSON Schema for each
  tool's config, enabling the frontend to render dynamic configuration
  forms.
- **Plugin manager** — search the remote registry, install plugins,
  update plugins, uninstall plugins. Uses the plugin system
  ([AD-007: Plugin System](007-plugin-system.md)).
- **Credential vault** — add, edit, and remove AI/MT provider
  configurations. Provider configs live as JSON at
  `~/.config/kapi/providers.json`; API keys live in the OS keychain
  under the `"kapi"` service name (shared with the kapi CLI).
- **Content matching** — resolves a project's content patterns against
  the filesystem via `ProjectContext.ResolveContent`, showing matched
  files grouped by collection.
- **Recents and settings** — persists recent files at
  `~/.config/desktop/recent.json` and settings at
  `~/.config/desktop/settings.json` (theme, plugin directory).

### Frontend views

- **Welcome** — logo, getting started, quick actions (open recent,
  create new, install a plugin).
- **Project overview** — source and target locales, content collections
  with matched file counts, declared flows, declared plugins.
- **Flow editor** — steps-based configuration with schema-driven forms;
  the `@neokapi/flow-editor` package provides a graph editor
  (xyflow) for visual composition.
- **Flow runner** — live progress view with node highlighting, per-file
  progress, streaming tool logs, and cancellation.
- **Tool runner** — ad-hoc single-tool execution for quick experiments
  outside a project.
- **Plugin manager** — browse the registry, install by name, update
  installed plugins, see plugin-provided formats and tools.
- **Credential vault** — add a provider, test the key, store in the
  keychain. Keys are never displayed after entry.
- **Settings** — theme (light/dark/system), plugin directory, telemetry
  opt-in.

### Reuse of framework primitives

Kapi Desktop reuses the framework's interfaces rather than
re-implementing them:

- **BlockStore** — the memory provider for ephemeral editing, the
  SQLite `cache` provider for persistent project state.
- **Tool registry** — the same registry the CLI uses; the desktop
  surfaces it visually.
- **Plugin system** — the same `go-plugin` + gRPC protocol.
- **AI/MT providers** — the same provider interfaces and worker pool.
- **Flow executor** — the same streaming pipeline; the runner view is a
  consumer of the executor's event stream.

The desktop never forks framework behavior; differences are purely
presentational (dynamic forms, event streaming, live progress).

### Shared frontend packages

Storybook:

- **`@neokapi/ui-primitives`** (`packages/ui/`) — shadcn/ui
  primitives (Button, Card, Badge, Label, Input, Tabs, ScrollArea, etc.)
  plus layout components (PageHeader, EmptyState, SkeletonCard,
  PanelHeader, LoadingSpinner).
- **`@neokapi/flow-editor`** (`packages/flow-editor/`) — a React flow
  editor component library built on xyflow, used by both kapi-desktop

Both packages resolve via npm workspace symlinks; no path aliases are
needed. `vp install` at the repo root installs the entire workspace.

### Opening projects with different block stores

Kapi Desktop opens any `.kapi` project, including projects that declare
of the `BlockStore` interface by the project machinery, not by the
desktop app directly. This keeps the desktop's dependency footprint
without switching tools.

### Distribution

- **macOS** — DMG via GitHub Releases, Homebrew Cask
  (`brew install --cask neokapi/tap/kapi-desktop`). `.kapi` file
  association via `CFBundleDocumentTypes` in Info.plist.
- **Windows** — ZIP archive via GitHub Releases.
- **Linux** — binary via GitHub Releases; AppImage planned.
- **CI** — a GitHub Actions workflow builds all platforms on tag push.

## Consequences

- Kapi Desktop provides a visual GUI for every kapi CLI capability
- `.kapi` files are shareable workflow documents — open, edit, save,
  commit to git; no hidden state travels with the document.
- The OS keychain integration makes the desktop the preferred way for
  non-engineering users to configure AI and MT providers.
  stack and frontend packages, reducing duplication and keeping
  shadcn primitives consistent.
- Separate Go modules keep Wails and keyring dependencies out of the
  kapi CLI module, preserving cross-compilation and small CLI
  binaries.
- Because the desktop reuses framework primitives directly, any new
  tool, format, or provider added to the framework appears in the
  desktop without backend changes (beyond registration).

## Related

- [AD-001: Vision and Modules](001-vision-and-modules.md) — module
  isolation contract
- [AD-004: Processing Engine](004-processing-engine.md) — executor and
  event streaming
- [AD-006: Tool System](006-tool-system.md) — tool registry and schemas
- [AD-007: Plugin System](007-plugin-system.md) — plugin install/update
- [AD-008: Kapi Project Model](008-project-model.md) — `.kapi` recipe
  and `.kapi/` state
- [AD-011: AI Providers](011-ai-providers.md) — provider credential
  store
- [AD-013: Kapi CLI](013-kapi-cli.md) — CLI companion; shared
  credential store
