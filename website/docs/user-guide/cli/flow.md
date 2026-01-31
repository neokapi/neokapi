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

## Listing Available Tools

```bash
kapi tools list
```
