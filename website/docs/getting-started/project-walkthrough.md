---
sidebar_position: 4
title: Project Walkthrough
---

# Project Walkthrough

This guide demonstrates the Kapi project model — a git-like `.kapi/` directory
for managing localization workflows.

## Step 1: Initialize a Project

Create a new Kapi project in your application directory:

```bash
cd my-app/
kapi init --name "My App Localization" --source en-US --targets fr-FR,de-DE,ja-JP
```

This creates:

```
my-app/
├── .kapi/
│   ├── config.yaml       # Project settings
│   ├── flows/            # Custom workflow definitions
│   ├── .state.json       # Sync state (gitignored)
│   └── .gitignore        # Auto-generated
└── src/
    └── locales/
        ├── en/
        │   └── messages.json
        ├── fr/
        └── de/
```

## Step 2: Configure File Mappings

Edit `.kapi/config.yaml` to map your local files to the project structure:

```yaml
project:
  name: My App Localization
  source_locale: en-US
  target_locales:
    - fr-FR
    - de-DE
    - ja-JP

mappings:
  - local: src/locales/**/*.json
    remote: ui/strings/{path}
    format: json

  - local: content/*.md
    remote: docs/{filename}
    format: markdown
```

**Template expansion:**

| Local File | Remote Item |
|------------|-------------|
| `src/locales/en/messages.json` | `ui/strings/en/messages` |
| `src/locales/fr/buttons.json` | `ui/strings/fr/buttons` |
| `content/faq.md` | `docs/faq` |

## Step 3: Create a Translation Flow

Define a custom workflow in `.kapi/flows/translate-with-qa.yaml`:

```yaml
name: translate-with-qa
description: AI translation with terminology enforcement and QA checks

steps:
  - tool: term-lookup
    config:
      termbase: .kapi/termbase.tbx

  - tool: ai-translate
    config:
      provider: anthropic
      model: claude-sonnet-4.5
      temperature: 0.3

  - tool: term-enforce
    config:
      termbase: .kapi/termbase.tbx
      required: true

  - tool: qa-check
    config:
      rules:
        - whitespace
        - punctuation
        - placeholders
        - terminology
```

## Step 4: Run the Flow

Execute your custom flow:

```bash
kapi flow run translate-with-qa
```

The flow automatically:
- Reads files matching `.kapi/config.yaml` mappings
- Uses configured source/target locales
- Processes through all tools in sequence
- Writes results back to local files

**Output:**

```
Project: My App Localization
Flow: translate-with-qa (AI translation with QA checks)

Processing files:
  ✓ src/locales/en/messages.json → fr/messages.json (42 blocks)
  ✓ src/locales/en/messages.json → de/messages.json (42 blocks)
  ✓ src/locales/en/messages.json → ja/messages.json (42 blocks)
  ✓ src/locales/en/buttons.json → fr/buttons.json (8 blocks)

Running term-enforce...
  ✓ No violations found

Running qa-check...
  ⚠ 2 warnings in de/messages.json:
    - Block 'welcome_message': extra whitespace at end
    - Block 'error_text': placeholder format inconsistency

Flow completed: 200 blocks translated
```

## Step 5: Check Status

View what changed:

```bash
kapi status
```

**Output:**

```
Project: My App Localization
Root: /Users/me/my-app

Last pull: never
Last push: never

Modified local files:
  M src/locales/fr/messages.json
  M src/locales/de/messages.json
  M src/locales/ja/messages.json
  M src/locales/fr/buttons.json

No server configured
Run 'kapi init --server <URL> --project <ID>' to connect
```

## Step 6: Connect to Bowrain Server (Optional)

If you're collaborating with a team, connect to a Bowrain Server instance:

```bash
kapi init --server https://bowrain.example.com --project abc123
```

This updates `.kapi/config.yaml`:

```yaml
server:
  url: https://bowrain.example.com
  project_id: abc123
```

Now you can sync with the server:

```bash
# Push local translations to server
kapi push -m "Translate UI strings for v2.0 release"

# Pull translations from team members
kapi pull

# Show differences before pushing
kapi diff

# Check sync status
kapi status
```

## Step 7: Add Quality Hooks

Configure pre-push hooks to catch issues before upload:

Edit `.kapi/config.yaml`:

```yaml
hooks:
  pre-push:
    - qa-check
    - term-enforce

  post-pull:
    - segmentation
```

When you push, hooks run automatically:

```bash
kapi push -m "Update translations"
```

**Output with hook failure:**

```
Running pre-push hooks: [qa-check, term-enforce]
✓ qa-check: 0 issues found
✗ term-enforce: 2 violations (blocking)

Block 'login_button':
  Missing required term "sign in" (found "log in")

Block 'password_field':
  Incorrect translation of "password" → use "mot de passe"

Push aborted. Fix issues or use --force to bypass.
```

## Step 8: Version Control

Commit your project configuration to git:

```bash
git add .kapi/config.yaml .kapi/flows/
git commit -m "Add Kapi project configuration"
```

**Do NOT commit:**
- `.kapi/.state.json` — auto-gitignored (local sync state)
- `.kapi/.server-token` — auto-gitignored (auth credentials)

`kapi init` creates a `.gitignore` automatically.

## Project Discovery

Kapi searches for `.kapi/` by walking up the directory tree (like git):

```bash
cd my-app/src/locales/fr/
kapi status  # Finds .kapi/ at ../../../.kapi/
kapi flow run translate-with-qa  # Works from any subdirectory
```

All commands work from anywhere within the project tree.

## Local vs. Server Workflows

### Local-Only Workflow

If you don't need team collaboration:

1. `kapi init` — initialize project
2. Define flows in `.kapi/flows/`
3. `kapi flow run <flow>` — process files
4. Commit results to git

No server required. Perfect for individual translators or small teams using git directly.

### Server-Connected Workflow

For team collaboration with Bowrain Server:

1. `kapi init --server <URL> --project <ID>` — connect to server
2. `kapi pull` — fetch latest translations
3. Edit files locally or run flows
4. `kapi diff` — review changes
5. `kapi push -m "message"` — upload to server
6. Repeat

Think of it as **git for localization content**:

| Git | Kapi |
|-----|------|
| `git clone` | `kapi init --server` |
| `git status` | `kapi status` |
| `git diff` | `kapi diff` |
| `git pull` | `kapi pull` |
| `git add` | (automatic — based on file mappings) |
| `git commit -m` | `kapi push -m` |
| `git push` | (part of `kapi push`) |

## Next Steps

Now that you have a Kapi project:

- **Explore flows**: [`kapi flow list`](/docs/kapi-cli/commands/flow)
- **Manage terminology**: [Terminology features](/docs/features/terminology)
- **Serve locally**: [`kapi serve`](/docs/kapi-cli/commands/serve)
- **Understand sync**: [Push](/docs/kapi-cli/commands/push) and [Pull](/docs/kapi-cli/commands/pull)
- **Read ADRs**: [Architecture decisions](/docs/adr/001-vision)

For team deployments, see [Bowrain Server](/docs/developer/server).
