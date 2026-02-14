---
sidebar_position: 3
title: Project Model
---

# Kapi Project Model

Kapi uses a `.kapi/` directory (like `.git`) to manage localization projects within your repository.

## Directory Structure

```
my-app/
├── .kapi/
│   ├── config.yaml       # Project configuration
│   ├── flows/            # Custom flow definitions
│   │   └── my-flow.yaml
│   ├── .state.json       # Sync state (gitignored)
│   ├── .server-token     # Auth token (gitignored)
│   └── .gitignore        # Auto-generated
├── src/
│   └── locales/
│       ├── en/
│       │   └── messages.json
│       └── fr/
│           └── messages.json
└── .gitignore            # Add .kapi/.state.json
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

# File mappings: local paths ↔ remote items
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
| `{path}` | Full relative path without extension | `en/messages` |
| `{filename}` | Filename with extension | `messages.json` |
| `{basename}` | Filename without extension | `messages` |

### Examples

```yaml
mappings:
  # Pattern: src/locales/en/buttons.json
  # → Remote: ui/strings/en/buttons
  - local: src/locales/**/*.json
    remote: ui/strings/{path}
    format: json

  # Pattern: content/faq.md
  # → Remote: docs/faq.md
  - local: content/*.md
    remote: docs/{filename}
    format: markdown

  # Pattern: data/settings.yaml
  # → Remote: config/settings
  - local: data/*.yaml
    remote: config/{basename}
    format: yaml
```

## Project Discovery

Kapi searches for `.kapi/` by walking up the directory tree (like git):

```bash
cd my-app/src/locales/fr/
kapi status  # Finds .kapi/ at ../../../.kapi/
```

All commands work from any subdirectory within the project.

## Version Control

### Commit to git

Files to commit:
- `.kapi/config.yaml` — project settings
- `.kapi/flows/*.yaml` — flow definitions

### Do NOT commit

Files that should NOT be committed (auto-gitignored):
- `.kapi/.state.json` — local sync state
- `.kapi/.server-token` — authentication token

`kapi init` automatically creates `.kapi/.gitignore` with these entries.

## Initialization

Create a new Kapi project:

```bash
cd my-app/
kapi init --name "My App" --source en-US --targets fr-FR,de-DE,ja-JP
```

This creates:
1. `.kapi/` directory
2. `.kapi/config.yaml` with specified settings
3. `.kapi/flows/` for custom flows
4. `.kapi/.gitignore` to exclude state files

## Server Connection (Optional)

Connect an existing project to Bowrain Server:

```bash
kapi init --server https://bowrain.example.com --project abc123
```

This updates `.kapi/config.yaml` with server details. You can then:

```bash
kapi pull   # Fetch translations from server
kapi push   # Upload local changes to server
```

## Next Steps

- [Initialize a Project](/docs/kapi-cli/commands/init)
- [Configure File Mappings](/docs/kapi-cli/commands/init#configuration-file)
- [Custom Flows](/docs/kapi-cli/flows/custom-flows)
- [Server Sync](/docs/kapi-cli/commands/push)
