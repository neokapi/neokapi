---
id: 013-cli-and-server
sidebar_position: 13
title: "AD-013: Bowrain CLI and Bowrain Server"
---
# AD-013: Bowrain CLI and Bowrain Server

## Context

The Bowrain platform exposes its functionality through two application entry points:
- **Bowrain CLI** (`bowrain` binary) — Project-centric command-line tool for syncing files with Bowrain Server
- **Bowrain Server** (`bowrain-server` binary) — Multi-user platform for team collaboration and integrations

The CLI reflects the **project-based architecture** ([AD-016](./016-kapi-project-model.md)). All commands operate within a `.bowrain/` project directory. The server provides both REST (for external integrations and webhooks) and gRPC (for Bowrain desktop app streaming) APIs.

**This AD establishes the role separation:**
- **Bowrain CLI** = project-centric sync companion, git-like workflow, syncs with Bowrain Server
- **Bowrain Server** = multi-user platform, integration connectors, automation, ContentStore
- **Kapi** (separate CLI, see below) = standalone file processing tool demonstrating the neokapi framework, no project directory, no server sync

## Decision

### Bowrain CLI: Project-Based Commands

The CLI uses [Cobra](https://github.com/spf13/cobra) for hierarchical subcommands. **All commands require a `.bowrain/` project directory** (discovered by searching upward from the current directory, like git). Commands include `init`, `config`, `ls`, `add`, `rm`, `status`, `diff`, `pull`, `push`, `flow`, `serve`, `auth`, `termbase`, `formats`, `tools`, `plugins`, and `registry`.

See [CLI Commands Reference](/docs/notes/cli-commands-reference) for the full command tree, `bowrain init` workflows, `bowrain pull/push` algorithms, and all command details.

### Authentication

The `bowrain auth` command group enables CLI users to authenticate against a Bowrain Server instance using OAuth 2.0 Device Authorization Grant (RFC 8628) via OIDC ([AD-015](./015-auth-and-workspaces.md)). The token is stored at `~/.config/bowrain/auth.json` and automatically attached to API requests.

### Bowrain Server

The server binary (`bowrain/cmd/bowrain-server/`) provides remote access via two protocols. Server logic lives in `bowrain/server/` and supports two deployment modes:

**Deployment modes:**
1. **Server mode** (bowrain-server with `JWTSecret` set) — Full OIDC authentication, workspaces, binds to `0.0.0.0`
2. **Standalone mode** (bowrain-server with empty `JWTSecret`, or `bowrain serve`) — No authentication, binds to `localhost`

Mode is determined by the `ServerConfig.JWTSecret` field: when set, the server enables authentication, OIDC login, and workspace management; when empty, routes are registered without auth middleware. The server reports its mode via `GET /api/v1/config` so the web UI can adapt.

The **REST API** (Echo v4) covers public health/config routes, auth routes (device flow, OIDC, desktop PKCE, refresh), workspace CRUD with members, project CRUD scoped to workspaces, content/block routes, sync routes for Bowrain CLI pull/push, KAZ export/import, connector management, and flow execution.

**gRPC** serves the Bowrain desktop app via a dedicated `EditorService` with 24 RPCs covering auth, projects, blocks, TM, terminology, and real-time collaboration. See [AD-020](./020-collaborative-editor.md) for the full EditorService specification. gRPC and HTTP are multiplexed on the same port via h2c (cleartext HTTP/2) protocol detection — requests with `Content-Type: application/grpc` are routed to the gRPC server, all others to the HTTP router.

Both unary and streaming RPCs use JWT authentication via `authorization: Bearer <token>` in gRPC metadata, validated by server interceptors.

See [CLI Commands Reference](/docs/notes/cli-commands-reference) for the full REST API route listing, gRPC service definition, and CI/CD example.

### Kapi: Standalone File Processing

Kapi (`kapi` binary) is a separate CLI that demonstrates the neokapi open-source framework. It operates on files directly without requiring a project directory or server connection:

```bash
kapi formats list                                              # List available formats
kapi flow run pseudo-translate -i file.json --target-lang qps  # Process files directly
kapi plugins list                                              # List installed plugins
```

Kapi and Bowrain CLI share a common command base (`cli/` module) for format, plugin, tool, flow, preset, termbase, and version commands. Each CLI selects which commands to register and extends them with CLI-specific behavior.

## Alternatives Considered

- **REST only**: Simpler but no streaming for real-time progress and events. Bowrain would need polling, degrading the editing experience.

- **gRPC only**: Efficient and type-safe but harder for simple integrations, webhooks, and `curl`-based debugging.

- **GraphQL**: Flexible queries but over-engineered for this use case where the API surface is well-defined and resource-oriented.

- **Store-based CLI** (`bowrain store`): Mixes server concerns into the CLI. The ContentStore is a server-side persistence layer; the CLI should only interact via API.

- **Global config** (no project directories): Makes collaboration harder. The `.bowrain/` directory model enables team workflows (check config into git, .sync-cache gitignored) and git-like mental model.

- **Single CLI for both roles**: Combining standalone file processing and project sync in one binary blurs boundaries. Separate binaries (kapi for the framework, bowrain for the platform) keep each focused.

## Consequences

- **Bowrain CLI is project-based** — all commands require a `.bowrain/` directory, enforcing clean project structure and enabling stateful operations.

- **`bowrain init`** is the entry point, analogous to `git init`.

- **`bowrain pull/push`** mirror git's fetch/push mental model, making localization workflows familiar to developers.

- **`bowrain status/diff`** show sync state without modifying files, enabling safe inspection before pull/push.

- **Flow system** in Bowrain CLI — flows run on local files, defined in `.bowrain/flows/*.yaml`, with optional server interaction.

- **`bowrain serve`** becomes a project dashboard showing local + remote state.

- **REST + gRPC dual protocol** serves both external integrations (REST) and Bowrain desktop app real-time requirements (gRPC).

- **OAuth device flow authentication** (`bowrain auth`) makes CI/CD integration seamless with stored tokens.

- **Workspace-scoped API routes** enforce multi-tenancy at the HTTP layer, complementing the domain-level workspace model ([AD-015](./015-auth-and-workspaces.md)).

- **Clear role separation**: Bowrain CLI = project-centric sync companion, Bowrain Server = platform for integrations and collaboration, Kapi = standalone framework demonstration.

- **`bowrain ls/add/rm`** manage the project's file tracking without server interaction — they operate purely on `.bowrain/config.yaml` and local files.

- Bowrain CLI positions itself as **the file connector** for Bowrain Server — it handles files, the server handles integrations ([AD-005](./005-connector-system.md)).
