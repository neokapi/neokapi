---
title: workspace
sidebar_position: 11
---

# kapi workspace

List the workspaces you can access on a Bowrain server, or create a new team
workspace. Requires authentication (run [`kapi auth login`](/cli/commands/auth)
first).

## Usage

```bash
kapi workspace list
kapi workspace create --name "<name>" [--slug <slug>]
```

The server is resolved from `--server`, then `BOWRAIN_SERVER_URL` /
`server.url` in `~/.config/bowrain/bowrain.yaml`, then your stored login.

## Subcommands

### `list`

Lists every workspace your account can access, marking your personal workspace.

```bash
kapi workspace list

# Example output:
# alice (Alice) [personal]
# acme (Acme Corp)
```

Add `--json` for machine-readable output:

```bash
kapi workspace list --json
```

### `create`

Creates a new team workspace. The slug is derived from `--name` when `--slug`
is omitted.

```bash
kapi workspace create --name "Acme Corp"
# Workspace created: acme-corp (Acme Corp)

kapi workspace create --name "Acme Corp" --slug acme
# Workspace created: acme (Acme Corp)
```

## Flags

| Flag       | Applies to | Description                                      |
| ---------- | ---------- | ------------------------------------------------ |
| `--server` | both       | Server URL (overrides config and stored login)   |
| `--name`   | `create`   | Workspace name (required)                        |
| `--slug`   | `create`   | URL-friendly slug (derived from `--name` if omitted) |

## Exit Codes

- `0` — Success
- `1` — Error (not authenticated, server unreachable, slug conflict, …)

## Related Commands

- [`kapi auth`](/cli/commands/auth) — Authenticate before listing or creating workspaces
- [`kapi init`](/cli/commands/init) — Scaffolds a project and can select or create a workspace interactively
