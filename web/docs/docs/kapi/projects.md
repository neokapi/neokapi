---
sidebar_position: 2
title: Kapi Projects
description: A Kapi project (.kapi) is a portable YAML document that captures a localization workflow — source and target languages, content file patterns, tool pipelines, plugin requirements, and processing defaults. Compare it with ad-hoc, flag-driven runs.
keywords: [kapi project, .kapi, ad-hoc, YAML, localization workflow, portability, project format]
---

# Kapi Projects

There are two ways to drive kapi:

- **Ad-hoc** — run a tool or flow directly on files, configured entirely by flags: `kapi ai-translate -i file.json --target-lang fr`. Nothing is saved; ideal for one-off jobs and scripting.
- **Project** — capture the same workflow once in a portable `.kapi` file and replay it with `kapi run <flow> -p my-app.kapi`. Ideal for a repository you localize repeatedly, and shareable via git.

A Kapi project file is a YAML document that captures a localization workflow. It defines source and target languages, content file patterns, tool pipelines (flows), plugin requirements, and processing defaults.

## Format

```yaml
version: v1
name: My App Localization

content:
  - path: "src/locales/en/*.json"
    format: json
    target: "src/locales/{lang}/*.json"

preset: nextjs
plugins:
  okapi: "^1.47.0"

flows:
  translate:
    steps:
      - tool: ai-translate
        config:
          provider: anthropic
          model: claude-sonnet-4-20250514

  translate-and-qa:
    steps:
      - tool: ai-translate
        config:
          provider: anthropic
      - tool: qa-check

  pseudo:
    steps:
      - tool: pseudo-translate
        config:
          expansion_rate: 1.3

defaults:
  source_language: en-US
  target_languages:
    - fr-FR
    - de-DE
    - ja-JP
  concurrency: 4
  parallel_blocks: 3
  encoding: utf-8
```

## Fields

### Required

| Field     | Type   | Description                    |
| --------- | ------ | ------------------------------ |
| `version` | string | Schema version, must be `"v1"` |
| `name`    | string | Project display name           |

### Optional

| Field              | Type           | Description                                          |
| ------------------ | -------------- | ---------------------------------------------------- |
| `content`          | ContentEntry[] | File patterns to process                             |
| `preset`           | string         | Framework preset name (e.g., `nextjs`, `react-intl`) |
| `plugins`          | map            | Plugin requirements keyed by name (e.g., `okapi: "^1.47.0"`) |
| `flows`            | map            | Named flow definitions                               |
| `defaults`         | Defaults       | Processing defaults (source/target languages live here) |

### ContentEntry

| Field    | Type   | Description                                   |
| -------- | ------ | --------------------------------------------- |
| `path`   | string | Glob pattern for source files (required)      |
| `format` | string | Format ID; auto-detected if omitted           |
| `target` | string | Output path pattern with `{lang}` placeholder |

### Defaults

| Field              | Type     | Description                            |
| ------------------ | -------- | -------------------------------------- |
| `source_language`  | string   | BCP-47 source locale (e.g., `en-US`)   |
| `target_languages` | string[] | BCP-47 target locales                  |
| `concurrency`      | int      | Number of files to process in parallel |
| `parallel_blocks`  | int      | Number of blocks to process in parallel within a file |
| `encoding`         | string   | Input file encoding (default: `utf-8`) |

### Flow Steps

Each flow contains an ordered list of steps. See [Flow Steps Format](/contribute/notes-internal/flow-steps-format) for the full specification.

```yaml
flows:
  my-flow:
    steps:
      - tool: ai-translate # tool name (required)
        config: # tool-specific config (optional)
          provider: anthropic
        label: "Translate" # display label (optional)
```

Steps can also define parallel branches:

```yaml
flows:
  parallel-qa:
    steps:
      - parallel:
          - tool: qa-check
          - tool: ai-qa
```

## Key Properties

- **No credentials** — API keys are never stored in Kapi project files. They come from the OS keychain (Kapi) or environment variables (CLI).
- **No state** — No sync cursors, caches, or timestamps. Kapi project files are always clean and safe to commit.
- **Portable** — Save anywhere, have multiple per directory, share via git.
- **CLI-compatible** — `kapi run flowname -p file.kapi`

## Using with Kapi CLI

```bash
# Run a flow from a Kapi project
kapi run translate -p translation.kapi

# Override defaults with CLI flags
kapi run translate -p translation.kapi --target-lang de --provider openai

# One-shot mode still works without a project
kapi ai-translate -i file.json --target-lang fr
```

## Using with Kapi

- **File > Open** to load a Kapi project
- **File > New** to create one from scratch
- **File > Save / Save As** for standard document operations
- Double-click a Kapi project to open it in Kapi (macOS)
- Drag and drop Kapi projects onto the app window
