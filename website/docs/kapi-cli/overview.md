---
sidebar_position: 1
title: Overview
---

# Kapi CLI

Kapi is the command-line interface for file-based localization workflows. It manages `.kapi/` projects, executes translation flows, and syncs with Bowrain Server for team collaboration.

## What is Kapi?

Kapi is to Bowrain Server as **git is to GitHub** — a local-first tool that:

- Initializes and manages `.kapi/` projects in your repository
- Runs translation flows (AI, MT, TM, QA) on local files
- Syncs changes with Bowrain Server via push/pull (optional)
- Operates on files without requiring a server

## Key Features

### Project Model

`.kapi/` directories (like `.git/`) contain:
- **config.yaml** — project settings, file mappings, locales
- **flows/** — custom YAML flow definitions
- **.sync-cache** — sync cache (gitignored, local only)

### Translation Flows

Composable pipelines that process files through tools:

```bash
# Run built-in AI translation flow
kapi flow run ai-translate

# Create custom flows in .kapi/flows/my-flow.yaml
# Run custom flow
kapi flow run my-flow
```

Flows automatically process all files matching `.kapi/config.yaml` mappings.

### Server Sync (Optional)

Push/pull workflow similar to git:

```bash
kapi status    # Show local changes
kapi diff      # Compare local vs. server
kapi pull      # Fetch from server
kapi push -m "message"  # Upload to server
```

Only changed blocks transfer (content-addressed sync).

## When to Use Kapi

Use Kapi CLI when you:

- **Work with files** in a git repository
- **Need local workflows** without server dependency
- **Want automation** via CI/CD pipelines
- **Prefer terminal** over GUI

Use Bowrain Desktop/Web when you:

- **Need visual editing** with split preview, context panel
- **Collaborate** with team members in real-time
- **Manage workspaces** and projects in a browser

## Installation

```bash
# macOS
brew install gokapi/tap/kapi

# Go install
go install github.com/gokapi/gokapi/cmd/kapi@latest

# Download binary
# Visit https://github.com/gokapi/gokapi/releases
```

## Quick Start

```bash
# Initialize project
cd my-app/
kapi init --name "My App" --source en-US --targets fr-FR,de-DE

# Run translation flow
kapi flow run ai-translate

# Check status
kapi status
```

## Next Steps

- [Installation](/docs/kapi-cli/installation)
- [Project Model](/docs/kapi-cli/project-model)
- [Commands Reference](/docs/kapi-cli/commands/init)
- [Flows](/docs/kapi-cli/flows/overview)
- [Use Cases](/docs/kapi-cli/use-cases/website-translation)
