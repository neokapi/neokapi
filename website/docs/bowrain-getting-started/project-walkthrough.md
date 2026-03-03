---
sidebar_position: 4
title: Project Walkthrough
slug: /bowrain/project-walkthrough
---

# Project Walkthrough

This guide demonstrates the Bowrain project model — a git-like `.bowrain/` directory
for managing localization workflows.

## Step 1: Initialize a Project

Create a new Bowrain project in your application directory:

```bash
cd my-app/
bowrain init
```

The interactive wizard guides you through setup. Choose **Local only** for a
local-only project, or use flags to skip the wizard:

```bash
bowrain init --name "My App Localization" --source en-US --targets fr-FR,de-DE,ja-JP
```

This creates:

```
my-app/
├── .bowrain/
│   ├── config.yaml       # Project settings
│   ├── flows/            # Custom workflow definitions
│   ├── .sync-cache       # Sync cache (gitignored)
│   └── .gitignore        # Auto-generated
└── src/
    └── locales/
        ├── en/
        │   └── messages.json
        ├── fr/
        └── de/
```

## Step 2: Configure File Mappings

Edit `.bowrain/config.yaml` to map your local files to the project structure:

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

Define a custom workflow in `.bowrain/flows/translate-with-qa.yaml`:

```yaml
name: translate-with-qa
description: AI translation with terminology enforcement and QA checks

steps:
  - tool: term-lookup
    config:
      termbase: .bowrain/termbase.tbx

  - tool: ai-translate
    config:
      provider: anthropic
      model: claude-sonnet-4.5
      temperature: 0.3

  - tool: term-enforce
    config:
      termbase: .bowrain/termbase.tbx
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
bowrain flow run translate-with-qa
```

The flow automatically:
- Reads files matching `.bowrain/config.yaml` mappings
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
bowrain status
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
Run 'bowrain auth login' to connect to a server
```

## Step 6: Connect to Bowrain Server (Optional)

If you're collaborating with a team, connect your project to a Bowrain Server.

### Option A: Sign In and Claim

If you already initialized a local project and want to move it to a server:

```bash
# Authenticate with the server
bowrain auth login --server https://bowrain.example.com

# Claim your local project into your personal workspace
bowrain auth claim
```

The claim transfers your anonymous local project into your personal workspace
on the server, preserving all files and translations. Your `.bowrain/config.yaml`
is updated with the server connection details.

### Option B: Interactive Init

Re-run `bowrain init` and choose **Sign in to Bowrain** to create a new
server-connected project from scratch:

```bash
bowrain init
# → Choose "Sign in to Bowrain"
# → Authenticate via browser (device flow)
# → Select workspace (or create a new one)
# → Enter project name and source locale
```

### After Connecting

Once connected, you can sync with the server:

```bash
# Push local translations to server
bowrain push -m "Translate UI strings for v2.0 release"

# Pull translations from team members
bowrain pull

# Show differences before pushing
bowrain diff

# Check sync status
bowrain status
```

## Step 7: Add Quality Hooks

Configure pre-push hooks to catch issues before upload:

Edit `.bowrain/config.yaml`:

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
bowrain push -m "Update translations"
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
git add .bowrain/config.yaml .bowrain/flows/
git commit -m "Add Bowrain project configuration"
```

**Do NOT commit:**
- `.bowrain/.sync-cache` — auto-gitignored (local sync cache)
- `.bowrain/.server-token` — auto-gitignored (auth credentials)

`bowrain init` creates a `.gitignore` automatically.

## Project Discovery

Bowrain CLI searches for `.bowrain/` by walking up the directory tree (like git):

```bash
cd my-app/src/locales/fr/
bowrain status  # Finds .bowrain/ at ../../../.bowrain/
bowrain flow run translate-with-qa  # Works from any subdirectory
```

All commands work from anywhere within the project tree.

## Local vs. Server Workflows

### Local-Only Workflow

If you don't need team collaboration:

1. `bowrain init` — initialize project (choose "Local only")
2. Define flows in `.bowrain/flows/`
3. `bowrain flow run <flow>` — process files
4. Commit results to git

No server required. Perfect for individual translators or small teams using git directly.

### Server-Connected Workflow

For team collaboration with Bowrain Server:

1. `bowrain auth login --server <URL>` — authenticate with the server
2. `bowrain auth claim` — claim your local project into a workspace, or re-run `bowrain init` with "Sign in to Bowrain"
3. `bowrain pull` — fetch latest translations
4. Edit files locally or run flows
5. `bowrain diff` — review changes
6. `bowrain push -m "message"` — upload to server
7. Repeat

Think of it as **git for localization content**:

| Git | Bowrain CLI |
|-----|------------|
| `git clone` | `bowrain init` (Sign in) |
| `git status` | `bowrain status` |
| `git diff` | `bowrain diff` |
| `git pull` | `bowrain pull` |
| `git add` | (automatic — based on file mappings) |
| `git commit -m` | `bowrain push -m` |
| `git push` | (part of `bowrain push`) |

## Next Steps

Now that you have a Bowrain project:

- **Explore flows**: [`bowrain flow list`](/docs/bowrain-cli/commands/flow)
- **Manage terminology**: [Terminology features](/docs/features/terminology)
- **Serve locally**: [`bowrain serve`](/docs/bowrain-cli/commands/serve)
- **Understand sync**: [Push](/docs/bowrain-cli/commands/push) and [Pull](/docs/bowrain-cli/commands/pull)
- **Read ADs**: [Architecture decisions](/docs/ad/001-vision)

For team deployments, see [Bowrain Server](/docs/developer/server).
