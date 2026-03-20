---
sidebar_position: 1
title: Overview
---

# Bowrain CLI

Bowrain CLI is the project companion CLI for the Bowrain localization platform. It manages `.bowrain/` projects in your repository and syncs with Bowrain Server for team collaboration.

## What is Bowrain CLI?

Bowrain CLI is to Bowrain Server as **git is to GitHub** — a local-first project management tool that:

- Initializes and manages `.bowrain/` projects in your repository
- Runs translation flows (AI, MT, TM, QA) on project files
- Syncs changes with Bowrain Server via push/pull
- Provides project status, diff, and configuration commands

## Key Features

### Project Model

`.bowrain/` directories (like `.git/`) contain:

- **config.yaml** — project settings, file mappings, locales
- **flows/** — custom YAML flow definitions
- **.sync-cache** — sync cache (gitignored, local only)

### Translation Flows

Composable pipelines that process files through tools:

```bash
# Run built-in AI translation flow
bowrain flow run ai-translate

# Create custom flows in .bowrain/flows/my-flow.yaml
# Run custom flow
bowrain flow run my-flow
```

Flows automatically process all files matching `.bowrain/config.yaml` mappings.

### Server Sync

Push/pull workflow similar to git:

```bash
bowrain status    # Show local changes
bowrain diff      # Compare local vs. server
bowrain pull      # Fetch from server
bowrain push -m "message"  # Upload to server
```

Only changed blocks transfer (content-addressed sync).

### Configuration

View or set project and global configuration values:

```bash
bowrain config project.name              # Print project name
bowrain config project.name "My App"     # Set project name
bowrain config --global server.url https://bowrain.example.com  # Set global server URL
```

## When to Use Bowrain CLI

Use Bowrain CLI when you:

- **Manage localization projects** with `.bowrain/` configuration
- **Sync with Bowrain Server** for team collaboration
- **Run project-based flows** defined in `.bowrain/flows/`
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
bowrain init --name "My App" --source en-US --targets fr-FR,de-DE

# Run translation flow
bowrain flow run ai-translate

# Check status
bowrain status
```

## Next Steps

- [Project Model](/bowrain/cli/project-model)
- [Commands Reference](/bowrain/cli/commands/init)
- [Flows](/bowrain/cli/flows/overview)
- [Use Cases](/bowrain/cli/use-cases/website-translation)
