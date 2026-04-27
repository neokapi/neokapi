---
id: 001-vision-and-modules
sidebar_position: 1
title: "AD-001: Bowrain Vision and Module Architecture"
---

# AD-001: Bowrain Vision and Module Architecture

## Summary

Bowrain is the full-stack localization platform built on the neokapi framework.
It adds a versioned content store, bidirectional connectors to live systems,
event-driven automation, multi-user workspaces, and collaborative editing on
top of the framework's streaming pipeline. Bowrain ships as three Go modules —
`bowrain/core` (shared platform types), `bowrain/cli` (project-sync companion
CLI), and `bowrain` (server, workers, desktop and web apps) — all licensed
under AGPL-3.0.

## Context

The neokapi framework is the Apache-licensed core: content model, formats,
tools, pipeline executor, plugin system, TM, terminology, and AI/MT providers.
It has zero platform dependencies — no database, no server, no authentication.
See [AD-framework-001: Vision and Module Architecture](https://neokapi.github.io/web/neokapi/docs/architecture/001-vision-and-modules).

Bowrain is the commercial-grade platform layered on top. It persists content,
syncs with live systems (CMS, design tools, code repos, marketing platforms),
runs multi-user workflows, and exposes a REST + gRPC server, a Wails desktop
app, a SaaS web UI, and admin control surfaces. The framework is one consumer
agnostic; Bowrain is opinionated about persistence, sync, connectors,
authentication, and collaboration.

The license boundary is structural: framework code is Apache-2.0 and never
depends on AGPL code; every module under `bowrain/` is AGPL-3.0. CI enforces
this with `GOWORK=off` builds against the framework module.

## Decision

### Identity

Bowrain is a localization platform, dual-licensed under AGPL-3.0 and a
commercial license. It is available as a SaaS offering and for open-source
self-hosting. Its job is to turn the framework's pipeline into a
production-ready, multi-user system with persistent state, real-time
collaboration, and integrations with the tools that already hold content.

Design principles:

1. **Connector-first.** Integration with live systems — CMS, design tools,
   code repositories, marketing platforms, TMS — is the primary integration
   mechanism. File formats are one connector category, not the whole story.
   See [AD-008: Connector System](008-connector-system.md).
2. **Content-addressable blocks.** Blocks are keyed by content hash, enabling
   deduplication across sources, incremental sync that only transfers changed
   content, and efficient version diffing. See
   [AD-004: Content Store and Versioning](004-content-store.md).
3. **Event-driven automation.** Every content mutation emits a typed event.
   Automation rules, connector publishers, graph syncers, and background
   workers subscribe to these events.
4. **Real-time collaboration.** The desktop app, web UI, and embedded panels
   all connect to a single server instance via gRPC streaming. Block edits,
   presence, and TM/terminology updates propagate to every connected client.
5. **Per-workspace multi-tenancy.** Workspaces are the top-level isolation
   unit. Every project, token, connector, blob, and background job carries a
   workspace ID. See [AD-002: Authentication and Workspaces](002-authentication-and-workspaces.md).

### Three Bowrain Modules

Bowrain is composed of three independent Go modules, each with its own
`go.mod`:

| Module         | Import Path                               | Directory       | Role                                                                       |
| -------------- | ----------------------------------------- | --------------- | -------------------------------------------------------------------------- |
| `bowrain/core` | `github.com/neokapi/neokapi/bowrain/core` | `bowrain/core/` | Shared platform types and interfaces: project model, auth, connector, store, event, client, agent |
| `bowrain/cli`  | `github.com/neokapi/neokapi/bowrain/cli`  | `bowrain/cli/`  | Project-sync companion CLI (`bowrain` binary): `.kapi` recipe init, push, pull, status, auth |
| `bowrain`      | `github.com/neokapi/neokapi/bowrain`      | `bowrain/`      | Server, workers, desktop and web apps, storage implementations             |

Dependency rules, verified in CI with `GOWORK=off` builds:

```
framework (root, Apache-2.0)
    ↑
    ├── cli (shared CLI base, framework only)
    │    ↑
    │    └── bowrain/cli (framework + cli + bowrain/core)
    │
    └── bowrain/core (framework only — pure types/interfaces)
         ↑
         ├── bowrain/cli (framework + cli + bowrain/core)
         └── bowrain (framework + bowrain/core)
```

Key constraints:

- `bowrain/core` depends only on the framework module. It contains pure types
  and interfaces — no Cobra, no Viper, no SQLite, no Wails, no Echo, no OIDC,
  no keychain bindings. Every package that wants to share platform types
  without taking on platform dependencies imports from `bowrain/core`.
- `bowrain/cli` depends on framework + shared CLI + `bowrain/core`. It is the
  only module that combines the shared CLI base with platform types.
- `bowrain` depends on framework + `bowrain/core`. It does NOT depend on the
  shared CLI module — the server and its apps don't need Cobra.
- Kapi and `bowrain` have no dependency on each other.

### Module Contents

**`bowrain/core/`** holds:

| Package     | Purpose                                                           |
| ----------- | ----------------------------------------------------------------- |
| `project/`  | Workflow facade pairing the framework's `KapiProject` recipe loader with the on-disk Layout: server-block helpers, sync cache, URL parsing |
| `auth/`     | Auth types (User, Workspace, Token), JWT handling, PKCE, device flow client |
| `connector/` | `ConnectorBase`, `IntegrationConnector`, `SourceConnector` interfaces |
| `client/`   | REST client for Bowrain Server                                    |
| `store/`    | `ContentStore` interface + domain types (Project, Version, Asset) |
| `event/`    | Event types + bus interface                                       |
| `agent/`    | Agent mode types, session grants                                  |
| `config/`   | Auth persistence helpers                                          |

**`bowrain/cli/`** holds:

| Package          | Purpose                                                  |
| ---------------- | -------------------------------------------------------- |
| `cmd/bowrain/`   | Root command wiring. Extends the shared CLI base with project commands (init, ls, add, rm, status, diff, pull, push, run, serve, auth, stream) |

The bowrain CLI sets the shared CLI base's `RegistryResolver` hook during
initialization to resolve plugin registries from the project's `.kapi` recipe.
The shared CLI module never imports `bowrain/core/project`.

**`bowrain/`** holds the full platform:

| Subdirectory             | Purpose                                                       |
| ------------------------ | ------------------------------------------------------------- |
| `cmd/bowrain-server/`    | REST (Echo v4) + gRPC API server                              |
| `cmd/bowrain-worker/`    | Background worker (async push, asset processing, etc.)        |
| `server/`                | HTTP/gRPC handlers, middleware chain                          |
| `service/`               | Business logic — auth, project, connector, flow services      |
| `auth/`                  | OIDC integration, `AuthStore` (SQLite + PostgreSQL)           |
| `store/`                 | `ContentStore` implementations (SQLite + PostgreSQL)          |
| `storage/`               | Shared SQLite + PostgreSQL migration utilities                |
| `connector/`             | Concrete connector implementations                            |
| `event/`                 | Event bus implementation + automation engine                  |
| `billing/`               | Subscription and quota management                             |
| `jobs/`                  | Background job processor                                      |
| `brand/`                 | Brand voice profiles, tag dimensions                          |
| `graph/`                 | Apache AGE and SQLite graph store backends                    |
| `analytics/`             | Usage analytics and reporting                                 |
| `sievepen/`              | SQLite + PostgreSQL TM implementation                         |
| `termbase/`              | SQLite + PostgreSQL termbase implementation                   |
| `proto/`                 | Protobuf definitions for gRPC and sync                        |
| `apps/bowrain/`          | Wails v3 desktop app (Go + React/TypeScript)                  |
| `apps/web/`              | SaaS web UI                                                   |
| `apps/ctrl/`             | Admin control panel                                           |
| `apps/pulse/`            | Public real-time dashboard                                    |
| `apps/keycloak-theme/`   | Custom Keycloak theme                                         |
| `packages/ui/`           | `@neokapi/ui` AGPL component library                          |

### Binaries and Apps

| Binary / App            | Module         | Role                                                     |
| ----------------------- | -------------- | -------------------------------------------------------- |
| `bowrain` CLI           | `bowrain/cli`  | Project-centric sync CLI, analogous to `git`             |
| `bowrain-server`        | `bowrain`      | Echo v4 REST + gRPC server (multiplexed on one port via h2c) |
| `bowrain-worker`        | `bowrain`      | Background worker for async push, asset processing, events |
| `apps/bowrain`          | `bowrain`      | Wails v3 desktop app                                     |
| `apps/web`              | `bowrain`      | SaaS web UI                                              |
| `apps/ctrl`             | `bowrain`      | Admin control panel                                      |
| `apps/pulse`            | `bowrain`      | Public real-time dashboard                               |

The server multiplexes gRPC and HTTP on the same port via h2c (cleartext
HTTP/2) protocol detection: requests with `Content-Type: application/grpc` are
routed to gRPC, all others to the HTTP router. See
[AD-009: Sync Protocol](009-sync-protocol.md) for the sync endpoints.

### Relationship to the Framework

Bowrain consumes framework interfaces and never forks them. The key framework
interfaces Bowrain uses:

| Framework Interface              | Bowrain Use                                                        |
| -------------------------------- | ------------------------------------------------------------------ |
| `format.DataFormatReader/Writer` | Connector implementations extract and re-emit Parts                |
| `tool.Tool`                      | Server-side flows run the same tools as the CLI                    |
| `flow.Executor`                  | The server executes flows via the same executor                    |
| `aiprovider.LLMProvider`         | AI translation, QA, and review tools use the same provider interface |
| `mtprovider.MTProvider`          | MT translation tools likewise                                      |
| `core/storage.BlobStore`         | Bowrain provides Azure Blob and local filesystem implementations. See [AD-007: Media and Blob Storage](007-media-and-blob-storage.md). |
| `sievepen.TM`                    | Bowrain provides SQLite + PostgreSQL implementations               |
| `termbase.TermBase`              | Bowrain provides SQLite + PostgreSQL implementations               |
| `plugin` registries              | Bowrain hosts format and tool plugins via gRPC + Java bridge       |

The framework never imports `bowrain/*`. Bowrain never forks framework types.
Shared types live in `bowrain/core` so that packages like `bowrain/auth` can
re-export them via Go type aliases for backward compatibility.

### Workspace Coordination

A top-level `go.work` at the repository root lists all seven Go modules
(framework, shared CLI, kapi, kapi-desktop, `bowrain/core`, `bowrain/cli`,
`bowrain`) so that a single `go build ./...` from the root resolves
cross-module imports without `replace` directives. Each child module still
declares `replace` directives in its own `go.mod` for CI builds with
`GOWORK=off` — these CI builds catch accidental cross-module imports before
they reach main.

Each module is tagged independently:

```
v0.16.0              → framework
bowrain/core/v0.1.0  → bowrain/core
bowrain/cli/v0.1.0   → bowrain/cli
bowrain/v0.16.0      → bowrain
```

### Frontend Workspace

A root `package.json` npm workspace coordinates every frontend package under
the monorepo. Apache-2.0 packages (`packages/ui`, `packages/flow-editor`,
`packages/storybook-config`) are shared by both kapi-desktop and bowrain apps.
AGPL-3.0 packages live under `bowrain/packages/ui` (`@neokapi/ui`) and
`bowrain/apps/*`. `vp install` at the repo root installs all workspace
members.

## Consequences

- Each binary carries only the dependencies it needs. `bowrain` CLI has no
  SQLite, Wails, Echo, or OIDC; `bowrain-server` has no Cobra; kapi CLI has
  no bowrain at all.
- The three-module split inside `bowrain/` lets `bowrain/core` evolve as a
  stable types contract while `bowrain/cli` and `bowrain` iterate on
  behavior.
- All AGPL-3.0 code is contained within the `bowrain/` subtree, making the
  license boundary visually obvious and verifiable in CI.
- The framework stays Apache-2.0 and fully reusable by any consumer; Bowrain
  is one of them.
- Content flows from connectors into the ContentStore, gets processed by
  framework tools, and flows back to its source system — the same streaming
  pipeline that powers the standalone CLI.
- Single-binary distribution: each server binary is a static Go binary with
  no JVM, no Node.js runtime, and no container required for basic usage.

## Related

- [AD-002: Authentication and Workspaces](002-authentication-and-workspaces.md)
- [AD-004: Content Store and Versioning](004-content-store.md)
- [AD-008: Connector System](008-connector-system.md)
- [AD-009: Sync Protocol](009-sync-protocol.md)
- [AD-framework-001: Vision and Module Architecture](https://neokapi.github.io/web/neokapi/docs/architecture/001-vision-and-modules)
