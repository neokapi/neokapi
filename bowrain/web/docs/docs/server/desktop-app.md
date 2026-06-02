---
title: The desktop app
sidebar_label: The desktop app
description: Bowrain Desktop is a native, offline-capable client for the same server-hosted workspace as the web client — a working copy of the server, never a separate source of truth.
keywords: [desktop, offline, working copy, native app, bowrain]
---

# The desktop app

Everything in this section describes one product served from one place — the
Bowrain server. You reach a workspace through two clients: a browser and a
native **desktop app**. They are equal real-time clients of the same server, so
presence, edits, reviews, terminology, and translation memory are shared live
across both. The pages on workspaces, the editor, memory, terminology, brand
governance, and connectors apply to both clients; this page covers only what is
**different** about the desktop app.

## A working copy of the server

The desktop app is a **working copy of the server, never a separate source of
truth**. The authoritative content, translation memory, terminology, and project
configuration live on the server; the desktop holds only:

- a **content cache**, so a project opens fast and stays readable offline, and
- an **offline edit queue**, which holds your changes when the network drops and
  replays them to the server, in order, on reconnect.

Neither is a source of truth. If the local cache is cleared, the desktop
re-fetches everything from the server. Authoring local files and project
configuration — the `.kapi` recipe with its content, flows, plugins, languages,
and brand — is [kapi's job](/getting-started/kapi-vs-bowrain), not the desktop
app's.

## What the desktop adds: offline editing

The browser client is always online. The desktop client keeps working when the
network drops — on a flight, in a tunnel, or during server maintenance. Edits,
reviews, and memory updates made offline queue locally and replay in order the
moment you reconnect, then rejoin the live session. You never lose work to a
dropped connection, and you do not have to think about sync — it happens on
reconnect. See [Real-time collaboration](/server/collaboration) for the shared
presence and live-update model both clients use.

## Web and desktop, in sync

Both clients connect to the same server and the same workspaces. Which you reach
for is a matter of where you work, not what you can do.

| | **Browser** | **Desktop app** |
| --- | --- | --- |
| Install | Nothing — open a URL | Native app (macOS / Linux / Windows) |
| Real-time presence & live updates | Yes | Yes |
| Translation editor, TM & terminology explorers | Yes | Yes |
| Brand, terminology & TM context | Yes | Yes |
| AI/MT translation | Yes | Yes |
| Works offline | No (always online) | **Yes** — edits queue, sync on reconnect |
| Local footprint | Browser session | **Cache only** — content cache, offline edit queue, TM/termbase mirrors |
| Best for | Quick access, reviewers, occasional contributors | Daily translators, large files, unreliable connectivity |

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
4. Pick a workspace, then open a project from its project list.

Until you connect, the rail shows a single placeholder **Personal** workspace.
The desktop does not author local-file projects there — creating and configuring
projects is [kapi's job](/getting-started/kapi-vs-bowrain). Connect to a server
to open real projects. Once connected, its workspaces, projects, and resources
are **the server's** — the same ones the team works on in the browser. See
[Workspaces & members](/server/workspaces) for the role and permission model,
and [The editor](/server/translation-editor) for the editing surfaces both
clients share.

## Remote connectors only

Pulling content from a CMS, a git host, or a design tool is configured
server-side through [connectors](/server/connectors); both clients edit the
resulting project. The desktop app itself offers only **remote/CMS** connectors
(WordPress, Figma, HubSpot). The local-filesystem connectors (file, git) are
registered **server-side only** — sourcing content from a filesystem or a git
checkout is a server-side concern, and a local codebase syncs in through kapi
(`kapi push` / `kapi pull`).

## File export

In the browser, **Export** triggers a file download. On the desktop, the file is
saved to disk and opened in your system file manager. The export itself produces
the same file in its original format with all translations applied. See
[The editor](/server/translation-editor#file-export).

## Platform

Bowrain Desktop runs as a single native application on macOS, Windows, and
Linux — no additional runtimes or dependencies are needed.
