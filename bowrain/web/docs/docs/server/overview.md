---
sidebar_position: 1
title: Overview
---

# Bowrain Server

Bowrain Server is the shared, governed home for the work a team does locally with kapi. It is to kapi what GitHub is to git — the server holds the project history, shared terminology, translation memory, and team access; local kapi instances push and pull against it.

## What the server adds

A team running kapi on its own has powerful local tools. Bowrain Server adds what a local checkout cannot:

- **Shared terminology and memory** — one authoritative glossary and translation memory for the whole workspace, versioned and auditable
- **Multi-user editing** — translators work in the web or desktop editor; changes reach every connected client
- **Connectors** — sync against CMS, design tools, and code repositories, not just local files
- **Server-side automation** — event-driven rules run translation flows, quality gates, and notifications when content arrives
- **Workspace access control** — role-based membership (owner, admin, member, viewer) across multiple workspaces

## Workspaces

Each workspace is an isolated environment with its own projects, members, translation memory, and terminology. One Bowrain Server can host any number of workspaces.

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

Deploy Bowrain Server when a team needs:

- Multiple translators or reviewers sharing the same project
- Connectors to CMS, design tools, or code repositories
- Server-side automation triggered by content changes
- A single governed translation memory shared across projects
- Role-based access control

For solo work or local-only workflows, kapi on its own is sufficient — no server required.

## Deployment

See [Installation](/server/installation) for Docker and native binary setup, and [Self-Hosting](/server/self-hosting) for production configuration with TLS, persistent storage, and backups.

## Next Steps

- [Installation](/server/installation)
- [Configuration](/server/configuration)
- [Workspaces](/server/workspaces)
- [Connectors](/server/connectors)
- [Automation](/server/automation)
