---
id: 018-four-module-architecture
sidebar_position: 18
title: "AD-018: Four-Module Monorepo Architecture"
---
# AD-018: Four-Module Monorepo Architecture

## Context

The original two-module structure (framework + bowrain) coupled the kapi CLI to the bowrain platform module. This meant:

- kapi CLI binary pulled in all bowrain dependencies (SQLite, Wails, Echo, OIDC, keyring), making it heavier than necessary
- kapi and bowrain couldn't be versioned independently
- Shared types (project model, auth, connectors) lived in bowrain, forcing kapi to import the entire platform
- Framework packages at the repository root created clutter alongside module directories

## Decision

Split the repository into four Go modules coordinated by a `go.work` workspace:

```
gokapi (framework)  <--  platform (shared)  <--  kapi (CLI)
                              ^
                              └───────────────  bowrain (server/desktop)

kapi ✗ bowrain  (no dependency in either direction)
```

### Module Responsibilities

| Module | Import Path | Role |
|--------|------------|------|
| Framework | `github.com/gokapi/gokapi` | Content model, formats, tools, pipeline, plugin system |
| Platform | `github.com/gokapi/gokapi/platform` | Shared types: project model, auth, connector interfaces, REST client, config |
| Kapi | `github.com/gokapi/gokapi/kapi` | CLI tool with Cobra commands, lightweight local dashboard |
| Bowrain | `github.com/gokapi/gokapi/bowrain` | REST server, desktop app, SQLite storage, OIDC, connectors |

### Directory Layout

Framework packages live under `core/` to reduce root-level clutter. Each module has its own `go.mod` with `replace` directives for local development:

```
gokapi/
├── go.work          # use . ./platform ./kapi ./bowrain
├── go.mod           # github.com/gokapi/gokapi
├── core/            # All framework Go packages
├── platform/        # Shared platform types
│   └── go.mod       # github.com/gokapi/gokapi/platform
├── kapi/            # CLI tool
│   └── go.mod       # github.com/gokapi/gokapi/kapi
├── bowrain/          # Server + desktop
│   └── go.mod       # github.com/gokapi/gokapi/bowrain
└── packages/ui/     # Shared React component library
```

### Dependency Constraints

- **Platform** depends only on framework. No SQLite, Wails, Echo, OIDC, keyring, Cobra.
- **Kapi** depends on framework + platform. No SQLite, Wails, Echo, OIDC, keyring.
- **Bowrain** depends on framework + platform. Has all heavy dependencies.
- **Kapi and bowrain have no dependency on each other.**

These constraints are verified in CI with `GOWORK=off` builds and `go list -m all` checks.

### Type Sharing via Aliases

To minimize the blast radius of the split, bowrain packages that previously defined shared types (auth, connector, store, event) now re-export platform types via Go type aliases:

```go
// bowrain/auth/types.go
package auth
import platauth "github.com/gokapi/gokapi/platform/auth"
type User = platauth.User
type Workspace = platauth.Workspace
// ...
```

This allows existing bowrain code to continue importing `bowrain/auth` while the canonical definitions live in `platform/auth`.

### Tagging & Versioning

Each module is tagged independently using Go module version conventions:

```
v0.16.0           → framework
platform/v0.1.0   → platform
kapi/v0.1.0       → kapi
bowrain/v0.16.0   → bowrain
```

### Build and Tooling

All four modules target Go 1.24+. The Makefile provides per-module build, test, vet, and lint targets. CI verifies module isolation with `GOWORK=off` builds to catch accidental cross-module imports.

`go mod tidy` does not respect `go.work` — each child module requires explicit `replace` directives in its `go.mod` for local development (e.g., `replace github.com/gokapi/gokapi => ../`).

## Consequences

- kapi CLI binary is significantly smaller and faster to build (no SQLite, Wails, etc.)
- Shared types in platform can evolve independently of both kapi and bowrain
- Framework packages under `core/` give a cleaner repository root
- Four `go.mod` files to maintain, but `go.work` makes daily development seamless
- GoReleaser, CI, Dockerfile, and Makefile all updated for four-module builds
