---
sidebar_position: 3
title: Quick Start
slug: /quickstart
---

# Quick Start

Connect a kapi project to Bowrain in 5 minutes. Commands run through kapi with the bowrain plugin installed.

## Initialize a Project

Create a `.kapi` project — a `<dir-name>.kapi` recipe at the project root with a sibling `.kapi/` state directory (like `.git` for localization):

```bash
kapi init
```

The interactive wizard guides you through setup:

1. **Sign in to Bowrain** — authenticate and create a server-connected project
2. **Email me a claim link** — create an anonymous project with email claim
3. **Continue without signing in** — create an anonymous project (prints claim URL)
4. **Local only** — no server connection, pure local project

Or skip the wizard with flags:

```bash
kapi init --name "My Project" --source en-US --targets fr-FR,de-DE
```

This writes `<dir-name>.kapi` (the recipe) and `.kapi/` (state, including `flows/`).

## Translate Files

Run the built-in AI translation tool:

```bash
kapi ai-translate
```

kapi automatically:

- Reads files matching your recipe's `content:` collections
- Translates from source to target locales
- Writes results back to local files

## Sync with Bowrain Server

Push translations to the server for team collaboration:

```bash
kapi push -m "Translate UI strings"
```

Pull translations from teammates:

```bash
kapi pull
```

Check sync status:

```bash
kapi status
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
kapi run my-flow
```

## Key Commands

| Command                    | Description                       |
| -------------------------- | --------------------------------- |
| `kapi init`             | Initialize a project              |
| `kapi status`           | Show sync state                   |
| `kapi ai-translate`     | Translate with AI                 |
| `kapi pseudo-translate` | Generate pseudo-translations      |
| `kapi qa-check`         | Run quality checks                |
| `kapi run <flow>`       | Execute a composed or custom flow |
| `kapi flows`            | List available flows              |
| `kapi tools`            | List available tools              |
| `kapi push`             | Upload to server                  |
| `kapi pull`             | Fetch from server                 |
| `kapi config`           | View or set configuration         |

## Next Steps

- **Full walkthrough**: See [Walkthrough](/walkthroughs/bowrain-getting-started)
- **Connect to server**: Use interactive `kapi init` and choose "Sign in to Bowrain"
- **Explore flows and tools**: `kapi flows` and `kapi tools`
- **CLI reference**: [Bowrain CLI](/cli/commands/init)
