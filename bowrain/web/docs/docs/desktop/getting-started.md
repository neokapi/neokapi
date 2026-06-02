---
sidebar_position: 2
title: Getting started
---

# Getting started with the desktop app

Bowrain Desktop is a [working copy of the server](/desktop/overview): you sign
in to a workspace, open a project that lives on the server, and edit it
alongside the web app. You do not create local-file projects in the desktop —
that is [kapi's job](/getting-started/kapi-vs-bowrain).

## Installation

### macOS (Homebrew)

```bash
brew install --cask neokapi/tap/bowrain
```

### Binary download

Download the latest release from [GitHub Releases](https://github.com/neokapi/neokapi/releases):

- **macOS**: DMG (Apple Silicon)
- **Windows**: signed installer (`bowrain-X.Y.Z-windows-amd64-setup.exe` or `-arm64-setup.exe`)
- **Linux**: tarball (amd64, arm64)

## First sign-in

1. Launch Bowrain.
2. Open **Settings** and enter your server URL (e.g. `https://bowrain.example.com`).
3. Click **Connect** — this opens the login flow in your browser. After
   authorization, your workspaces appear in the workspace rail.
4. Pick a workspace, then open a project from its project list. On the BowMart
   workspace, Maya opens the `store-frontend` project.

The project's files and blocks load from the server, and teammates' presence
appears live. See [Workspaces](/desktop/workspaces) for navigation and
[Projects](/desktop/projects) for how server projects open in the desktop.

![Editor showing 100% translated blocks](/img/bowrain/dark/editor-translated.png)

## Editing surfaces

The desktop opens three per-file surfaces — the same as the web app:

- **Translate**: edit blocks with AI/TM assistance, in a **Visual** view (an inline card over a formatted document preview) or a **Table** view (a source/target grid for scanning and editing many blocks).
- **Review**: work through blocks by status, run QA, and approve or reject translations.
- **Pre-process**: file-wide source prep (pseudo-translate, bulk TM leverage) before editing.

See the [Translation editor](/server/translation-editor) for the full detail.

## Block status

Each translation block has a status that is tracked automatically:

| Status      | Indicator | Condition                                 |
| ----------- | --------- | ----------------------------------------- |
| Not started | Gray      | No target text                            |
| Draft       | Yellow    | Has target text but no translation origin |
| Translated  | Blue      | Translation origin is set (AI, TM, etc.)  |
| Reviewed    | Green     | Manually marked as reviewed               |

The progress bar at the top of the editor shows the distribution of block statuses.

## Keyboard shortcuts

| Shortcut               | Action                               |
| ---------------------- | ------------------------------------ |
| `Cmd/Ctrl+Enter`       | Confirm translation and move to next |
| `Cmd/Ctrl+Shift+Enter` | Copy source to target                |
| `Cmd/Ctrl+Shift+R`     | Mark block as reviewed               |

Edits commit to the server as you work (and queue locally when offline). There
is no separate "save to a local file" step — the project's authoritative state
lives on the server.

## Translation memory and terminology

A project's translation memory and terminology are **hosted on the server** and
shared across the workspace, so Maya and Jonas always see the same memory and
the same approved terms. The desktop keeps a local mirror for fast, offline
lookups; it is a cache, never the source of truth.

- **Context panel**: in the editor toolbar, click **Context** to see per-block
  TM matches (score, source, target, match type) and terminology suggestions
  (matched term, target suggestions, domain, lifecycle status). Click **Apply**
  on a TM match to insert it.
- **TM lookup**: in the editor toolbar, click **TM Lookup** to batch-apply TM
  matches to all untranslated blocks.
- **Explorers**: from the navigation panel, open **Memory** to browse, search,
  and edit TM entries, or **Termbase** to manage concept-oriented terminology
  with multi-locale terms and lifecycle statuses (preferred, approved, admitted,
  deprecated, proposed, forbidden).

Changes made here are committed to the server and visible to the rest of the
workspace. See [Translation memory](/server/translation-memory) and
[Terminology](/server/terminology) for the full reference.
