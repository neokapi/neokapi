---
sidebar_position: 1
title: Overview
---

# Bowrain Desktop App

Bowrain is a cross-platform desktop application for visual translation editing, built with [Wails v3](https://wails.io/), React 19, and TypeScript.

## Features

- **Translation editor** with inline code/tag support, semantic tag validation, and block preview
- **Flow editor** for drag-and-drop tool chain building
- **Translation Memory explorer** with fuzzy match visualization
- **Plugin manager** for install/update from registry
- **Batch file manager** with per-file locale/format configuration
- **Progress tracking** with real-time progress bars

## Technology Stack

| Component | Technology |
|-----------|-----------|
| Framework | Wails v3 |
| Backend | Go (full gokapi library access) |
| Frontend | React 19, TypeScript, Vite |
| Styling | TailwindCSS, shadcn/ui |
| Testing | Playwright (E2E) |

## Project Format

Bowrain uses the `.kaz` archive format as its native project format. Projects can be opened from the CLI:

```bash
bowrain project.kaz
```

## Architecture

The Go backend exposes methods as auto-generated TypeScript bindings. Go can emit events (`flow-complete`, `progress-updated`) for real-time UI updates and open native file dialogs.

Single binary distribution: Go runtime + webview (WKWebView on macOS, WebView2 on Windows, GTK WebKit on Linux). No Node.js or Chromium shipped; binary size is ~20-30MB.

See [ADR-012](/docs/adr/012-bowrain-desktop-app) for the design rationale.
