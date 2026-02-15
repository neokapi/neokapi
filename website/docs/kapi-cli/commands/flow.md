---
sidebar_position: 6
title: flow
---

# kapi flow

Run multi-step processing flows. Flows can be defined in `.kapi/flows/` (project-based)
or use built-in legacy flows.

## Synopsis

```bash
kapi flow run <flow-name> [flags]
kapi flow list
```

## Description

The `flow run` command executes a named processing pipeline. Documents are read, streamed through each tool in the flow, and written to the output. Multiple input files can be processed in parallel.

**Project-based flows**: If a `.kapi/` project exists, flows are loaded from `.kapi/flows/*.yaml` files.

**Built-in flows**: Without a project, built-in flows can still be executed.

Use `flow list` to see available flows.

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
| `--concurrency` | `-j` | Max parallel documents (0 = auto, 1 = sequential) |
| `--provider` | | LLM provider: anthropic, openai, ollama (default: anthropic) |
| `--api-key` | | API key for LLM provider |
| `--model` | | LLM model name |
| `--source-lang` | | Source language, BCP 47 (default: en) |
| `--target-lang` | | Target language, BCP 47 (required) |

## Project-Based Flows

If you've initialized a Kapi project with `kapi init`, create custom flows in `.kapi/flows/`:

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
kapi flow run translate-review
```

Project flows automatically use file mappings and locales from `.kapi/config.yaml`.
No need to specify `--input`, `--output`, `--source-lang`, or `--target-lang`.

## Built-in Flows

Without a `.kapi/` project, you can still use built-in flows with explicit flags:

```bash
kapi flow run ai-translate -i input.html -o output.html --source-lang en --target-lang fr
```

Available built-in flows:

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
