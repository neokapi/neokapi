---
id: 041-kapi-desktop
sidebar_position: 41
title: "AD-041: Kapi and .kapi Project Files"
---
# AD-041: Kapi and .kapi Project Files

## Context

Kapi CLI is a standalone, one-shot localization tool: `kapi ai-translate -i file.json --target-lang fr`. It requires no project setup, no server, no configuration files. This simplicity is its strength — but it means every invocation requires repeating flags, and there is no way to save a workflow for reuse.

Developers who process files regularly need:
- A way to **save and share** localization workflows (source/target languages, content patterns, tool pipelines)
- A **visual interface** for building flows, running tools, and managing plugins
- **Credential management** for AI providers (API keys in the OS keychain, not in environment variables or command flags)

Bowrain Desktop ([AD-012](./012-bowrain.md)) serves the Bowrain platform — it connects to a server, syncs content, manages workspaces. It is not suitable as a standalone kapi companion because it depends on the platform module and requires a `.bowrain/` project directory tied to a server.

## Decision

### .kapi Project Files

A `.kapi` file is a **self-contained YAML document** that captures a localization workflow recipe. It follows the desktop document paradigm — users open, edit, save, and share `.kapi` files like any other project file.

```yaml
# translation.kapi
version: v1
name: My App Localization

source_language: en-US
target_languages: [fr-FR, de-DE, ja-JP]

content:
  - path: "src/locales/en/*.json"
    format: json
    target: "src/locales/{lang}/*.json"

preset: nextjs
plugins: [okapi@1.47.0]

flows:
  translate:
    steps:
      - tool: ai-translate
        config:
          provider: anthropic
          model: claude-sonnet-4-5-20241022

  translate-and-qa:
    steps:
      - tool: ai-translate
        config:
          provider: anthropic
      - tool: qa-check

defaults:
  concurrency: 4
  parallel_blocks: 3
```

**Key properties:**
- **Self-contained** — languages, content patterns, flows, tool configs, plugin requirements in one file
- **No credentials** — provider names only; API keys come from the OS keychain or environment variables
- **No state** — no sync cursors, caches, or timestamps; `.kapi` files are always clean
- **Portable** — save anywhere, have multiple per directory (`translate.kapi`, `qa-only.kapi`), commit to git
- **CLI-compatible** — `kapi run translate -p translation.kapi` uses the project for defaults

The `.kapi` format uses the same flow steps format as `.bowrain/flows/` ([flow-steps-format](../notes/flow-steps-format.md)), ensuring compatibility between the two systems.

### .kapi vs .bowrain

| Aspect | `.kapi` file | `.bowrain/` directory |
|---|---|---|
| Scope | Standalone file processing | Server-connected project sync |
| Format | Single YAML file | Directory with config + flows + cache |
| Server | None | Required (Bowrain Server URL) |
| Sync state | None | `.sync-cache` (block hashes, cursors) |
| Automation | Local flows only | Server-side hooks and automation rules |
| Module | Framework (`core/project/`) | Platform (`platform/project/`) |
| CLI | `kapi run -p file.kapi` | `bowrain run flowname` |
| Desktop | Kapi | Bowrain Desktop |

The upsell path from kapi to bowrain is about **managed AI projects** — team collaboration, server-side automation, connector integrations — not technical migration. Users who outgrow standalone file processing adopt bowrain for its platform features.

### Kapi App

Kapi is a Wails v3 application at `framework/apps/kapi-desktop/`. It is part of the **framework** module (no platform dependencies) and provides a GUI for the same capabilities as kapi CLI.

**Technology stack** — identical to Bowrain Desktop ([AD-012](./012-bowrain.md)):
- Go backend with Wails v3 auto-generated TypeScript bindings
- React 19 + Vite + TailwindCSS 4 frontend
- Storybook 10 for component development

**Architecture** — significantly simpler than Bowrain Desktop:

```
Kapi                          Bowrain Desktop
─────────────                         ────────────────
Format/tool registries    ✓           ✓
Plugin loader             ✓           ✓
Credential store (keyring) ✓          ✓ (server-managed)
.kapi project files       ✓           ✗ (.bowrain/ projects)
Flow execution + events   ✓           ✓
Server connection         ✗           ✓ (gRPC)
Offline queue             ✗           ✓
Content store (SQLite)    ✗           ✓
Workspace management      ✗           ✓
Real-time collaboration   ✗           ✓
```

**Backend service** (`backend/app.go`) exposes:
- Project operations: New, Open, Save, SaveAs
- Flow CRUD within the open `.kapi` file
- Flow execution with TraceRecorder event streaming
- Tool listing with JSON schema for dynamic config forms
- Plugin search, install, update from the remote registry
- Credential management via the OS keychain
- Content pattern glob resolution
- Recent files and settings persistence

**Frontend views:**
- Welcome page with neokapi logo, getting started, quick actions, recent files
- Project overview (languages, content, flows, plugins)
- Flow editor (steps-based, with visual graph editor planned)
- Flow runner with live progress (node highlighting, per-file progress)
- Tool runner for ad-hoc single-tool execution
- Plugin manager (browse registry, install, update)
- Credential manager (AI provider configs, keychain storage)
- Settings (theme, plugin directory)

### Module Placement

Kapi lives at `framework/apps/kapi-desktop/` as a **separate Go module**. It cannot live inside `framework/kapi/` because the Makefile isolation check forbids Wails and go-keyring dependencies in that module.

```
go.work:
  ./framework/apps/kapi-desktop   ← depends on framework + cli only
```

The module depends on:
- `github.com/neokapi/neokapi` (core: flow, format, tool, plugin, registry, project)
- `github.com/neokapi/neokapi/cli` (shared CLI: credentials, config)
- `github.com/wailsapp/wails/v3` (desktop framework)
- `github.com/zalando/go-keyring` (OS keychain)

It has **zero platform dependencies** — verified by `GOWORK=off go build ./...`.

### Credential Store

The credential store (`cli/credentials/`) was extracted from `platform/credentials/` to the framework CLI module. It stores provider configurations as JSON at `~/.config/kapi/providers.json` and API keys in the OS keychain under the `"kapi"` service name. Both Kapi and the kapi CLI share this store.

### Distribution

- **macOS**: DMG via GitHub Releases, Homebrew Cask (`brew install --cask neokapi/tap/kapi-desktop`)
- **Windows**: ZIP archive via GitHub Releases
- **Linux**: Binary via GitHub Releases
- **CI**: GitHub Actions workflow builds all platforms on tag push
- **File association**: `.kapi` files open in Kapi on macOS (via `CFBundleDocumentTypes` in Info.plist)

## Alternatives Considered

- **Extend Bowrain Desktop for standalone use**: Would require making the platform module's server connection optional and stripping workspace management. Simpler to build a focused app from the framework module.

- **Web-only UI (kapi serve)**: Considered as a lighter alternative. Does not provide OS keychain integration, file associations, or native file dialogs. Could be added later as a complement.

- **`.kapi/` directory instead of `.kapi` file**: A directory model (like `.bowrain/`) was considered but rejected. The single-file document paradigm is simpler, more portable, and fits the standalone tool positioning better.

- **Auto-discover .kapi files in current directory**: Rejected to keep `kapi run` as a pure one-shot tool. Project files are only used with the explicit `-p` flag.

## Consequences

- Kapi provides a visual GUI for all kapi CLI capabilities without requiring bowrain or a server.

- `.kapi` files are portable, self-contained workflow documents that can be shared via git.

- The credential store is now in the framework CLI module, making OS keychain access available to both Kapi and the kapi CLI.

- `kapi run -p file.kapi` enables project-based CLI workflows without breaking the one-shot default behavior.

- Kapi and Bowrain Desktop share the same technology stack (Wails v3, React 19, TailwindCSS) but are independent applications with no shared frontend code (shared flow editor extraction is planned as a follow-up).

- The `.kapi` file format uses `core/flow.StepsSpec` for flow definitions, ensuring compatibility with `.bowrain/flows/` and built-in flows.

- Distribution via Homebrew Cask provides a familiar install path for macOS developers: `brew install neokapi/tap/kapi && brew install --cask neokapi/tap/kapi-desktop`.
