---
sidebar_position: 1
title: Overview
---

# Project sync

The bowrain plugin adds project sync to [kapi](https://neokapi.github.io/web/neokapi/): it connects a `.kapi` project in your repository to a Bowrain server, so a team shares one source of brand voice, terminology, memory, and translations.

kapi is the **local-files / git connector** — one of [several ways content reaches Bowrain](/server/connectors), and the path for a developer working from a codebase. Content can just as well arrive server-side from a CMS, a design tool, or a git host with no local checkout.

:::note
Project sync ships as the **`kapi-bowrain` plugin** for the `kapi` CLI — there is no separate `bowrain` binary. Every command below is invoked as `kapi <command>` (e.g. `kapi init`, `kapi push`). See [Installation](/installation) to set it up.
:::

## How it works

With the plugin installed, kapi is the local end of a **git-to-GitHub** relationship with the server — you work locally and push/pull to share. It:

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

## When to use it

Use kapi with the bowrain plugin when you:

- **Manage localization projects** with a `.kapi` recipe
- **Sync with a Bowrain server** for team collaboration
- **Run project-based flows** defined in `.kapi/flows/` or inline on the recipe
- **Want automation** via CI/CD pipelines

Use kapi on its own (no plugin) when you:

- **Process standalone files** without a project
- **Need quick one-off operations** (word count, pseudo-translate)

Use the desktop or web editor when you:

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
