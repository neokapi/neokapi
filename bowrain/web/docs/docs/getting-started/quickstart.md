---
sidebar_position: 3
title: Quick Start
slug: /quickstart
---

# Quick Start

Get started with the Bowrain CLI in 5 minutes.

## Initialize a Project

Create a `.kapi` project — a `<dir-name>.kapi` recipe at the project root with a sibling `.kapi/` state directory (like `.git` for localization):

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

This writes `<dir-name>.kapi` (the recipe) and `.kapi/` (state, including `flows/`).

## Translate Files

Run the built-in AI translation tool:

```bash
bowrain ai-translate
```

Bowrain CLI automatically:

- Reads files matching your recipe's `content:` collections
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

Define a workflow in `.kapi/flows/my-flow.yaml`:

```yaml
name: my-flow
description: Translate with AI and run QA checks

steps:
  - tool: ai-translate
  - tool: qa-check
```

Run it:

```bash
bowrain run my-flow
```

## Key Commands

| Command                    | Description                       |
| -------------------------- | --------------------------------- |
| `bowrain init`             | Initialize a project              |
| `bowrain status`           | Show sync state                   |
| `bowrain ai-translate`     | Translate with AI                 |
| `bowrain pseudo-translate` | Generate pseudo-translations      |
| `bowrain qa-check`         | Run quality checks                |
| `bowrain run <flow>`       | Execute a composed or custom flow |
| `bowrain flows`            | List available flows              |
| `bowrain tools`            | List available tools              |
| `bowrain push`             | Upload to server                  |
| `bowrain pull`             | Fetch from server                 |
| `bowrain config`           | View or set configuration         |
| `bowrain serve`            | Open local web editor             |

## Next Steps

- **Full walkthrough**: See [Walkthrough](/walkthroughs/bowrain-getting-started)
- **Connect to server**: Use interactive `bowrain init` and choose "Sign in to Bowrain"
- **Explore flows and tools**: `bowrain flows` and `bowrain tools`
- **CLI reference**: [Bowrain CLI](/cli/commands/init)
