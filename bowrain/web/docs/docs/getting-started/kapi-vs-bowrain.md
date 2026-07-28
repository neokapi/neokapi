---
sidebar_position: 6
title: How Bowrain and kapi fit together
sidebar_label: Bowrain and kapi
description: Bowrain is the governed platform; kapi is the open toolchain it is built on and one of the connectors into it. Where the boundary runs, and which surface owns what.
keywords: [kapi, bowrain, boundary, toolchain, platform, connector, open source]
---

# How Bowrain and kapi fit together

[Kapi](https://neokapi.github.io/) is the Apache-2.0 toolchain
Bowrain is built on: it reads and writes content formats, runs checks, drafts
and adapts text, and works entirely from local files with no server or account.
Bowrain is the governed platform — shared voice, vocabulary, and content memory,
collaborative editing, review, automation, connectors, and version history.

kapi appears in two roles here, and they are worth keeping apart:

- **The engine underneath.** The same format handling, checks, and flow
  execution run inside Bowrain's server. This is invisible in day-to-day use;
  it is why the platform behaves identically whether content arrived from a
  content platform or from a repository.
- **One connector into the platform.** With the bowrain plugin installed, kapi
  connects a developer's checkout to a workspace — the developer and CI route
  described in [the kapi connector](/server/connectors/kapi). It sits alongside
  the content-platform, design, and repository connectors rather than in front
  of them.

## The boundary

**kapi owns the local files and the project configuration.** The `kapi.yaml`
recipe — content collections, flows, plugins, languages, brand, and the
`server:` block — is authored and versioned in the repository with everything
else. Bowrain never writes it.

**Bowrain's local footprint is cache and speed only, never a source of truth.**
The Bowrain desktop app is a working copy of the server: a content cache, an
offline edit queue, and memory and vocabulary mirrors. It does not author local
files or source projects from a filesystem — sourcing from a filesystem or a git
checkout happens *server-side* through [connectors](/server/connectors), on the
host the server runs on.

## At a glance

| | **kapi** | **Bowrain** |
| --- | --- | --- |
| Shape | A CLI + desktop app you install | A server + web and desktop clients |
| Users | One person, one checkout | One workspace — you, and your team when you have one |
| State | Local files + local memory and terms | Hosted, versioned content store (the desktop holds a **cache** only) |
| Brand & vocabulary | A profile you carry in files | Shared, governed, and **learned from corrections** |
| Content sources | Local files you own | Every [connector](/server/connectors) — content platforms, design tools, repositories, checkouts |
| Automation | Local recipe hooks | Server-side, event-driven |
| Cost | Free, open source | Hosted plans / self-host |

## kapi on its own is enough when

- One person works from one repository they own.
- The work is drafting, checking, or pseudo-translating files from a terminal or
  a desktop app — no account, offline by default.
- Checks and language work are being wired into CI, or into an AI assistant over
  MCP.

## Reach for Bowrain when

- Content lives in systems beyond one checkout — a content platform, a design
  tool, a repository nobody wants a pipeline in — and should sync through
  [connectors](/server/connectors).
- Several projects or several surfaces should draw on **one** memory,
  vocabulary, and voice that compound across all of them, kept current on the
  server rather than only when someone runs a command.
- Several people — writers, reviewers, editors — work on the same content and
  need to [see each other and edit together](/server/collaboration).
- One brand voice and vocabulary should be **shared and governed** across
  everyone and every AI tool, with history and audit.
- Corrections should compound: a fix made once becomes a
  [versioned, enforced check](/server/brand-voice).

The two are not alternatives, and neither is a stage you graduate from. See the
[introduction](/introduction) for what the platform does, and
[Connectors](/server/connectors) for every route into it.
