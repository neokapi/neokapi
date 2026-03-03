---
sidebar_position: 3
title: Project Model
---

# Bowrain Project Model

Bowrain CLI uses a `.bowrain/` directory (like `.git`) to manage localization projects within your repository.

## Directory Structure

```
my-app/
├── .bowrain/
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
└── .gitignore            # Add .bowrain/.sync-cache
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

Bowrain CLI searches for `.bowrain/` by walking up the directory tree (like git):

```bash
cd my-app/src/locales/fr/
bowrain status  # Finds .bowrain/ at ../../../.bowrain/
```

All commands work from any subdirectory within the project.

## Version Control

### Commit to git

Files to commit:
- `.bowrain/config.yaml` — project settings
- `.bowrain/flows/*.yaml` — flow definitions

### Do NOT commit

Files that should NOT be committed (auto-gitignored):
- `.bowrain/.sync-cache` — local sync cache (block hashes + cursor)
- `.bowrain/.server-token` — authentication token

`bowrain init` automatically creates `.bowrain/.gitignore` with these entries.

## Initialization

Create a new Bowrain project:

```bash
cd my-app/
bowrain init --name "My App" --source en-US --targets fr-FR,de-DE,ja-JP
```

This creates:
1. `.bowrain/` directory
2. `.bowrain/config.yaml` with specified settings
3. `.bowrain/flows/` for custom flows
4. `.bowrain/.gitignore` to exclude cache and token files

## Server Connection (Optional)

Connect an existing project to Bowrain Server:

```bash
bowrain init --server https://bowrain.example.com --project abc123
```

This updates `.bowrain/config.yaml` with server details. You can then:

```bash
bowrain push    # Upload local source blocks to server
bowrain pull    # Fetch translated blocks from server
bowrain status  # Show sync state (pending push/pull)
```

## Next Steps

- [Initialize a Project](/docs/bowrain-cli/commands/init)
- [Configure File Mappings](/docs/bowrain-cli/commands/init#configuration-file)
- [Custom Flows](/docs/bowrain-cli/flows/custom-flows)
- [Server Sync](/docs/bowrain-cli/commands/push)
