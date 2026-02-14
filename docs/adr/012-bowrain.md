---
id: 012-bowrain
sidebar_position: 12
title: "ADR-012: Bowrain Desktop App"
---
# ADR-012: Bowrain desktop app with Wails v3

## Context

A desktop GUI is needed for translators who prefer visual editing over CLI
workflows. The app must access all gokapi functionality: connectors, content
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
([ADR-005](./005-connector-system.md)) are the integration mechanism that
connects Bowrain to content sources:

1. **Connect** -- Configure a connector (CMS, design tool, code repo, or local
   files) with API keys, endpoints, and authentication credentials
2. **Browse** -- List available content items from the connected system via
   `Connector.List()`, displayed as a browsable tree
3. **Select** -- Choose which items to pull for translation, with locale and
   content type filters
4. **Pull** -- Extract content into the Content Store
   ([ADR-003](./003-content-store.md)) via `Connector.Pull()`, producing the
   same Part stream used throughout the pipeline
5. **Translate** -- Work in the translation editor with TM
   ([ADR-009](./009-translation-memory.md)), terminology
   ([ADR-010](./010-terminology.md)), and AI assistance
   ([ADR-008](./008-ai-integration.md))
6. **Push** -- Send translations back to the source system via
   `Connector.Push()`

For file-based workflows, the FileConnector ([ADR-005](./005-connector-system.md))
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

Four layout modes:

- **Grid** (default) -- Table view with source and target columns for all
  blocks. Efficient for scanning and bulk editing
- **Focus** -- Single-block deep editing with full-width source/target panels
  and block navigation (prev/next). Term highlights and TM matches are shown
  inline
- **Split Horizontal / Vertical** -- Side-by-side or stacked editing with a
  live document preview pane

Block status tracking: `not-started`, `draft` (has target text), `translated`
(translation-origin property set), `reviewed` (translation-status is
"reviewed"). A status-colored progress bar shows block completion at a glance.

Enhanced toolbar: Copy Source to Target, Mark as Reviewed, Navigate to
Prev/Next Untranslated.

### Context Panel

Rich metadata display alongside the translation editor, providing translators
with the information they need without leaving the editing view:

- **TM matches** -- Fuzzy and exact matches from Bowrain Memory
  ([ADR-009](./009-translation-memory.md)) with match scores, entity
  adaptations, and one-click application to the target field
- **Terminology** -- Recognized terms highlighted inline in the source text
  with definitions, preferred translations, and status. Click to navigate to the
  concept in the terminology module ([ADR-010](./010-terminology.md))
- **Display hints** -- Content type, max length, and preview from the source
  system, carried on Blocks via the content model
  ([ADR-002](./002-content-model.md))
- **ContentRef** -- Link back to the source item in the connected system,
  enabling translators to view content in its original context
  ([ADR-002](./002-content-model.md))

### Flow Editor

Built on React Flow (@xyflow/react v12) for drag-and-drop visual flow building:

- Nodes represent readers, tools, and writers with color-coded type indicators
  (Transform, Enrich, Validate)
- Edges represent the data flow between nodes
- Add/remove nodes and edges interactively
- Built-in flows available as templates; user-created flows are persisted via
  FlowStore
- Flow definitions are validated (cycle detection via TopologicalOrder, node
  reference integrity) before saving

### Terminology Module

Dedicated terminology management interface
([ADR-010](./010-terminology.md)):

- **Browse and search** -- Faceted search by locale, domain, product, status.
  Concept graph visualization for related terms. Term concordance showing all
  Blocks where a term appears in the current project
- **Edit and review** -- Create/edit concepts and terms. Inline term suggestion
  during translation (right-click selected text to suggest a new term).
  Moderation queue for proposed terms with approval workflow
- **Import and export** -- CSV, TBX, JSON import with field mapping UI. Export
  in any supported format. Merge imported terms with existing termbase via
  conflict resolution UI
- **Analytics** -- Term usage statistics across project content. Coverage
  (percentage of source terms with approved translations). Consistency
  (preferred terms vs. variants in target text). Freshness (terms not reviewed
  since configurable date)
- **Editor integration** -- Recognized terms highlighted inline in the
  translation editor. Hover to see definition, preferred translation, and
  status. Click to navigate to the term in the terminology module

### Translation Memory Explorer

- Fuzzy match visualization with score indicators
- TM entry browsing with entity mapping display showing generalized vs. plain
  matches ([ADR-009](./009-translation-memory.md))
- TMX import/export for interoperability with external TM systems

### Embedded Translation UI Concept

For design tools and CMS platforms, a lightweight Bowrain panel can be embedded
within the host application. This enables translators to work in context (seeing the
actual design or web page) while translating.

The embedded UI is a WebView rendering a subset of the Bowrain translation
editor, connected to the host application's connector
([ADR-005](./005-connector-system.md)) via a bidirectional message channel.
When a translator edits a translation in the embedded panel, the change
propagates through the connector to update the Content Store. When source
content changes in the host application, the embedded panel updates to reflect
the new content.

This approach extends the connector concept beyond data exchange to become a
full in-context translation experience within native tools.

### Workspace Switcher (Slack-like Sidebar)

Bowrain uses a Slack-inspired two-panel sidebar layout for workspace navigation:

- **Workspace Rail** (60px, dark background) -- Far-left icon rail showing
  workspace icons (first letter or custom logo with colored badge). Active
  workspace has a pill-shaped highlight. Bottom section has "+" button for
  creating workspaces and a user avatar with account menu
- **Main Sidebar** (220px) -- Navigation panel for the active workspace with
  sections: Translate (project list), Termbase, Memory (TM), Flows, Connectors,
  Settings. Below navigation: collapsible project list for quick switching

```
┌──────┬──────────┬────────────────────────────────────────┐
│ Rail │ Sidebar  │  Main Content                          │
│ 60px │  220px   │                                        │
│ [WS] │ Translate│  (ProjectDashboard/Editor/TM/Terms)    │
│ [WS] │ Termbase │                                        │
│      │ Memory   │                                        │
│  +   │ Flows    │                                        │
│ [AV] │ Settings │                                        │
└──────┴──────────┴────────────────────────────────────────┘
```

In local/desktop mode, a "Personal" workspace is created automatically. When
connected to a `bowrain-server` instance, the rail shows all workspaces the user
belongs to with role-based access ([ADR-015](./015-auth-and-workspaces.md)).

### Shared Component Library (`packages/ui/`)

Core UI components are extracted to `packages/ui/` (`@gokapi/ui`) for reuse
across Bowrain (desktop) and the web app. The library includes:

- **Layout**: `WorkspaceRail`, `MainSidebar`, `AccountMenu`, `WorkspaceIcon`
- **Context**: `AuthContext`, `WorkspaceContext` with React hooks
- **API Adapter**: `ApiAdapter` interface with platform-specific implementations
  -- `RestApiAdapter` for the web app, Wails bindings for desktop

Bowrain imports from `@gokapi/ui` and provides a Wails-specific API adapter that
bridges Go method calls to the shared `ApiAdapter` interface. This ensures the
same UI components render identically in both desktop and browser contexts.

### Plugin Manager

Install/update plugins from the plugin registry
([ADR-007](./007-plugin-system.md)). Display installed formats, tools, and
connectors with version information. One-click updates when new versions are
available.

### Project Format

The Bowrain app uses the `.kaz` archive format
([ADR-003](./003-content-store.md)) as its native project format. KAZ archives
embed content, TM snapshots, and terminology snapshots for offline work.
Projects can be opened from the CLI via `bowrain project.kaz`.

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
  repositories, and local files ([ADR-005](./005-connector-system.md))
- Embedded translation concept enables in-context translation within native
  design and CMS tools, a significant differentiator
- Hot reload in development via `wails3 dev`
- Playwright E2E tests validate UI workflows in CI
- Locale selectors show friendly names via the `core/locale` package
  ([ADR-001](./001-vision.md))
