---
sidebar_position: 1
title: Overview
description: Bowrain Server is the governed platform where a team's multilingual content lives — it ingests source through connectors, drafts and checks it, and serves the shared context (vocabulary, content memory, and voice profiles) to people and AI tools alike.
---

# Bowrain Server

Bowrain Server is the governed platform where a team's multilingual content lives. It ingests source through [connectors](/server/connectors) — content platforms, design tools, code repositories, and a developer's checkout — drafts and checks it, and serves that governed context (vocabulary, content memory, voice profiles) to people and AI tools alike, with full version history.

## What the server adds

Bowrain Server adds what a single source system — or a single local checkout — cannot:

- **Shared vocabulary and memory** — one authoritative term set and content memory for the whole workspace, versioned and auditable
- **Multi-user editing** — people work in the web or desktop editor; changes reach every connected client
- **Connectors** — every route in, from content platforms and design tools to repositories and a developer's checkout
- **Server-side automation** — event-driven rules run flows, quality gates, and notifications when content arrives
- **Workspace access control** — role-based membership (owner, admin, member, viewer) across multiple workspaces

## Workspaces

Each workspace is an isolated environment with its own projects, members, content memory, and vocabulary. One Bowrain Server can host any number of workspaces.

```
workspace / acme
├── Project: Website
├── Project: Mobile App
└── Members: alice, bob, carol

workspace / contoso
├── Project: Documentation
└── Members: dave, eve
```

## When to run a server

Deploy Bowrain Server when the work outlives a single run: several people on the same project, content spread across systems, or vocabulary and memory that should compound rather than be rebuilt each time. For solo work on files in one checkout, the [kapi toolchain](/getting-started/kapi-vs-bowrain) on its own is sufficient — no server required.

## Deployment

See [Installation](/server/installation) for Docker and native binary setup, and [Self-Hosting](/server/self-hosting) for production configuration with TLS, persistent storage, and backups.

## Next Steps

- [Installation](/server/installation)
- [Configuration](/server/configuration)
- [Workspaces](/server/workspaces)
- [Connectors](/server/connectors)
- [Automation](/server/automation)
