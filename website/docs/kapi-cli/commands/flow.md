---
sidebar_position: 1
title: flow
---

# kapi flow

Run multi-step processing flows on files.

## Synopsis

```bash
kapi flow run <flow-name> [flags]
kapi flow list
```

## Description

The `flow run` command executes a named processing pipeline. Documents are read, streamed through each tool in the flow, and written to the output. Multiple input files can be processed in parallel.

Use `flow list` to see available flows.

:::note
For project-based flows with automatic file mappings and locale configuration, see [Bowrain CLI flow command](/docs/bowrain-cli/commands/flow).
:::

## Examples

```bash
# Translate with AI
kapi flow run ai-translate -i input.html -o output.html --source-lang en --target-lang fr

# Translate then quality-check
kapi flow run ai-translate-qa -i input.html -o output.html --source-lang en --target-lang fr

# Pseudo-translate for testing
kapi flow run pseudo-translate -i input.html -o output.html --target-lang fr

# Process multiple files in parallel
kapi flow run ai-translate -i file1.html -i file2.html --source-lang en --target-lang fr -j 4

# Leverage translation memory
kapi flow run tm-leverage -i input.html -o output.html --source-lang en --target-lang fr

# Run quality checks
kapi flow run qa-check -i translations.html -o qa-report.html --target-lang fr

# List available flows
kapi flow list
```

## Flags (flow run)

| Flag | Short | Description |
|------|-------|-------------|
| `--input` | `-i` | Input file path(s); repeat for multiple files (required) |
| `--output` | `-o` | Output file path (single-file mode only) |
| `--format` | `-f` | Override input format detection |
| `--encoding` | `-e` | Input encoding (default: UTF-8) |
| `--source-lang` | | Source language, BCP 47 (default: en) |
| `--target-lang` | | Target language, BCP 47 (required) |
| `--concurrency` | `-j` | Max parallel documents (0 = auto, 1 = sequential) |
| `--provider` | | LLM provider: anthropic, openai, ollama (default: anthropic) |
| `--api-key` | | API key for LLM provider |
| `--model` | | LLM model name |

:::note
The `--format`, `--encoding`, `--source-lang`, and `--target-lang` flags are
specific to `flow run` and tool commands. They are not global flags.
:::

## Built-in Flows

| Flow | Description |
|------|-------------|
| `ai-translate` | Translate content using AI/LLM |
| `ai-translate-qa` | Translate then quality check using AI/LLM |
| `pseudo-translate` | Generate pseudo-translations for testing |
| `qa-check` | Run rule-based quality checks on translations |
| `tm-leverage` | Pre-fill translations from translation memory |
| `segmentation` | Split source text into sentence segments |

## Listing Available Tools

```bash
kapi tools
```
