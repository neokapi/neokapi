---
sidebar_position: 2
title: Kapi vs Bowrain
sidebar_label: Kapi vs Bowrain
description: When to use kapi (the local, single-user toolchain) and when to reach for Bowrain (the hosted, multi-user, governed platform on top of it). It is the same relationship as git and GitHub.
keywords: [kapi vs bowrain, when to use, comparison, platform, toolchain]
---

# Kapi vs Bowrain

[Kapi](https://neokapi.github.io/web/neokapi/) and Bowrain are not alternatives —
Bowrain is built on kapi. The split is the same as **git and GitHub**: the
toolchain runs anywhere and belongs to you; the platform adds the persistent,
multi-user, governed layer a team needs.

- **Kapi** is the open-source, single-user toolchain. It reads, translates, and
  ships content in any format, runs checks (terminology, QA, brand voice), and
  leverages translation memory — all locally, from files you own, with no server
  or account. It is also where your AI assistant plugs in, over MCP.
- **Bowrain** is the server platform. It hosts the brand-voice profile,
  terminology, and translation memory once for everyone; adds real-time
  collaboration, connectors, automation, and versioned history; and turns a
  team's corrections into enforced checks. You keep using kapi — the bowrain
  plugin lets a kapi project [sync](/cli/commands/sync) with a server.

The boundary is sharp: **kapi owns the local files and the project
configuration** — the `.kapi` recipe (content, flows, plugins, languages,
brand, and the `server:` block) is authored and versioned locally with kapi,
including the configuration of projects you push to Bowrain via `kapi push` /
`kapi sync`. **Bowrain's local footprint is cache and speed only, never a
source of truth.** The Bowrain desktop app is a working copy of the server: a
content cache, an offline edit queue, and TM/termbase mirrors. It does not
author local files or source projects from your filesystem — that is kapi's
job. Sourcing content from a filesystem or a git checkout happens *server-side*
through [connectors](/server/connectors), on the host the server runs on.

## At a glance

| | **Kapi** | **Bowrain** |
| --- | --- | --- |
| Shape | A CLI + desktop app you install | A server + web and desktop clients |
| Users | One person, one checkout | A team, one shared workspace |
| State | Local files + local TM/termbase | Hosted, versioned content store (desktop holds a local **cache** only) |
| Brand & terminology | A profile/glossary you carry in files | Shared, governed, and **learned from corrections** |
| Collaboration | — | [Real-time presence & co-editing](/server/collaboration) |
| Content sources | Local files you own | Server-side [connectors](/server/connectors) — CMS, git, design tools, files |
| Automation | Local recipe hooks | Server-side, event-driven |
| Cost | Free, open source | Hosted plans / self-host |

## Use kapi when

- You are a solo builder or a small team working from a repository you own.
- You want to localize files, check content, or pseudo-translate from the
  terminal or a desktop app — no account, offline by default.
- You are wiring localization and brand checks into CI, or into an AI assistant
  over MCP.

## Reach for Bowrain when

- Several people — writers, translators, reviewers — work on the same content
  and need to [see each other and edit together](/server/collaboration).
- One brand voice, glossary, and translation memory should be **shared and
  governed** across everyone and every AI tool, with history and audit.
- Content lives in systems beyond your local files (a CMS, a git host, a design
  tool) and should sync through server-side [connectors](/server/connectors).
- Corrections should compound: a fix made once becomes a
  [versioned, enforced check](/server/brand-voice).

You do not choose one forever. Start in kapi; connect a project to Bowrain when
the team and the governance needs arrive. See the
[introduction](/introduction) for how the two fit together.
