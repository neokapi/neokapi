---
sidebar_position: 3
title: Quick Start
---

# Quick Start

This guide walks through basic usage of the `kapi` CLI.

## Translate with AI

Translate a file using a built-in AI flow:

```bash
kapi flow run ai-translate -i input.html -o output.html --source-lang en --target-lang fr
```

## Pseudo-Translate for Testing

Generate pseudo-translations to test UI for truncation and RTL issues:

```bash
kapi pseudo-translate input.json --target-lang fr
```

## Word Count

Estimate translation costs:

```bash
kapi word-count docs/*.html
```

## Run a Multi-Step Flow

Use a flow that translates and then quality-checks the result:

```bash
kapi flow run ai-translate-qa -i docs/input.html -o out/output.html --source-lang en --target-lang fr
```

## List Supported Formats

```bash
kapi formats
```

## List Available Tools

```bash
kapi tools
```

## Open in the Web Editor

Start a local web UI for a project:

```bash
kapi serve project.kaz
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
