---
sidebar_position: 3
title: Quick Start
slug: /quickstart
---

# Quick Start

Get started with the Bowrain CLI in 5 minutes.

## Initialize a Project

Create a `.bowrain/` project directory (like `.git` for localization):

```bash
bowrain init
```

The interactive wizard guides you through setup:

1. **Sign in to Bowrain** — authenticate and create a server-connected project
2. **Email me a claim link** — create an anonymous project with email claim
3. **Continue without signing in** — create an anonymous project (prints claim URL)
4. **Local only** — no server connection, pure local project

Or skip the wizard with flags:

```bash
bowrain init --name "My Project" --source en-US --targets fr-FR,de-DE
```

This creates `.bowrain/config.yaml` with project settings and flow definitions.

## Translate Files

Run the built-in AI translation flow:

```bash
bowrain flow run ai-translate
```

Bowrain CLI automatically:
- Reads files matching your `.bowrain/config.yaml` mappings
- Translates from source to target locales
- Writes results back to local files

## Sync with Bowrain Server

Push translations to the server for team collaboration:

```bash
bowrain push -m "Translate UI strings"
```

Pull translations from teammates:

```bash
bowrain pull
```

Check sync status:

```bash
bowrain status
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
bowrain flow run my-flow
```

## Key Commands

| Command | Description |
|---------|-------------|
| `bowrain init` | Initialize a project |
| `bowrain status` | Show sync state |
| `bowrain flow run` | Execute a workflow |
| `bowrain flow list` | List available flows |
| `bowrain push` | Upload to server |
| `bowrain pull` | Fetch from server |
| `bowrain config` | View or set configuration |
| `bowrain serve` | Open local web editor |

## Next Steps

- **Full walkthrough**: See [Walkthrough](/bowrain/walkthrough)
- **Connect to server**: Use interactive `bowrain init` and choose "Sign in to Bowrain"
- **Explore flows**: `bowrain flow list`
- **CLI reference**: [Bowrain CLI](/bowrain/cli/commands/init)
