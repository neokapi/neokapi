---
sidebar_position: 11
title: AI Translation
description: neokapi integrates LLM-based translation, QA, and terminology extraction via a provider interface supporting Anthropic Claude, OpenAI, Google Gemini, Azure OpenAI, and Ollama for local models.
keywords: [AI translation, LLM, Anthropic Claude, OpenAI, Gemini, Ollama, ai-translate, localization AI]
---

# AI Translation

neokapi provides first-class LLM integration for translation, quality assurance, terminology extraction, and review.

## Supported Providers

| Provider          | Description                                             |
| ----------------- | ------------------------------------------------------- |
| **Anthropic**     | Claude models (recommended for quality)                 |
| **OpenAI**        | GPT models                                              |
| **Google Gemini** | Gemini models with streaming and live thinking progress |
| **Ollama**        | Local models (no API key needed)                        |

## Setup

Configure your AI provider in `kapi.yaml`:

```yaml
tools:
  ai-translation:
    provider: anthropic
    model: claude-sonnet-4-20250514
    apiKey: ${ANTHROPIC_API_KEY}
```

Or use environment variables:

```bash
export KAPI_TOOLS_AI_TRANSLATION_PROVIDER=anthropic
export KAPI_TOOLS_AI_TRANSLATION_MODEL=claude-sonnet-4-20250514
export ANTHROPIC_API_KEY=sk-...
```

### Google Gemini

```yaml
tools:
  ai-translation:
    provider: gemini
    model: gemini-3-flash-preview
    apiKey: ${GEMINI_API_KEY}
```

```bash
export KAPI_TOOLS_AI_TRANSLATION_PROVIDER=gemini
export KAPI_TOOLS_AI_TRANSLATION_MODEL=gemini-3-flash-preview
export GEMINI_API_KEY=...
```

Or via CLI flags: `--provider gemini --api-key $GEMINI_API_KEY`. The default model is `gemini-3-flash-preview`. Gemini supports streaming with live thinking progress, showing intermediate reasoning as translation proceeds.

## AI Tools

LLM-backed work is delivered as ordinary [tools](/framework/tools), so it
composes into [flows](/framework/flows) like any other stage:

| Tool             | Purpose                                               |
| ---------------- | ----------------------------------------------------- |
| `ai-translate`   | Translate untranslated Blocks using an LLM            |
| `ai-qa`          | Check translations for fluency, accuracy, terminology |
| `ai-terminology` | Extract terminology from source Blocks                |
| `ai-review`      | Review translations with explanations                 |

The generated [Tool Reference](/tools) lists each AI tool with its current
parameters.

## Usage

### Translate a file

```bash
kapi ai-translate -i input.html -o output.html --source-lang en --target-lang fr
```

### Translate and quality-check

```bash
kapi run ai-translate-qa -i input.html -o output.html --source-lang en --target-lang fr
```

## Prompt Engineering

Prompt templates in `ai/prompt/` are context-aware: they include surrounding Blocks, glossary constraints, TM matches, and format metadata. Templates are centralized for easy tuning.

## Environment Variables

| Variable            | Provider           |
| ------------------- | ------------------ |
| `ANTHROPIC_API_KEY` | Anthropic (Claude) |
| `OPENAI_API_KEY`    | OpenAI (GPT)       |
| `GEMINI_API_KEY`    | Google Gemini      |

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
