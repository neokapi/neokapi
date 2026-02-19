---
sidebar_position: 3
title: Quick Start
---

# Quick Start

Get started with Kapi in 5 minutes.

## Initialize a Project

Create a `.kapi/` project directory (like `.git` for localization):

```bash
kapi init
```

The interactive wizard guides you through setup. If you're already signed in,
it goes straight to workspace and project configuration. Otherwise, it offers:

1. **Sign in to Bowrain** — authenticate and create a server-connected project
2. **Email me a claim link** — create an anonymous project with email claim
3. **Continue without signing in** — create an anonymous project (prints claim URL)
4. **Local only** — no server connection, pure local project

You can also skip the wizard with flags:

```bash
kapi init --name "My Project" --source en-US --targets fr-FR,de-DE
```

This creates `.kapi/config.yaml` with project settings and flow definitions.

## Translate Files

Run the built-in AI translation flow:

```bash
kapi flow run ai-translate
```

Kapi automatically:
- Reads files matching your `.kapi/config.yaml` mappings
- Translates from source to target locales
- Writes results back to local files

## Check What Changed

View modified files:

```bash
kapi status
```

**Output:**

```
Project: My Project
Modified local files:
  M src/locales/fr/messages.json
  M src/locales/de/messages.json
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
kapi flow run my-flow
```

## List Available Commands

```bash
kapi --help
```

**Key commands:**

| Command | Description |
|---------|-------------|
| `kapi init` | Initialize a project |
| `kapi status` | Show sync state |
| `kapi flow run` | Execute a workflow |
| `kapi flow list` | List available flows |
| `kapi config` | View or set configuration values |
| `kapi serve` | Open local web editor |
| `kapi formats` | List supported formats |
| `kapi tools` | List available tools |

## Next Steps

- **Full walkthrough**: See [Project Walkthrough](/docs/getting-started/project-walkthrough)
- **Connect to server**: Use interactive `kapi init` and choose "Sign in to Bowrain"
- **Explore flows**: `kapi flow list`
- **CLI reference**: [User Guide](/docs/kapi-cli/commands/init)
