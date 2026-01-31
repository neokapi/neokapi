---
sidebar_position: 4
title: AI Translation
---

# AI Translation

gokapi provides first-class LLM integration for translation, quality assurance, terminology extraction, and review.

## Supported Providers

| Provider | Description |
|----------|-------------|
| **Anthropic** | Claude models (recommended for quality) |
| **OpenAI** | GPT models |
| **Ollama** | Local models (no API key needed) |

## Setup

Configure your AI provider in `gokapi.yaml`:

```yaml
tools:
  ai-translation:
    provider: anthropic
    model: claude-sonnet-4-20250514
    apiKey: ${ANTHROPIC_API_KEY}
```

Or use environment variables:

```bash
export GOKAPI_TOOLS_AI_TRANSLATION_PROVIDER=anthropic
export GOKAPI_TOOLS_AI_TRANSLATION_MODEL=claude-sonnet-4-20250514
export ANTHROPIC_API_KEY=sk-...
```

## AI Tools

| Tool | Purpose |
|------|---------|
| `ai-translate` | Translate untranslated Blocks using LLM |
| `ai-qa` | Check translations for fluency, accuracy, terminology |
| `ai-terminology` | Extract terminology from source Blocks |
| `ai-review` | Review translations with explanations |

## Usage

### Translate a file

```bash
kapi translate input.html -o output.html -s en -t fr --provider anthropic
```

### Use in a flow

AI tools compose naturally in flows with other tools:

```bash
kapi flow run --input docs/ --output out/ \
  --tools segmentation,tm-leverage,ai-translate,ai-qa \
  -s en -t fr
```

The ordering matters: TM leverage before AI translation avoids retranslating exact matches, reducing cost and improving consistency.

## Prompt Engineering

Prompt templates in `ai/prompt/` are context-aware: they include surrounding Blocks, glossary constraints, TM matches, and format metadata. Templates are centralized for easy tuning.

## Local Models with Ollama

For development and testing without API costs:

```bash
ollama pull llama3
```

```yaml
tools:
  ai-translation:
    provider: ollama
    model: llama3
    endpoint: http://localhost:11434
```
