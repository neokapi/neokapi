---
sidebar_position: 2
title: Kapi Project Files
description: A Kapi project file (.kapi) is a portable YAML document that captures a localization workflow — source and target languages, content file patterns, tool pipelines, plugin requirements, and processing defaults.
keywords: [kapi project file, .kapi, YAML, localization workflow, portability, project format]
---

# Kapi Project Files

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
          model: claude-sonnet-4-5-20241022

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
| `parallel_blocks`  | int      | Goroutine fan-out for block processing |
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
