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
| **Azure OpenAI**  | OpenAI models hosted on Azure                           |
| **Google Gemini** | Gemini models with streaming and live thinking progress |
| **Ollama**        | Local models (no API key needed)                        |

## Setup

A provider and model are chosen per run; an API key is supplied separately. The
provider and model carry safe defaults and may be set on the command line or in
a flow step, while the key — a secret — is kept out of the committable recipe.

### Choosing a provider and model

Pass `--provider` and `--model` on the command line:

```bash
kapi ai-translate -i input.html --source-lang en --target-lang fr \
  --provider anthropic --model claude-sonnet-4-20250514
```

Or set them as step config in a flow definition (these are non-secret defaults,
safe to commit):

```yaml
steps:
  - tool: ai-translate
    config:
      provider: anthropic
      model: claude-sonnet-4-20250514
```

When `--provider` is omitted, `ai-translate` defaults to Anthropic. Each
provider ships a sensible default model.

### Supplying the API key

The API key is never read from the `.kapi`/`kapi.yaml` recipe — the recipe is
meant to be safe to commit. Provide the key in one of three ways (if more than
one is present, an inline `--api-key` wins, then a saved `--credential`, then
the environment variable):

1. **Saved credential** — store the key once in the OS keychain and reference it
   by name. This is the recommended approach for day-to-day use:

   ```bash
   kapi credentials add my-anthropic --provider anthropic --api-key sk-…
   kapi ai-translate -i input.html --target-lang fr --credential my-anthropic
   ```

2. **Inline flag** — pass the key directly with `--api-key` (useful for one-off
   runs):

   ```bash
   kapi ai-translate -i input.html --target-lang fr \
     --provider openai --api-key sk-…
   ```

3. **Per-provider environment variable** — when neither `--credential` nor
   `--api-key` is given, the standard environment variable for the resolved
   provider is used (see [Environment variables](#environment-variables)):

   ```bash
   export ANTHROPIC_API_KEY=sk-…
   kapi ai-translate -i input.html --target-lang fr --provider anthropic
   ```

If no key is found by any of these means, the command reports a clear
"no credentials" error.

### Google Gemini

Gemini supports streaming with live thinking progress, showing intermediate
reasoning as translation proceeds. Select it like any other provider:

```bash
export GEMINI_API_KEY=…
kapi ai-translate -i input.html --target-lang fr --provider gemini
```

The default model is `gemini-3-flash-preview`.

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

Prompt templates in `core/ai/prompt/` are context-aware: they include surrounding Blocks, glossary constraints, TM matches, and format metadata. Templates are centralized for easy tuning.

## Environment variables

When no `--credential` and no `--api-key` are supplied, the API key falls back
to the standard per-provider environment variable for the resolved provider.
The variables follow each provider's own conventions:

| Variable                                | Provider          |
| --------------------------------------- | ----------------- |
| `ANTHROPIC_API_KEY`                     | Anthropic         |
| `OPENAI_API_KEY`                        | OpenAI            |
| `GEMINI_API_KEY` (then `GOOGLE_API_KEY`) | Google Gemini     |
| `AZURE_OPENAI_API_KEY`                  | Azure OpenAI      |

Ollama runs local models and requires no key. The
[machine translation services](/framework/mt-services#environment-variables)
page lists the equivalent variables for the MT providers. For the full set of
provider parameters, see the generated [Tool Reference](/tools).

## Local models with Ollama

For development and testing without API costs, run a local model through Ollama.
No API key is required:

```bash
ollama pull llama3
kapi ai-translate -i input.html --target-lang fr --provider ollama --model llama3
```

The same selection works as flow-step config:

```yaml
steps:
  - tool: ai-translate
    config:
      provider: ollama
      model: llama3
```
