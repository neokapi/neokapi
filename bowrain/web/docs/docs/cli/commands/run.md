---
sidebar_position: 6
title: run
---

# kapi run

Run composed multi-tool flows or custom project flows. For single built-in tools, use the top-level tool commands directly (e.g., `kapi ai-translate`, `kapi qa-check`).

## Synopsis

```bash
kapi run <flow-name> [flags]
kapi flows
```

## Description

The `kapi run` command executes a named multi-step processing pipeline. Where content comes from and goes to is a binding: a source is read, streamed through each tool, and written to the sink. Ad-hoc, the sink is a file (`-o`); inside a project with no `-o`, the run is *process-only* — it commits results to the project store and `kapi merge` materializes the files. Multiple input files can be processed in parallel. Use `--explain` to print the resolved `source → sink` without running.

**Project-based flows**: If a `.kapi` project exists (a `*.kapi` recipe found by walking up the tree), flows are loaded from inline `flows:` on the recipe and from `.kapi/flows/*.yaml`. This is the primary mode for the bowrain plugin.

**Built-in composed flows**: Multi-tool pipelines like `ai-translate-qa` are available as built-in flows.

**Single tools as top-level commands**: Individual tools run directly as top-level commands — `kapi ai-translate`, `kapi pseudo-translate`, `kapi qa-check`, `kapi tm-leverage`, etc.

Use `kapi flows` to see available flows, or `kapi tools` to see available tools.

## Examples

```bash
# Translate with AI (top-level tool command)
kapi ai-translate -i input.html -o output.html --source-lang en --target-lang fr

# Translate then quality-check (composed flow)
kapi run ai-translate-qa -i input.html -o output.html --source-lang en --target-lang fr

# Pseudo-translate for testing (top-level tool command)
kapi pseudo-translate input.html -o output.html --target-lang fr

# Process multiple files in parallel (top-level tool command)
kapi ai-translate -i file1.html -i file2.html --source-lang en --target-lang fr -j 4

# Leverage translation memory (top-level tool command)
kapi tm-leverage -i input.html -o output.html --source-lang en --target-lang fr

# Run quality checks (top-level tool command)
kapi qa-check -i translations.html -o qa-report.html --target-lang fr

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
| `--provider`    |       | LLM provider: anthropic, openai, ollama (default: anthropic) |
| `--api-key`     |       | API key for LLM provider                                     |
| `--model`       |       | LLM model name                                               |

:::note
The `--format`, `--encoding`, `--source-lang`, and `--target-lang` flags are
specific to `kapi run` and tool commands. They are not global flags.
:::

## Project-Based Flows

If you've initialized a Bowrain project with `kapi init`, create custom flows in `.kapi/flows/`:

```yaml
# .kapi/flows/translate-review.yaml
name: translate-review
description: Translate with AI then run QA checks

steps:
  - tool: ai-translate
    config:
      provider: anthropic
      model: claude-sonnet-4.5

  - tool: qa-check
    config:
      rules:
        - whitespace
        - punctuation
        - placeholders

  - tool: term-enforce
    config:
      termbase: project.tbx
      required: true
```

Run with:

```bash
kapi run translate-review
```

Project flows automatically use the recipe's content collections and locale defaults.
No need to specify `--input`, `--output`, `--source-lang`, or `--target-lang`. A
project run is process-only — results land in the project store; run
[`kapi merge`](/cli/commands/merge) to write the localized files.

## Built-in Composed Flows

Without a `.kapi` project, you can use built-in composed flows with explicit flags:

```bash
kapi run ai-translate-qa -i input.html -o output.html --source-lang en --target-lang fr
```

Available built-in composed flows:

| Flow              | Description                               |
| ----------------- | ----------------------------------------- |
| `ai-translate-qa` | Translate then quality check using AI/LLM |
| `segmentation`    | Split source text into sentence segments  |

## Top-Level Tool Commands

Single tools run directly as top-level commands:

| Command                    | Description                                   |
| -------------------------- | --------------------------------------------- |
| `kapi ai-translate`     | Translate content using AI/LLM                |
| `kapi pseudo-translate` | Generate pseudo-translations for testing      |
| `kapi qa-check`         | Run rule-based quality checks on translations |
| `kapi tm-leverage`      | Pre-fill translations from translation memory |

## Listing Available Tools

```bash
kapi tools
```
