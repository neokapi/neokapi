---
id: 012-bowrain
sidebar_position: 12
title: "AD-012: Bowrain Desktop App"
---
# AD-012: Bowrain desktop app with Wails v3

## Context

A desktop GUI is needed for translators who prefer visual editing over CLI
workflows. The app must access all neokapi functionality: connectors, content
store, tools, flows, plugins, TM, terminology, and AI providers.

Key requirements:

- Cross-platform (macOS, Linux, Windows)
- Single binary distribution
- Full access to Go backend (registries, executors, connectors, plugins)
- Modern UI framework for a responsive editing experience
- The app is evolving from a file-centric tool to a connector-driven platform
  experience, where content flows bidirectionally between source systems and the
  translation environment

## Decision

### Technology Stack

Use [Wails v3](https://wails.io/) with a React 19 / TypeScript frontend.

**Go backend** exposes methods as auto-generated TypeScript bindings:
`ListFormats()`, `ExecuteFlow()`, `LoadProject()`, `SaveProject()`,
`PullContent()`, `PushContent()`, etc. Go emits events (`flow-complete`,
`progress-updated`, `sync-status-changed`) for real-time UI updates and opens
native file dialogs.

**Frontend** uses React 19, Vite (fast HMR), TailwindCSS, and shadcn/ui
components. Playwright provides E2E test coverage.

Previously the app used Wails v2 but was migrated to Wails v3 for improved API
stability and ES module support.

### Connector-Driven Project Workflow

The primary workflow is connector-driven rather than file-centric. Connectors
([AD-005](./005-connector-system.md)) are the integration mechanism that
connects Bowrain to content sources:

1. **Connect** -- Configure a connector (CMS, design tool, code repo, or local
   files) with API keys, endpoints, and authentication credentials
2. **Browse** -- List available content items from the connected system via
   `Connector.List()`, displayed as a browsable tree
3. **Select** -- Choose which items to pull for translation, with locale and
   content type filters
4. **Pull** -- Extract content into the Content Store
   ([AD-003](./003-content-store.md)) via `Connector.Pull()`, producing the
   same Part stream used throughout the pipeline
5. **Translate** -- Work in the translation editor with TM
   ([AD-009](./009-translation-memory.md)), terminology
   ([AD-010](./010-terminology.md)), and AI assistance
   ([AD-008](./008-ai-integration.md))
6. **Push** -- Send translations back to the source system via
   `Connector.Push()`

For file-based workflows, the FileConnector ([AD-005](./005-connector-system.md))
provides the same experience: browse files, select, extract, translate, merge
back. This means the UI does not distinguish between a CMS integration and a
local file -- the same workflow applies to both.

### Connector Management UI

The connector management panel provides:

- **Configuration** -- API keys, endpoints, authentication for each connector.
  Connector-specific settings are rendered dynamically based on the connector's
  `Configure()` schema
- **Content browser** -- Tree/list view of remote content items returned by
  `Connector.List()`, with content type icons and locale indicators
- **Sync status** -- Last pull/push timestamps, pending changes count, and
  change indicators from `Connector.Sync()`. Visual badges alert when source
  content has changed since the last pull
- **Visual diff** -- Compare local content store state against remote source
  to identify what has changed before pulling or pushing

### Translation Editor

Four layout modes (Grid, Focus, Split Horizontal, Split Vertical) with block status tracking and an enhanced toolbar. The context panel provides TM matches, terminology highlights, display hints, and ContentRef links alongside the editor.

### Flow Editor

Built on React Flow (@xyflow/react v12) for drag-and-drop visual flow building with color-coded node types, validation, and template support.

See [Bowrain UI Components](/docs/notes/bowrain-ui-components) for translation editor modes, context panel details, and flow editor specifics.

### Terminology Module

Dedicated terminology management interface with faceted search, concept editing, import/export, analytics, and editor integration ([AD-010](./010-terminology.md)).

### Translation Memory Explorer

TM entry browsing with fuzzy match visualization, entity mapping display, and TMX import/export ([AD-009](./009-translation-memory.md)).

See [Bowrain UI Components](/docs/notes/bowrain-ui-components) for terminology module and TM explorer details.

### Embedded Translation UI Concept

For design tools and CMS platforms, a lightweight Bowrain panel can be embedded
within the host application. This enables translators to work in context (seeing the
actual design or web page) while translating.

The embedded UI is a WebView rendering a subset of the Bowrain translation
editor, connected to the host application's connector
([AD-005](./005-connector-system.md)) via a bidirectional message channel.
When a translator edits a translation in the embedded panel, the change
propagates through the connector to update the Content Store. When source
content changes in the host application, the embedded panel updates to reflect
the new content.

This approach extends the connector concept beyond data exchange to become a
full in-context translation experience within native tools.

### Workspace Switcher (Slack-like Sidebar)

Bowrain uses a Slack-inspired two-panel sidebar layout: a 60px workspace rail and a 220px main sidebar for navigation within the active workspace.

### Shared Component Library (`packages/ui/`)

Core UI components are extracted to `packages/ui/` (`@neokapi/ui`) for reuse across Bowrain (desktop) and the web app, with platform-specific API adapters.

See [Bowrain UI Components](/docs/notes/bowrain-ui-components) for workspace rail layout, shared component library details, and API adapter architecture.

### Plugin Manager

Install/update plugins from the plugin registry
([AD-007](./007-plugin-system.md)). Display installed formats, tools, and
connectors with version information. One-click updates when new versions are
available.

### Server Connection and Collaborative Mode

Bowrain operates in two modes: **local** (standalone, no server) and
**connected** (collaborative, via a `bowrain-server` instance). The Go backend
acts as a smart proxy — when connected, it forwards operations to the server via
gRPC; when offline or local, it uses the local SQLite ContentStore directly. The
frontend only talks to the Go backend via Wails IPC and is unaware of the
communication layer.

```
Desktop Frontend ──Wails IPC──► Desktop Go Backend ──gRPC──► Bowrain Server
```

**Connection flow:**

1. **Server URL** — Translator enters the bowrain-server URL. The backend
   discovers the gRPC port via `GET /api/v1/grpc-info`
2. **Device Auth** — OAuth Device Flow (RFC 8628): backend requests a device
   code, displays a user code + verification URL, polls for authorization
3. **Workspace Selection** — After auth, the translator picks a workspace from
   the server
4. **Project Dashboard** — Lists server projects (read-only — project creation
   happens on the server). "New Project" is hidden in connected mode

**Connection states:** `disconnected` → `connecting` → `connected` ↔ `offline`.
The header displays the current state with visual indicators: green "Connected"
badge with user name, yellow "Offline" badge with pending changes count, or
grey "Local" when working standalone.

**Auth persistence:** Credentials are stored at
`<UserConfigDir>/bowrain-desktop/auth.json` and auto-loaded on startup for seamless
reconnection.

**gRPC Client (`ServerClient`):** Wraps the generated `EditorServiceClient`
with typed Go methods for all editor operations — projects, blocks, TM,
terminology, review, and presence. Uses `grpc.WithPerRPCCredentials` for
automatic JWT injection.

### Real-Time Collaboration

When connected, the `ProjectWatcher` opens a server-side gRPC stream
(`WatchProject`) that delivers real-time events:

- **Block changes** — `BlockChangeEvent` with block IDs, item name, change
  type ("created", "updated", "deleted"), and the name of the user who made the
  change. The frontend receives these as `blocks-changed` Wails events and
  re-fetches affected blocks
- **Presence** — `PresenceChangeEvent` indicating which users are viewing or
  editing which items and blocks. Displayed as user avatars in the project
  header and colored indicators next to blocks being edited by others

The watcher auto-reconnects on network errors with exponential backoff. It
starts when a project is opened in connected mode and stops on project
navigation or disconnect.

**Presence reporting:** The frontend calls `UpdatePresence(itemName, blockID)`
when the translator navigates between items or starts editing a block. This
is forwarded to the server, which broadcasts the update to all watchers of the
same project.

### Offline Layer

When the gRPC connection drops, the app transitions to `offline` state with a SQLite-backed offline queue that persists mutations for FIFO replay on reconnection. Changes are never lost, even across app restarts.

See [Bowrain UI Components](/docs/notes/bowrain-ui-components) for offline queue operations, reconnection algorithm, and frontend indicators.

### Project Format

Projects on the local ContentStore use SQLite
([AD-003](./003-content-store.md)). When connected to a server, the local
store acts as a read cache. In fully local mode, it is the source of truth.

## Alternatives Considered

- **Electron**: Large binary (~100MB+); ships entire Chromium + Node.js; Go
  backend would require IPC bridge.
- **Flutter**: Dart language mismatch; Go FFI complexity.
- **Tauri**: Rust-native; Go integration requires FFI or separate process.
- **Terminal UI (tview/bubbletea)**: Insufficient for translation editing with
  inline tags, terminology highlights, and document previews.
- **Web-only**: Loses native file system access and single-binary distribution.
  A web deployment can still be served by the REST API server, but the desktop
  app provides the best translator experience.

## Consequences

- Single binary: Go runtime + webview (WKWebView on macOS, WebView2 on Windows,
  GTK WebKit on Linux); binary size ~20-30MB
- Frontend developers use familiar React/TypeScript tooling
- Wails auto-generates TypeScript bindings from Go method signatures
- Connector-driven workflow positions Bowrain as a platform client, not just a
  file editor. The same UI works for CMS content, design tool text, code
  repositories, and local files ([AD-005](./005-connector-system.md))
- Embedded translation concept enables in-context translation within native
  design and CMS tools, a significant differentiator
- Collaborative mode via gRPC gives real-time multi-user editing with presence
  awareness. The smart proxy pattern (local cache + offline queue) ensures the
  translator can always work, regardless of network state
- Device auth flow provides a secure, clipboard-friendly login experience
  without requiring the desktop app to open a browser-based redirect
- Offline-first architecture: SQLite-backed queue guarantees zero data loss
  during network outages with automatic FIFO replay on reconnection
- Hot reload in development via `wails3 dev`
- Playwright E2E tests validate UI workflows in CI
- Locale selectors show friendly names via the `locale` package
  ([AD-001](./001-vision.md))
