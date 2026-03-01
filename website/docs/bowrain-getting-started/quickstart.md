---
sidebar_position: 3
title: Quick Start
slug: /bowrain/quickstart
---

# Quick Start

Get started with the Bowrain CLI in 5 minutes.

## Initialize a Project

Create a `.bowrain/` project directory (like `.git` for localization):

```bash
brain init
```

The interactive wizard guides you through setup:

1. **Sign in to Bowrain** — authenticate and create a server-connected project
2. **Email me a claim link** — create an anonymous project with email claim
3. **Continue without signing in** — create an anonymous project (prints claim URL)
4. **Local only** — no server connection, pure local project

Or skip the wizard with flags:

```bash
brain init --name "My Project" --source en-US --targets fr-FR,de-DE
```

This creates `.bowrain/config.yaml` with project settings and flow definitions.

## Translate Files

Run the built-in AI translation flow:

```bash
brain flow run ai-translate
```

Bowrain CLI automatically:
- Reads files matching your `.bowrain/config.yaml` mappings
- Translates from source to target locales
- Writes results back to local files

## Sync with Bowrain Server

Push translations to the server for team collaboration:

```bash
brain push -m "Translate UI strings"
```

Pull translations from teammates:

```bash
brain pull
```

Check sync status:

```bash
brain status
```

## Create a Custom Flow

Define a workflow in `.bowrain/flows/my-flow.yaml`:

```yaml
name: my-flow
description: Translate with AI and run QA checks

steps:
  - tool: ai-translate
  - tool: qa-check
```

Run it:

```bash
brain flow run my-flow
```

## Key Commands

| Command | Description |
|---------|-------------|
| `brain init` | Initialize a project |
| `brain status` | Show sync state |
| `brain flow run` | Execute a workflow |
| `brain flow list` | List available flows |
| `brain push` | Upload to server |
| `brain pull` | Fetch from server |
| `brain config` | View or set configuration |
| `brain serve` | Open local web editor |

## Next Steps

- **Full walkthrough**: See [Project Walkthrough](/docs/bowrain/project-walkthrough)
- **Connect to server**: Use interactive `brain init` and choose "Sign in to Bowrain"
- **Explore flows**: `brain flow list`
- **CLI reference**: [Bowrain CLI](/docs/bowrain-cli/commands/init)
