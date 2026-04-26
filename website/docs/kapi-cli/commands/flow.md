---
sidebar_position: 1
title: run
---

# kapi run

Run composed multi-tool flows on files.

## Synopsis

```bash
kapi run <flow-name> [flags]
kapi flows
```

## Description

The `kapi run` command executes a composed pipeline that chains multiple tools together. For single-tool operations, use the tool directly as a top-level command (e.g., `kapi ai-translate`, `kapi pseudo-translate`, `kapi qa-check`).

Use `kapi flows` to see available composed flows.

:::note
:::

## Examples

```bash
# Translate then quality-check (composed flow)
kapi run ai-translate-qa -i input.html -o output.html --source-lang en --target-lang fr

# List available composed flows
kapi flows
```

### Single-Tool Commands

Single-tool operations run directly as top-level commands:

```bash
# Translate with AI
kapi ai-translate -i input.html -o output.html --source-lang en --target-lang fr

# Pseudo-translate for testing
kapi pseudo-translate -i input.html -o output.html --target-lang fr

# Process multiple files in parallel
kapi ai-translate -i file1.html -i file2.html --source-lang en --target-lang fr -j 4

# Leverage translation memory
kapi tm-leverage -i input.html -o output.html --source-lang en --target-lang fr

# Run quality checks
kapi qa-check -i translations.html -o qa-report.html --target-lang fr
```

## Flags

| Flag            | Short | Description                                                                 |
| --------------- | ----- | --------------------------------------------------------------------------- |
| `--input`       | `-i`  | Input file path(s); repeat for multiple files (required)                    |
| `--output`      | `-o`  | Output file path (single-file mode only)                                    |
| `--format`      | `-f`  | Override input format detection                                             |
| `--encoding`    | `-e`  | Input encoding (default: UTF-8)                                             |
| `--source-lang` |       | Source language, BCP 47 (default: en)                                       |
| `--target-lang` |       | Target language, BCP 47 (required)                                          |
| `--concurrency` | `-j`  | Max parallel documents (0 = auto, 1 = sequential)                           |
| `--provider`    |       | LLM provider: anthropic, openai, ollama (default: anthropic)                |
| `--api-key`     |       | API key for LLM provider                                                    |
| `--model`       |       | LLM model name                                                              |
| `--tm`          |       | Named TM for tm-leverage (resolves from KAPI_HOME)                          |
| `--termbase`    |       | Named termbase for terminology lookup/enforcement (resolves from KAPI_HOME) |

:::note
The `--format`, `--encoding`, `--source-lang`, and `--target-lang` flags apply to
both `kapi run` and top-level tool commands. They are not global flags.
:::

## Built-in Composed Flows

| Flow              | Description                               |
| ----------------- | ----------------------------------------- |
| `ai-translate-qa` | Translate then quality check using AI/LLM |

## Built-in Tool Commands

Single-tool operations are available as top-level commands. See [tools](/kapi-cli/commands/tools) for the full list.

| Command                 | Description                                   |
| ----------------------- | --------------------------------------------- |
| `kapi ai-translate`     | Translate content using AI/LLM                |
| `kapi pseudo-translate` | Generate pseudo-translations for testing      |
| `kapi qa-check`         | Run rule-based quality checks on translations |
| `kapi tm-leverage`      | Pre-fill translations from translation memory |
| `kapi segmentation`     | Split source text into sentence segments      |
| `kapi word-count`       | Count words and characters per locale         |

## Use Cases

- [Terminology QA](/kapi-cli/use-cases/terminology-qa) — validate terminology compliance in translated files
- [Pre-Translation with Terminology](/kapi-cli/use-cases/terminology-pretranslation) — combine TM, AI, and terminology in a single workflow

## Listing Available Tools

```bash
kapi tools
```
