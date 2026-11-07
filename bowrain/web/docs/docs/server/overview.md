---
sidebar_position: 1
title: Overview
description: Bowrain Server is where a team's content and its context live. It ingests source through connectors, checks and drafts it, and serves the shared context (profiles, terms, content memory) to people and AI tools alike.
---

# Bowrain Server

Bowrain Server holds a team's content and its context. It takes in source
through [connectors](/server/connectors) from content platforms, design tools,
code repositories and developers' checkouts. It checks and drafts that content
against the context in force at each point, and serves the profiles, terms and
content memory to people and AI tools, with full version history.

## What the server adds

Beyond a single source system or a single local checkout, the server adds:

- **Shared context**: one set of profiles, one terms store and one content
  memory for the whole workspace, versioned and auditable
- **Governed review**: one review session per project, decisions on the
  record, and ship states per locale
- **Multi-user editing**: people work in the web or desktop editor; changes
  reach every connected client
- **Connectors**: every route in, from content platforms and design tools to
  repositories and a developer's checkout
- **Server-side automation**: event-driven rules run flows and send
  notifications when content arrives or a language's ship-gate result changes
- **Workspace access control**: role-based membership (owner, admin, member,
  viewer) across multiple workspaces, with grants bound to a language or a
  point

## Workspaces

Each workspace is an isolated environment with its own projects, members,
profiles, terms and content memory. One Bowrain Server can host any number of
workspaces. For example:

| Workspace | Projects | Members |
| --- | --- | --- |
| `acme` | Website, Mobile App | alice, bob, carol |
| `contoso` | Documentation | dave, eve |

## When to run a server

Deploy Bowrain Server when the context graph has to reach further than one
checkout:

- several people work on the same project;
- content is spread across systems;
- terms and memory should build up across projects.

For one person working on files in one checkout, the
[kapi toolchain](/getting-started/kapi-vs-bowrain) holds the same graph with no
server.

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
