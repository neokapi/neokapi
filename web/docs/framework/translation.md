---
sidebar_position: 15
title: Translation
description: neokapi exposes translation as a single tool whose provider is either an LLM (Anthropic, OpenAI, Gemini, Azure OpenAI, Ollama, Gemma) or a neural MT engine (DeepL, Google Translate, Microsoft Translator, ModernMT, MyMemory). Both routes share the same command, flags, and credential model.
keywords: [translation, LLM, MT, machine translation, AI translation, DeepL, Google Translate, Microsoft Translator, ModernMT, Anthropic, OpenAI, Gemini, Ollama, Gemma, provider, localization]
---

# Translation

neokapi exposes translation through a single `translate` tool. The `--provider`
flag selects the backend: an LLM provider (Anthropic, OpenAI, Gemini, Azure
OpenAI, Ollama, Gemma) routes to the AI translation path; a neural MT engine
(DeepL, Google Translate, Microsoft Translator, ModernMT, MyMemory) routes to
the machine-translation path. The command, flags, and credential model are the
same in both cases.

## Providers

### LLM providers

| Provider         | ID           | Notes                                                |
| ---------------- | ------------ | ---------------------------------------------------- |
| Anthropic        | `anthropic`  | Claude models; default when `--provider` is omitted  |
| OpenAI           | `openai`     | GPT models                                           |
| Google Gemini    | `gemini`     | Supports streaming with live thinking progress       |
| Azure OpenAI     | `azureopenai`| OpenAI models hosted on Azure                        |
| Ollama           | `ollama`     | Local models — no API key required                   |
| Gemma (local)    | `gemma`      | On-device Gemma 4 via the `kapi-llm` plugin — no API key, private |

### MT engines

| Engine                  | ID           | Notes                             |
| ----------------------- | ------------ | --------------------------------- |
| DeepL                   | `deepl`      | High-quality neural MT            |
| Google Translate        | `google`     | Google Cloud Translation API      |
| Microsoft Translator    | `microsoft`  | Azure Cognitive Services          |
| ModernMT                | `modernmt`   | Adaptive MT                       |
| MyMemory                | `mymemory`   | Works without an API key          |

The generated [Tool Reference](/tools) lists the current parameters for each provider.

## Setup

### Choosing a provider

Pass `--provider` on the command line. Each provider has a sensible default
model; override with `--model`:

```bash
# LLM provider
kapi translate -i input.html --source-lang en --target-lang fr \
  --provider anthropic --model claude-sonnet-4-20250514

# MT engine
kapi translate -i input.html --target-lang fr \
  --provider deepl
```

Or set `provider` (and optional non-secret options) as step config in a flow
definition. These are safe to commit:

```yaml
steps:
  - tool: translate
    config:
      provider: anthropic
      model: claude-sonnet-4-20250514
```

```yaml
steps:
  - tool: translate
    config:
      provider: microsoft
      region: westus2
```

When `--provider` is omitted it defaults to `anthropic`. To change the default
for LLM tools across all commands:

```bash
kapi config set ai.provider gemma
kapi config set ai.model gemma-4-e2b
```

A recipe `defaults` entry also works for project-scoped defaults:

```yaml
defaults:
  tools:
    translate:
      provider: gemma
```

An explicit `--provider` flag, inline config, or project recipe default always
overrides the stored `ai.provider` value.

### Supplying credentials

The API key is never read from the `.kapi`/`kapi.yaml` recipe — the recipe is
meant to be safe to commit. Provide the key in one of three ways (in priority
order — an inline `--api-key` wins, then a saved `--credential`, then the
environment variable):

1. **Saved credential** — store the key once in the OS keychain and reference it
   by name. This is the recommended approach for day-to-day use:

   ```bash
   kapi credentials add my-deepl --provider deepl --api-key …
   kapi translate -i input.html --target-lang fr \
     --provider deepl --credential my-deepl
   ```

2. **Inline flag** — pass the key directly with `--api-key` for a one-off run:

   ```bash
   kapi translate -i input.html --target-lang fr \
     --provider openai --api-key sk-…
   ```

3. **Per-provider environment variable** — when neither `--credential` nor
   `--api-key` is given, the standard environment variable for the resolved
   provider is used (see [Environment variables](#environment-variables)).

If no key is found by any of these means, the command reports a clear
"no credentials" error. Ollama, Gemma, and MyMemory require no API key.

### Environment variables

When no `--credential` and no `--api-key` are supplied, the key falls back to
the standard per-provider environment variable. Where two are listed, the first
non-empty one wins:

| Variable                                                        | Provider              |
| --------------------------------------------------------------- | --------------------- |
| `ANTHROPIC_API_KEY`                                             | Anthropic             |
| `OPENAI_API_KEY`                                                | OpenAI                |
| `GEMINI_API_KEY` (then `GOOGLE_API_KEY`)                        | Google Gemini         |
| `AZURE_OPENAI_API_KEY`                                          | Azure OpenAI          |
| `DEEPL_API_KEY`                                                 | DeepL                 |
| `GOOGLE_TRANSLATE_API_KEY` (then `GOOGLE_API_KEY`)              | Google Translate      |
| `MICROSOFT_TRANSLATOR_KEY` (then `AZURE_TRANSLATOR_KEY`)        | Microsoft Translator  |
| `MODERNMT_API_KEY`                                              | ModernMT              |
| `MYMEMORY_API_KEY`                                              | MyMemory (optional)   |

Ollama and Gemma run local models and require no key. For the full set of
per-provider parameters, see the generated [Tool Reference](/tools).

## LLM providers in depth

### Google Gemini

Gemini supports streaming with live thinking progress, showing intermediate
reasoning as translation proceeds:

```bash
export GEMINI_API_KEY=…
kapi translate -i input.html --target-lang fr --provider gemini
```

The default model is `gemini-3-flash-preview`.

### Local models: Ollama

For development and testing without API costs, run a local model through Ollama.
No API key is required:

```bash
ollama pull llama3
kapi translate -i input.html --target-lang fr --provider ollama --model llama3
```

As flow-step config:

```yaml
steps:
  - tool: translate
    config:
      provider: ollama
      model: llama3
```

### Local models: Gemma

The `gemma` provider runs Google's Gemma 4 on-device, in-process, with no API
key and nothing sent to a server. It is delivered by the `kapi-llm` plugin (the
heavy ONNX runtime stays in the plugin, not the `kapi` binary):

```bash
kapi plugins install llm          # or: brew install neokapi/tap/kapi-llm
kapi translate -i input.html --target-lang fr --provider gemma
```

The model downloads on demand on first use and is cached. The same `gemma`
provider works across the AI tools (`qa`, `brand-voice-check`, …) and in the
browser — see the [Core Framework lab](/lab), which runs the same model via
WebGPU.

## MT engines in depth

MT providers accept additional non-secret step config alongside the shared
`provider`/`apiKey`:

```yaml
steps:
  - tool: translate
    config:
      provider: microsoft
      region: westus2
```

Switching between MT engines — or from an MT engine to an LLM provider — is a
configuration change only. Replace `provider: deepl` with `provider: anthropic`
and the rest of the flow is unchanged.

## Related AI tools

Translation composes with other LLM-backed tools in the same [flow](/framework/flows):

| Tool           | Purpose                                                               |
| -------------- | --------------------------------------------------------------------- |
| `translate`    | Translate untranslated blocks (LLM or MT provider)                    |
| `qa`           | LLM-judged quality check (fluency, accuracy, terminology)             |
| `review`       | Detailed translation review with explanations                         |
| `term-extract` | Extract candidate terminology from source blocks                      |

The `qa` tool runs deterministic rule-based checks without `--provider`, and
switches to LLM-judged review when a provider is given. See
[QA checks](/framework/checks/qa-checks) for the full check catalogue.

## Usage

### Translate a file

```bash
kapi translate -i input.html -o output.html --source-lang en --target-lang fr
```

### Translate with an MT engine

```bash
kapi translate -i messages.json --target-lang de --provider deepl
```

### Translate then quality-check

```bash
kapi run translate-qa -i input.html -o output.html --source-lang en --target-lang fr
```

## Composing in flows

The `translate` tool composes into [flows](/framework/flows) like any other
stage. A production flow typically chains TM leverage, a translate pass, and a
review step:

```yaml
steps:
  - tool: recycle
  - tool: translate
    config:
      provider: deepl
  - tool: review
  - tool: qa
```

Switching providers — `deepl` to `anthropic`, or vice versa — is a
configuration change; the surrounding steps are unchanged.

## Provider trade-offs

MT engines and LLM providers are both values of `--provider` on the one
`translate` command; the choice is a trade-off:

| Factor        | MT engines            | LLM providers        |
| ------------- | --------------------- | -------------------- |
| Speed         | Faster for bulk       | Slower per segment   |
| Cost          | Per-character pricing | Per-token pricing    |
| Quality       | Consistent            | Context-aware        |
| Customization | Limited               | Full prompt control  |
| Offline       | No                    | Yes (with Ollama/Gemma) |

Both approaches can be combined in a flow: use an MT engine for bulk translation
and an LLM for quality review.

## Prompt engineering

Prompt templates in `core/ai/prompt/` are context-aware for LLM providers: they
include surrounding blocks, glossary constraints, [TM matches](/framework/translation-memory),
and format metadata. Templates are centralized for tuning.
