---
id: 012-bowrain-desktop-app
sidebar_position: 12
title: "ADR-012: Bowrain Desktop App"
---

# ADR-012: Bowrain desktop app with Wails v3

## Context

A desktop GUI is needed for translators who prefer visual editing over CLI
workflows. The app must access all gokapi functionality: format readers,
tools, flows, plugins, TM, and AI providers.

Key requirements:

- Cross-platform (macOS, Linux, Windows)
- Single binary distribution
- Full access to Go backend (registries, executors, plugins)
- Modern UI framework for a responsive editing experience

## Decision

### Technology Stack

Use [Wails v3](https://wails.io/) with a React 19 / TypeScript frontend.

**Go backend** exposes methods as auto-generated TypeScript bindings:
`ListFormats()`, `ExecuteFlow()`, `LoadProject()`, `SaveProject()`, etc.
Go can emit events (`flow-complete`, `progress-updated`) for real-time UI
updates and open native file dialogs.

**Frontend** uses React 19, Vite (fast HMR), TailwindCSS, and shadcn/ui
components. Playwright provides E2E test coverage.

Previously the app used Wails v2 but was migrated to Wails v3 for improved
API stability and ES module support.

### Key UI Features

- **Translation editor** with four layout modes:
  - *Grid* (default) — table view with source and target columns for all blocks
  - *Focus* — single-block deep editing with full-width source/target panels
    and block navigation (prev/next)
  - *Split Horizontal* / *Split Vertical* — side-by-side or stacked editing
    with a live document preview pane

  Block status tracking: `not-started`, `draft` (has target text),
  `translated` (translation-origin property set), `reviewed`
  (translation-status is "reviewed"). A status-colored progress bar shows
  block completion at a glance.

  Enhanced toolbar with: Copy Source to Target, Mark as Reviewed, Navigate
  to Prev/Next Untranslated.

- **Flow editor** built on React Flow (@xyflow/react v12) for drag-and-drop
  visual flow building:
  - Nodes represent readers, tools, and writers with color-coded type
    indicators
  - Edges represent the data flow between nodes
  - Add/remove nodes and edges interactively
  - Built-in flows available as templates; user-created flows are persisted
    via FlowStore
  - Flow definitions are validated (cycle detection via TopologicalOrder,
    node reference integrity) before saving

- **Action tools integration** — built-in action tools (Segmentation,
  QA Check, TM Leverage) are available in the flow builder. Tools appear
  as nodes in the flow graph with their category color coding (Transform,
  Enrich, Validate).

- **Translation Memory explorer** with fuzzy match visualization
- **Plugin manager** for install/update from registry
- **Batch file manager** with per-file locale/format configuration
- **Progress tracking** with real-time progress bars

### Project Format

The Bowrain app uses the `.kaz` archive format ([ADR-011](/docs/adr/011-kaz-archive-format)) as its native
project format. Projects can be opened from the CLI via
`bowrain project.kaz`.

## Alternatives Considered

- **Electron**: large binary (~100MB+); ships entire Chromium + Node.js;
  Go backend would require IPC bridge.
- **Flutter**: Dart language mismatch; Go FFI complexity.
- **Tauri**: Rust-native; Go integration requires FFI or separate process.
- **Terminal UI (tview/bubbletea)**: insufficient for translation editing
  with inline tags and previews.

## Consequences

- Single binary: Go runtime + webview (WKWebView on macOS, WebView2 on
  Windows, GTK WebKit on Linux)
- No Node.js or Chromium shipped; binary size ~20-30MB
- Frontend developers use familiar React/TypeScript tooling
- Wails auto-generates TypeScript bindings from Go method signatures
- Hot reload in development via `wails3 dev`
- Playwright E2E tests validate UI workflows in CI
