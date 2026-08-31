---
sidebar_position: 1
title: Overview
description: Bowrain Server is where a team's content and its context live. It ingests source through connectors, checks and drafts it, and serves the shared context (profiles, terms, content memory) to people and AI tools alike.
---

# Bowrain Server

Bowrain Server is where a team's content and its context live. It ingests source
through [connectors](/server/connectors), content platforms, design tools, code
repositories, and a developer's checkout, checks and drafts it against the
context in force at each point, and serves that governed context (profiles,
terms, content memory) to people and AI tools alike, with full version history.

## What the server adds

Bowrain Server adds what a single source system, or a single local checkout,
cannot:

- **Shared context**: one set of profiles, one terms store and one content
  memory for the whole workspace, versioned and auditable
- **Governed review**: one review session per project, decisions on the
  record, and ship states per locale
- **Multi-user editing**: people work in the web or desktop editor; changes
  reach every connected client
- **Connectors**: every route in, from content platforms and design tools to
  repositories and a developer's checkout
- **Server-side automation**: event-driven rules run flows, quality gates, and
  notifications when content arrives
- **Workspace access control**: role-based membership (owner, admin, member,
  viewer) across multiple workspaces, with grants bound to a language or a
  point

## Workspaces

Each workspace is an isolated environment with its own projects, members,
profiles, terms and content memory. One Bowrain Server can host any number of
workspaces.

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

Deploy Bowrain Server when the graph has to reach further than one checkout:
several people on the same project, content spread across systems, or
vocabulary and memory that should compound across projects. For one person's
work on files in one checkout, the [kapi toolchain](/getting-started/kapi-vs-bowrain)
holds the same graph on its own, with no server required.

## Deployment

See [Installation](/server/installation) for Docker and native binary setup, and
[Self-Hosting](/server/self-hosting) for production configuration with TLS,
persistent storage, and backups.

## Next Steps

- [Installation](/server/installation)
- [Configuration](/server/configuration)
- [Workspaces](/server/workspaces)
- [Connectors](/server/connectors)
- [Automation](/server/automation)
