---
sidebar_position: 1
title: Overview
---

# Bowrain CLI

Bowrain CLI is the project companion CLI for the Bowrain localization platform. It manages `.kapi` projects in your repository and syncs with Bowrain Server for team collaboration.

:::note
Bowrain's CLI ships as the **`kapi-bowrain` plugin** for the `kapi` CLI — there is no standalone `bowrain` binary. Every command below is invoked as `kapi <command>` (e.g. `kapi init`, `kapi push`). See [Installation](/installation) to set it up.
:::

## What is Bowrain CLI?

Bowrain CLI is to Bowrain Server as **git is to GitHub** — a local-first project management tool that:

- Initializes and manages `.kapi` projects in your repository
- Runs translation flows (AI, MT, TM, QA) on project files
- Syncs changes with Bowrain Server via push/pull
- Provides project status, diff, and configuration commands

## Key Features

### Project Model

A bowrain project is a kapi project with a `server:` block on its recipe:

- **`<dir-name>.kapi`** — the recipe (committed) — project settings, content collections, flows, server connection
- **`.kapi/flows/`** — optional file-per-flow definitions (committed)
- **`.kapi/cache/sync-cache.json`** — sync cache (gitignored, local only)
- **`.kapi/cache/blocks.db`** — block store (gitignored, regenerable)

### Translation Tools and Flows

Built-in tools run as top-level commands, and composed flows run via `kapi run`:

```bash
# Run built-in AI translation tool
kapi ai-translate

# Run a composed multi-tool flow
kapi run ai-translate-qa

# Create custom flows in .kapi/flows/my-flow.yaml
# Run custom flow
kapi run my-flow
```

Tools and flows automatically process all files matching the recipe's `content:` collections.

### Server Sync

Push/pull workflow similar to git:

```bash
kapi status    # Show local changes
kapi diff      # Compare local vs. server
kapi pull      # Fetch from server
kapi push -m "message"  # Upload to server
```

Only changed blocks transfer (content-addressed sync).

### Configuration

View or set project and global configuration values:

```bash
kapi config name              # Print project name
kapi config name "My App"     # Set project name
kapi config --global server.url https://bowrain.example.com  # Set global server URL
```

## When to Use Bowrain CLI

Use Bowrain CLI when you:

- **Manage localization projects** with a `.kapi` recipe
- **Sync with Bowrain Server** for team collaboration
- **Run project-based flows** defined in `.kapi/flows/` or inline on the recipe
- **Want automation** via CI/CD pipelines

Use kapi CLI when you:

- **Process standalone files** without a project
- **Need quick one-off operations** (word count, pseudo-translate)

Use Bowrain Desktop/Web when you:

- **Need visual editing** with split preview, context panel
- **Collaborate** with team members in real-time
- **Manage workspaces** and projects in a browser

## Installation

```bash
# macOS
brew install neokapi/tap/bowrain-cli

# Download binary
# Visit https://github.com/neokapi/neokapi/releases
```

## Quick Start

```bash
# Initialize project
cd my-app/
kapi init --name "My App" --source en-US --targets fr-FR,de-DE

# Run AI translation
kapi ai-translate

# Check status
kapi status
```

## Next Steps

- [Project Model](/cli/project-model)
- [Commands Reference](/cli/commands/init)
- [Flows](/cli/flows/overview)
- [Use Cases](/cli/use-cases/website-translation)
