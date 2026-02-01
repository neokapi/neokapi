---
sidebar_position: 4
title: flow
---

# kapi flow

Run multi-step processing flows.

## Synopsis

```bash
kapi flow run --input <path> --output <path> --tools <tool1,tool2,...> [flags]
```

## Description

The `flow` command orchestrates a sequence of tools into a processing pipeline. Documents are read, streamed through each tool in order, and written to the output. Multiple documents can be processed in parallel.

## Examples

```bash
# Translation flow with TM and AI
kapi flow run --input docs/ --output out/ \
  --tools segmentation,tm-leverage,ai-translate \
  -s en -t fr

# Word count flow
kapi flow run --input docs/ \
  --tools wordcount \
  -s en

# Quality assurance flow
kapi flow run --input translations/ --output qa-report/ \
  --tools ai-qa \
  -s en -t fr

# Multi-tool flow with parallelism
kapi flow run --input docs/ --output out/ \
  --tools segmentation,tm-leverage,ai-translate,ai-qa \
  -s en -t fr \
  --concurrency 8
```

## Flags

| Flag | Short | Description |
|------|-------|-------------|
| `--input` | `-i` | Input file or directory (required) |
| `--output` | `-o` | Output file or directory |
| `--tools` | | Comma-separated list of tools |
| `--source-lang` | `-s` | Source language |
| `--target-lang` | `-t` | Target language |
| `--concurrency` | | Max parallel documents (default: CPU count) |
| `--channel-size` | | Channel buffer size (default: 64) |
| `--fail-fast` | | Stop on first error (default: true) |

## Built-in Flows

gokapi ships with five built-in flow definitions:

| Flow | Description | Tools |
|------|-------------|-------|
| `ai-translate` | Translate content using AI/LLM | ai-translate |
| `ai-translate-qa` | Translate then quality check | ai-translate, ai-qa |
| `pseudo-translate` | Generate pseudo-translations for testing | pseudo-translate |
| `qa-check` | Run rule-based quality checks on translations | qa-check |
| `tm-leverage` | Pre-fill translations from translation memory | tm-leverage |

Run a built-in flow by name:

```bash
kapi flow run --flow ai-translate-qa --input docs/ --output out/ -s en -t fr
```

## Flow Definitions

Flow definitions describe a processing graph with nodes and edges:

- **Nodes** represent processing steps: `reader` (input), `tool` (processing), `writer` (output)
- **Edges** define the data flow direction between nodes
- Flows are validated for cycles and dangling references before execution

User-created flows are stored as JSON files in `~/.config/gokapi/flows/` and can be edited visually in the Bowrain desktop app.

## Listing Available Tools

```bash
kapi tools list
```
