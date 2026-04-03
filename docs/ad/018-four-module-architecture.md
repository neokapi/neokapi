---
id: 018-multi-module-architecture
sidebar_position: 18
title: "AD-018: Multi-Module Monorepo Architecture"
---
# AD-018: Multi-Module Monorepo Architecture

## Context

A localization platform has distinct deployment targets — a standalone CLI tool, a project sync companion CLI, a REST server, and a desktop app — each with different dependency profiles. Coupling them in a single module forces every binary to pull in all dependencies (SQLite, Wails, Echo, OIDC, keyring), making builds slow and binaries large.

At the same time, both CLIs share a common command structure (formats, plugins, tools, flows, presets, termbase, version) that should be maintained in one place rather than duplicated.

## Decision

Split the repository into seven Go modules coordinated by a `go.work` workspace:

```
framework (root)
    ↑
    ├── cli (framework only)
    │    ↑
    │    ├── kapi (framework + cli)
    │    ├── kapi-desktop (framework + cli)
    │    └── bowrain/cli (framework + cli + bowrain/core)
    │
    └── bowrain/core (framework only — interfaces)
         ↑
         ├── bowrain/cli (framework + cli + bowrain/core)
         └── bowrain (framework + bowrain/core)
```

### Module Responsibilities

| Module | Import Path | Directory | Role |
|--------|------------|-----------|------|
| Framework | `github.com/neokapi/neokapi` | `.` (repo root) | Content model, formats, tools, pipeline, plugin system, TM, termbase, AI/MT providers |
| CLI | `github.com/neokapi/neokapi/cli` | `cli/` | Shared CLI base: App struct, command factories, output formatting, app config |
| Bowrain Core | `github.com/neokapi/neokapi/bowrain/core` | `bowrain/core/` | Shared platform types: project model, auth, connector interfaces, REST client, store |
| Kapi | `github.com/neokapi/neokapi/kapi` | `kapi/` | Standalone CLI tool for local file processing |
| Kapi Desktop | `github.com/neokapi/neokapi/kapi-desktop` | `apps/kapi-desktop/` | Wails v3 desktop app for visual localization workflows |
| Bowrain CLI | `github.com/neokapi/neokapi/bowrain/cli` | `bowrain/cli/` | Bowrain CLI — project sync companion (init, push, pull, auth, status) |
| Bowrain | `github.com/neokapi/neokapi/bowrain` | `bowrain/` | REST server, desktop app, PostgreSQL storage, OIDC, connectors |

### Directory Layout

Framework packages live at the repo root under `core/`, `sievepen/`, `termbase/`, and `providers/`. The entire `bowrain/` subtree contains all AGPL-3.0 code, with `bowrain/core/` and `bowrain/cli/` as nested modules. Each module has its own `go.mod` with `replace` directives for local development:

```
neokapi/
├── go.work          # use . ./cli ./kapi ./apps/kapi-desktop ./bowrain/core ./bowrain/cli ./bowrain
├── go.mod           # github.com/neokapi/neokapi
├── core/            # Framework Go packages (model, format, flow, tool, etc.)
├── sievepen/        # Translation memory
├── termbase/        # Terminology
├── providers/
│   ├── ai/          # package aiprovider — LLM providers
│   └── mt/          # package mtprovider — MT providers
├── bench/           # Benchmarks
├── examples/        # Plugin examples
├── cli/             # Shared CLI base + app config
│   └── go.mod       # github.com/neokapi/neokapi/cli
├── kapi/            # Standalone CLI tool
│   └── go.mod       # github.com/neokapi/neokapi/kapi
├── apps/
│   └── kapi-desktop/ # Wails v3 desktop app
│       └── go.mod   # github.com/neokapi/neokapi/kapi-desktop
├── bowrain/         # ALL AGPL-3.0 CODE
│   ├── go.mod       # github.com/neokapi/neokapi/bowrain
│   ├── Makefile     # Bowrain-specific build targets
│   ├── core/        # Shared platform interfaces
│   │   └── go.mod   # github.com/neokapi/neokapi/bowrain/core
│   ├── cli/         # Project sync CLI
│   │   └── go.mod   # github.com/neokapi/neokapi/bowrain/cli
│   ├── cmd/bowrain-server/  # REST API server
│   ├── cmd/bowrain-worker/  # Background worker
│   ├── server/ store/ auth/ connector/ event/ service/
│   ├── billing/ jobs/ brand/ graph/ analytics/
│   ├── apps/        # bowrain/ web/ ctrl/ pulse/ mobile/ keycloak-theme/
│   ├── packages/ui/ # @neokapi/ui (AGPL)
│   ├── docker/ deploy/ e2e/ emails/ proto/
│   ├── compose.yaml
│   └── compose.override.yaml
├── package.json     # Root npm workspace coordinating all frontend packages
├── .npmrc           # install-strategy=hoisted (npm 11)
└── packages/
    ├── ui/          # @neokapi/ui-primitives — shadcn/ui primitives (Apache license)
    └── flow-editor/ # @neokapi/flow-editor — shared flow editor (Apache license)
```

### Frontend Workspace

A root `package.json` npm workspace coordinates all frontend packages. `vp install` at the repo root installs all workspace members — no per-directory installs are needed. `.npmrc` sets `install-strategy=hoisted` for npm 11 compatibility.

**License boundary:** packages under `packages/*` are Apache-2.0 licensed (shared by both the open-source kapi and the commercial bowrain platform). Everything under `bowrain/` is AGPL-3.0.

### Dependency Constraints

- **CLI** depends only on framework. No bowrain, no SQLite, Wails, Echo, OIDC, keyring.
- **Bowrain Core** depends only on framework. No CLI, no Cobra, Viper, SQLite, Wails, Echo, OIDC, keyring.
- **CLI and bowrain/core have zero cross-dependency.**
- **Kapi** depends on framework + CLI. No bowrain, no heavy dependencies.
- **Kapi Desktop** depends on framework + CLI. No bowrain. Separate module due to Wails/keyring dependencies.
- **Bowrain CLI** depends on framework + CLI + bowrain/core.
- **Bowrain** depends on framework + bowrain/core. No CLI dependency.
- **Kapi and bowrain have no dependency on each other.**

These constraints are verified in CI with `GOWORK=off` builds and `go list -m all` checks.

### Type Sharing via Aliases

To minimize the blast radius of the split, bowrain packages that previously defined shared types (auth, connector, store, event) re-export bowrain/core types via Go type aliases:

```go
// bowrain/auth/types.go
package auth
import coreauth "github.com/neokapi/neokapi/bowrain/core/auth"
type User = coreauth.User
type Workspace = coreauth.Workspace
```

This allows existing bowrain code to continue importing `bowrain/auth` while the canonical definitions live in `bowrain/core/auth`.

### RegistryResolver Hook

The CLI module has no bowrain dependency, but Bowrain CLI needs to resolve plugin registries from `.bowrain/` project config. This is handled via a `RegistryResolver` hook on the CLI `App` struct:

```go
// cli/app.go
type App struct {
    RegistryResolver func() []config.RegistryEntry
    // ...
}
```

Bowrain CLI sets this hook during initialization to resolve registries from the project config. The CLI module never imports bowrain/core/project.

### Tagging & Versioning

Each module is tagged independently using Go module version conventions:

```
v0.16.0              → framework
cli/v0.1.0           → cli
kapi/v0.1.0          → kapi
kapi-desktop/v0.1.0  → kapi-desktop
bowrain/core/v0.1.0  → bowrain/core
bowrain/cli/v0.1.0   → bowrain/cli
bowrain/v0.16.0      → bowrain
```

### Build and Tooling

All modules target Go 1.26+. The Makefile provides per-module build, test, vet, and lint targets. CI verifies module isolation with `GOWORK=off` builds to catch accidental cross-module imports.

`go mod tidy` does not respect `go.work` — each child module requires explicit `replace` directives in its `go.mod` for local development (e.g., `replace github.com/neokapi/neokapi => ../`).

## Consequences

- kapi CLI binary has no bowrain, SQLite, Wails, or OIDC dependencies
- CLI and bowrain/core evolve independently — CLI changes don't affect bowrain, bowrain/core changes don't affect kapi
- Bowrain CLI (`bowrain` binary) is decoupled from bowrain's heavy dependencies (SQLite, Wails, Echo, keyring)
- Framework packages at the repo root (`core/`, `sievepen/`, `termbase/`, `providers/`) give clean organization
- All AGPL-3.0 code is contained within the `bowrain/` subtree, with a clear license boundary
- Seven `go.mod` files to maintain, but `go.work` makes daily development seamless
- GoReleaser, CI, Dockerfile, and Makefile all handle multi-module builds
