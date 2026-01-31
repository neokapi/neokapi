---
sidebar_position: 3
title: Quick Start
---

# Quick Start

This guide walks through basic usage of the `kapi` CLI.

## Convert a Document

Convert an HTML file to XLIFF for translation:

```bash
kapi convert input.html -o output.xliff -s en -t fr
```

## Translate with AI

Translate a file using an AI provider:

```bash
kapi translate input.html -o output.html -s en -t fr --provider anthropic
```

## Extract and Merge

Extract translatable content to XLIFF, translate externally, then merge back:

```bash
# Extract
kapi extract input.html -o translations.xliff -s en -t fr

# ... translate translations.xliff with your preferred tool ...

# Merge translations back
kapi merge translations.xliff -o output.html
```

## Run a Flow

Define a multi-step processing flow:

```bash
kapi flow run --input docs/ --output out/ \
  --tools segmentation,tm-leverage,ai-translate \
  -s en -t fr
```

## List Supported Formats

```bash
kapi formats list
```

## List Available Tools

```bash
kapi tools list
```

## Configuration

Create a `gokapi.yaml` file in your project root for persistent settings:

```yaml
formats:
  html:
    preserveWhitespace: false

tools:
  ai-translation:
    provider: anthropic
    model: claude-sonnet-4-20250514
```

See the [Configuration](/docs/user-guide/configuration) page for all options.
