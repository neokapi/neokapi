---
sidebar_position: 1
title: Overview
---

# Translation Flows

Flows are composable pipelines that process localization files through a sequence of tools.

## What Are Flows?

A flow is a multi-step processing pipeline where each step transforms the content:

```
Input Files -> [Tool 1] -> [Tool 2] -> [Tool 3] -> Output Files
              |          |          |
          Translate    QA Check   Enforce Terms
```

Flows automatically:
- Read files matching `.bowrain/config.yaml` mappings
- Process through each tool in sequence
- Write results back to local files

## Built-In Flows

Bowrain CLI includes several built-in flows:

| Flow | Description |
|------|-------------|
| `ai-translate` | Translate with AI/LLM (Anthropic, OpenAI, Ollama) |
| `ai-translate-qa` | AI translation + quality checks |
| `pseudo-translate` | Generate pseudo-translations for UI testing |
| `qa-check` | Rule-based quality checks (whitespace, punctuation, placeholders) |
| `tm-leverage` | Pre-fill translations from translation memory |
| `segmentation` | Split source text into sentence segments |

### Running Built-In Flows

```bash
# List available flows
brain flow list

# Run a flow (project-based)
brain flow run ai-translate

# Standalone mode (without .bowrain/ project)
brain flow run ai-translate -i input.html -o output.html --source-lang en --target-lang fr
```

## Custom Flows

Create custom flows in `.bowrain/flows/` as YAML files.

### Example: Translation with QA

`.bowrain/flows/translate-with-qa.yaml`:

```yaml
name: translate-with-qa
description: AI translation with quality checks and terminology enforcement

steps:
  - tool: term-lookup
    config:
      termbase: .bowrain/termbase.tbx

  - tool: ai-translate
    config:
      provider: anthropic
      model: claude-sonnet-4.5
      temperature: 0.3

  - tool: term-enforce
    config:
      termbase: .bowrain/termbase.tbx
      required: true

  - tool: qa-check
    config:
      rules:
        - whitespace
        - punctuation
        - placeholders
        - terminology
```

Run with:

```bash
brain flow run translate-with-qa
```

## Flow Tools

Available tools for flows:

### Translation
- **ai-translate** — LLM-based translation
- **mt-translate** — Machine translation (DeepL, Google, Microsoft)
- **tm-leverage** — Translation memory matching
- **pseudo-translate** — Pseudo-localization

### Terminology
- **term-lookup** — Find terms in source text
- **term-enforce** — Validate required terminology

### Quality Assurance
- **qa-check** — Rule-based quality checks
- **ai-qa** — LLM-based quality review

### Utilities
- **segmentation** — Sentence segmentation
- **word-count** — Count words/characters
- **search-replace** — Find and replace patterns

## How Flows Work

1. **File Discovery**: Bowrain CLI reads files matching `.bowrain/config.yaml` mappings
2. **Parsing**: Each file is parsed into blocks (translatable units)
3. **Processing**: Blocks stream through tools in sequence
4. **Writing**: Results are written back to local files

### Streaming Pipeline

Flows use a streaming architecture for efficiency:

```
Read File -> Parse -> [Tool 1] -> [Tool 2] -> [Tool 3] -> Write
            |         |          |          |
         Channel   Channel    Channel    Channel
```

Benefits:
- **Low memory**: Blocks stream through tools, not loaded entirely
- **Parallelism**: Multiple tools can process different blocks concurrently
- **Cancellation**: Ctrl+C stops immediately (context cancellation)

## Next Steps

- [Create Custom Flows](/docs/bowrain-cli/flows/custom-flows)
- [Configure Hooks](/docs/bowrain-cli/flows/hooks)
- [Available Tools](/docs/features/formats)
- [Flow Command Reference](/docs/bowrain-cli/commands/flow)
