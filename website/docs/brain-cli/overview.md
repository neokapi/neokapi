---
sidebar_position: 1
title: Overview
---

# Brain CLI

Brain is the project companion CLI for Bowrain — the localization platform. It manages `.brain/` projects in your repository and syncs with Bowrain Server for team collaboration.

## What is Brain?

Brain is to Bowrain Server as **git is to GitHub** — a local-first project management tool that:

- Initializes and manages `.brain/` projects in your repository
- Runs translation flows (AI, MT, TM, QA) on project files
- Syncs changes with Bowrain Server via push/pull
- Provides project status, diff, and configuration commands

## Key Features

### Project Model

`.brain/` directories (like `.git/`) contain:
- **config.yaml** — project settings, file mappings, locales
- **flows/** — custom YAML flow definitions
- **.sync-cache** — sync cache (gitignored, local only)

### Translation Flows

Composable pipelines that process files through tools:

```bash
# Run built-in AI translation flow
brain flow run ai-translate

# Create custom flows in .brain/flows/my-flow.yaml
# Run custom flow
brain flow run my-flow
```

Flows automatically process all files matching `.brain/config.yaml` mappings.

### Server Sync

Push/pull workflow similar to git:

```bash
brain status    # Show local changes
brain diff      # Compare local vs. server
brain pull      # Fetch from server
brain push -m "message"  # Upload to server
```

Only changed blocks transfer (content-addressed sync).

### Configuration

View or set project and global configuration values:

```bash
brain config project.name              # Print project name
brain config project.name "My App"     # Set project name
brain config --global server.url https://bowrain.example.com  # Set global server URL
```

## When to Use Brain

Use Brain CLI when you:

- **Manage localization projects** with `.brain/` configuration
- **Sync with Bowrain Server** for team collaboration
- **Run project-based flows** defined in `.brain/flows/`
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
brew install gokapi/tap/brain

# Download binary
# Visit https://github.com/gokapi/gokapi/releases
```

## Quick Start

```bash
# Initialize project
cd my-app/
brain init --name "My App" --source en-US --targets fr-FR,de-DE

# Run translation flow
brain flow run ai-translate

# Check status
brain status
```

## Next Steps

- [Project Model](/docs/brain-cli/project-model)
- [Commands Reference](/docs/brain-cli/commands/init)
- [Flows](/docs/brain-cli/flows/overview)
- [Use Cases](/docs/brain-cli/use-cases/website-translation)
