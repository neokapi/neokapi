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

Split the repository into six Go modules coordinated by a `go.work` workspace:

```
framework (core/)
    ↑
    ├── cli (framework only — shared CLI base + app config)
    │    ↑
    │    ├── kapi (framework + cli)
    │    └── brain (framework + cli + platform)
    │
    └── platform (framework only — project model, auth, connectors, client, store)
         ↑
         ├── brain (framework + cli + platform)
         └── bowrain (framework + platform)
```

### Module Responsibilities

| Module | Import Path | Role |
|--------|------------|------|
| Framework | `github.com/gokapi/gokapi` | Content model, formats, tools, pipeline, plugin system |
| CLI | `github.com/gokapi/gokapi/cli` | Shared CLI base: App struct, command factories, output formatting, app config |
| Platform | `github.com/gokapi/gokapi/platform` | Shared platform types: project model, auth, connector interfaces, REST client |
| Kapi | `github.com/gokapi/gokapi/kapi` | Standalone CLI tool for local file processing |
| Brain | `github.com/gokapi/gokapi/brain` | Project sync companion CLI (init, push, pull, auth, status) |
| Bowrain | `github.com/gokapi/gokapi/bowrain` | REST server, desktop app, SQLite storage, OIDC, connectors |

### Directory Layout

Framework packages live under `core/` to reduce root-level clutter. Each module has its own `go.mod` with `replace` directives for local development:

```
gokapi/
├── go.work          # use . ./cli ./platform ./kapi ./brain ./bowrain
├── go.mod           # github.com/gokapi/gokapi
├── core/            # All framework Go packages
├── cli/             # Shared CLI base + app config
│   ├── go.mod       # github.com/gokapi/gokapi/cli
│   ├── config/      # Viper-based app configuration
│   └── output/      # Shared output formatting
├── platform/        # Shared platform types
│   └── go.mod       # github.com/gokapi/gokapi/platform
├── kapi/            # Standalone CLI tool
│   └── go.mod       # github.com/gokapi/gokapi/kapi
├── brain/           # Project sync CLI
│   └── go.mod       # github.com/gokapi/gokapi/brain
├── bowrain/         # Server + desktop
│   └── go.mod       # github.com/gokapi/gokapi/bowrain
└── packages/ui/     # Shared React component library
```

### Dependency Constraints

- **CLI** depends only on framework. No platform, no SQLite, Wails, Echo, OIDC, keyring.
- **Platform** depends only on framework. No CLI, no Cobra, Viper, SQLite, Wails, Echo, OIDC, keyring.
- **CLI and platform have zero cross-dependency.**
- **Kapi** depends on framework + CLI. No platform, no heavy dependencies.
- **Brain** depends on framework + CLI + platform.
- **Bowrain** depends on framework + platform. No CLI dependency.
- **Kapi and bowrain have no dependency on each other.**

These constraints are verified in CI with `GOWORK=off` builds and `go list -m all` checks.

### Type Sharing via Aliases

To minimize the blast radius of the split, bowrain packages that previously defined shared types (auth, connector, store, event) re-export platform types via Go type aliases:

```go
// bowrain/auth/types.go
package auth
import platauth "github.com/gokapi/gokapi/platform/auth"
type User = platauth.User
type Workspace = platauth.Workspace
```

This allows existing bowrain code to continue importing `bowrain/auth` while the canonical definitions live in `platform/auth`.

### RegistryResolver Hook

The CLI module has no platform dependency, but brain needs to resolve plugin registries from `.brain/` project config. This is handled via a `RegistryResolver` hook on the CLI `App` struct:

```go
// cli/app.go
type App struct {
    RegistryResolver func() []config.RegistryEntry
    // ...
}
```

Brain sets this hook during initialization to resolve registries from the project config. The CLI module never imports platform/project.

### Tagging & Versioning

Each module is tagged independently using Go module version conventions:

```
v0.16.0           → framework
cli/v0.1.0        → cli
platform/v0.1.0   → platform
kapi/v0.1.0       → kapi
brain/v0.1.0      → brain
bowrain/v0.16.0   → bowrain
```

### Build and Tooling

All modules target Go 1.26+. The Makefile provides per-module build, test, vet, and lint targets. CI verifies module isolation with `GOWORK=off` builds to catch accidental cross-module imports.

`go mod tidy` does not respect `go.work` — each child module requires explicit `replace` directives in its `go.mod` for local development (e.g., `replace github.com/gokapi/gokapi => ../`).

## Consequences

- kapi CLI binary has no platform, SQLite, Wails, or OIDC dependencies
- CLI and platform evolve independently — CLI changes don't affect bowrain, platform changes don't affect kapi
- Brain CLI is decoupled from bowrain's heavy dependencies (SQLite, Wails, Echo, keyring)
- Framework packages under `core/` give a cleaner repository root
- Six `go.mod` files to maintain, but `go.work` makes daily development seamless
- GoReleaser, CI, Dockerfile, and Makefile all handle multi-module builds
