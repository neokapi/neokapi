---
sidebar_position: 6
title: run
---

# kapi run

Run composed multi-tool flows or custom project flows. For single built-in tools, use the top-level tool commands directly (for example `kapi translate`, `kapi exec qa`).

## Synopsis

```bash
kapi run <flow-name> [flags]
kapi flows
```

## Description

The `kapi run` command executes a named multi-step processing pipeline. Where content comes from and goes to is a binding: a source is read, streamed through each tool, and written to the sink. Ad-hoc, the sink is a file (`-o`); inside a project with no `-o`, the run is *process-only*: it commits results to the project store and `kapi merge` materializes the files. Multiple input files can be processed in parallel. Use `--explain` to print the resolved `source → sink` without running.

**Project-based flows**: If a `.kapi` project exists (a `kapi.yaml` recipe found by walking up the tree), flows are loaded from inline `flows:` on the recipe and from `.kapi/flows/*.yaml`. This is the primary mode for the bowrain plugin.

**Built-in composed flows**: Multi-tool pipelines like `translate-qa` are available as built-in flows.

**Single tools as top-level commands**: Individual tools run directly as top-level commands: `kapi translate`, `kapi pseudo-translate`, `kapi exec qa`, `kapi exec recycle`, and so on.

Use `kapi flows` to see available flows, or `kapi tools` to see available tools.

## Examples

```bash
# Draft with AI (top-level tool command)
kapi translate -i input.html -o output.html --source-lang en --target-lang fr

# Draft then quality-check (composed flow)
kapi run translate-qa -i input.html -o output.html --source-lang en --target-lang fr

# Pseudo-translate for testing (top-level tool command)
kapi pseudo-translate input.html -o output.html --target-lang fr

# Process multiple files in parallel (top-level tool command)
kapi translate -i file1.html -i file2.html --source-lang en --target-lang fr -j 4

# Reuse from content memory (top-level tool command)
kapi exec recycle -i input.html -o output.html --source-lang en --target-lang fr

# Run quality checks (top-level tool command)
kapi exec qa -i translations.html -o check-report.html --target-lang fr

# Run a custom project flow
kapi run translate-review

# List available flows
kapi flows

# List available tools
kapi tools
```

## Flags (kapi run)

| Flag            | Short | Description                                                  |
| --------------- | ----- | ------------------------------------------------------------ |
| `--input`       | `-i`  | Input file path(s); repeat for multiple files (required)     |
| `--output`      | `-o`  | Output file path (single-file mode); omit in a project for a process-only run to the store |
| `--explain`     |       | Print the resolved `source → sink` bindings and exit without running |
| `--format`      | `-f`  | Override input format detection                              |
| `--encoding`    | `-e`  | Input encoding (default: UTF-8)                              |
| `--source-lang` |       | Source language, BCP 47 (default: en)                        |
| `--target-lang` |       | Target language, BCP 47 (required)                           |
| `--concurrency` | `-j`  | Max parallel documents (0 = auto, 1 = sequential)            |
| `--provider`    |       | AI provider; any the [translate tool](https://neokapi.github.io/reference/tools/translate) supports |
| `--api-key`     |       | API key for the AI provider                                  |
| `--model`       |       | Model name                                                   |

:::note
The `--format`, `--encoding`, `--source-lang`, and `--target-lang` flags are
specific to `kapi run` and tool commands. They are not global flags.
:::

## Project-based flows

If you've initialized a project with `kapi init`, create custom flows in `.kapi/flows/`:

```yaml
# .kapi/flows/translate-review.yaml
name: translate-review
description: Draft with AI then run the checks

steps:
  - tool: translate
    config:
      provider: anthropic
      model: claude-sonnet-5

  - tool: qa
    config:
      checkDoubleSpaces: true
      checkPatterns: true

  - tool: term-check
    config:
      caseSensitive: false
```

The term step checks against the project's terms, compiled from the
source the recipe binds (`defaults.terms_source`), not one configured per step;
`--termstore` overrides it for a single run.

Run with:

```bash
kapi run translate-review
```

Project flows automatically use the recipe's content collections and locale defaults.
No need to specify `--input`, `--output`, `--source-lang`, or `--target-lang`. A
project run is process-only: results land in the project store; run
`kapi merge` to write the target files.

## Built-in composed flows

Without a `.kapi` project, you can use built-in composed flows with explicit flags:

```bash
kapi run translate-qa -i input.html -o output.html --source-lang en --target-lang fr
```

Available built-in composed flows:

| Flow              | Description                               |
| ----------------- | ----------------------------------------- |
| `translate-qa`    | Draft then quality check with an AI provider |
| `segmentation`    | Split source text into sentence segments  |

## Top-level tool commands

Single tools run directly as top-level commands:

| Command                    | Description                                   |
| -------------------------- | --------------------------------------------- |
| `kapi translate`     | Draft content with an AI provider             |
| `kapi pseudo-translate` | Generate pseudo-translations for testing      |
| `kapi exec qa`         | Run rule-based quality checks                 |
| `kapi exec recycle`      | Pre-fill targets from content memory        |

## Listing available tools

```bash
kapi tools
```
