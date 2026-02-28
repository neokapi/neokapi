---
sidebar_position: 3
title: Project Model
---

# Brain Project Model

Brain uses a `.brain/` directory (like `.git`) to manage localization projects within your repository.

## Directory Structure

```
my-app/
├── .brain/
│   ├── config.yaml       # Project configuration
│   ├── flows/            # Custom flow definitions
│   │   └── my-flow.yaml
│   ├── .sync-cache       # Sync cache (gitignored)
│   ├── .server-token     # Auth token (gitignored)
│   └── .gitignore        # Auto-generated
├── src/
│   └── locales/
│       ├── en/
│       │   └── messages.json
│       └── fr/
│           └── messages.json
└── .gitignore            # Add .brain/.sync-cache
```

## config.yaml

The main configuration file defines project metadata, file mappings, and settings:

```yaml
project:
  name: My App Localization
  source_locale: en-US
  target_locales:
    - fr-FR
    - de-DE
    - ja-JP

# Optional: connect to Bowrain Server
server:
  url: https://bowrain.example.com
  project_id: abc123
  workspace: my-team

# File mappings: local paths <-> remote items
mappings:
  - local: src/locales/**/*.json
    remote: ui/strings/{path}
    format: json
  - local: content/*.md
    remote: docs/{filename}
    format: markdown

# Hooks: tool chains to run before/after sync
hooks:
  pre-push:
    - qa-check
    - term-enforce
  post-pull:
    - segmentation
```

## File Mappings

Mappings define how local files correspond to remote translation items.

### Template Variables

| Variable | Description | Example |
|----------|-------------|---------|
| `\{path\}` | Full relative path without extension | `en/messages` |
| `\{filename\}` | Filename with extension | `messages.json` |
| `\{basename\}` | Filename without extension | `messages` |

### Examples

```yaml
mappings:
  # Pattern: src/locales/en/buttons.json
  # -> Remote: ui/strings/en/buttons
  - local: src/locales/**/*.json
    remote: ui/strings/{path}
    format: json

  # Pattern: content/faq.md
  # -> Remote: docs/faq.md
  - local: content/*.md
    remote: docs/{filename}
    format: markdown

  # Pattern: data/settings.yaml
  # -> Remote: config/settings
  - local: data/*.yaml
    remote: config/{basename}
    format: yaml
```

## Project Discovery

Brain searches for `.brain/` by walking up the directory tree (like git):

```bash
cd my-app/src/locales/fr/
brain status  # Finds .brain/ at ../../../.brain/
```

All commands work from any subdirectory within the project.

## Version Control

### Commit to git

Files to commit:
- `.brain/config.yaml` — project settings
- `.brain/flows/*.yaml` — flow definitions

### Do NOT commit

Files that should NOT be committed (auto-gitignored):
- `.brain/.sync-cache` — local sync cache (block hashes + cursor)
- `.brain/.server-token` — authentication token

`brain init` automatically creates `.brain/.gitignore` with these entries.

## Initialization

Create a new Brain project:

```bash
cd my-app/
brain init --name "My App" --source en-US --targets fr-FR,de-DE,ja-JP
```

This creates:
1. `.brain/` directory
2. `.brain/config.yaml` with specified settings
3. `.brain/flows/` for custom flows
4. `.brain/.gitignore` to exclude cache and token files

## Server Connection (Optional)

Connect an existing project to Bowrain Server:

```bash
brain init --server https://bowrain.example.com --project abc123
```

This updates `.brain/config.yaml` with server details. You can then:

```bash
brain push    # Upload local source blocks to server
brain pull    # Fetch translated blocks from server
brain status  # Show sync state (pending push/pull)
```

## Next Steps

- [Initialize a Project](/docs/brain-cli/commands/init)
- [Configure File Mappings](/docs/brain-cli/commands/init#configuration-file)
- [Custom Flows](/docs/brain-cli/flows/custom-flows)
- [Server Sync](/docs/brain-cli/commands/push)
