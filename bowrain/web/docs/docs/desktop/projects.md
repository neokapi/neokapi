---
title: Projects
sidebar_position: 4
---

# Projects in the desktop app

Projects in Bowrain Desktop live **on the server**, not in a local database. The
desktop is a [working copy of the server](/desktop/overview): you open a project
that belongs to a [workspace](/desktop/workspaces), edit it alongside the web
app, and your changes sync back. The authoritative content, version history, and
project configuration are all held server-side.

The desktop does not create or author local-file projects — that is
[kapi's job](/getting-started/kapi-vs-bowrain). A project's `.kapi` recipe
(content, flows, plugins, languages, brand) is authored and versioned locally
with kapi and pushed to the server with `kapi push`; the desktop then opens that
project as a live client.

## Opening a project

1. Sign in to your workspace (see [Workspaces](/desktop/workspaces)).
2. Pick a project from the workspace's project list. On the BowMart workspace,
   Maya opens the `store-frontend` project — the same one Jonas is reviewing in
   the browser.
3. The desktop loads the project's files and blocks from the server and shows
   teammates' presence live.

## Versions and history

Version snapshots are created and stored **server-side**, so every client sees
the same history. The desktop surfaces a project's versions and the differences
between them; it does not keep an independent local version store. See
[Real-time collaboration](/server/collaboration) for how concurrent edits merge.

## Where local state fits

The desktop's local footprint is **cache and offline queue only**:

- a content cache so the project opens fast and stays readable offline, and
- an offline edit queue that holds your changes when the network drops and
  replays them to the server on reconnect.

Neither is a source of truth. If the local cache is cleared, the desktop
re-fetches from the server.

## Content sources

Pulling content from a CMS, a git host, or a design tool is configured
server-side through [connectors](/server/connectors); the desktop edits the
resulting project. The desktop itself offers only **remote** connectors — see
[Connectors](/desktop/connectors).
