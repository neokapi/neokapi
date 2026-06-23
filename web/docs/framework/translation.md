---
sidebar_position: 15
title: Translation
description: neokapi exposes translation as a single tool whose provider is either an LLM (Anthropic, OpenAI, Gemini, Azure OpenAI, Ollama) or a neural MT engine (DeepL, Google Translate, Microsoft Translator, ModernMT, MyMemory). Both routes share the same command, flags, and credential model.
keywords: [translation, LLM, MT, machine translation, AI translation, DeepL, Google Translate, Microsoft Translator, ModernMT, Anthropic, OpenAI, Gemini, Ollama, provider, localization]
---

# Translation

neokapi exposes translation through a single `translate` tool. One `--provider`
flag selects the backend, and the command, flags, and credential model are the
same whichever backend you choose. There are two provider families:

- **LLM providers** — Anthropic, OpenAI, Google Gemini, Azure OpenAI, Ollama.
  Context-aware, full prompt control, and (with Ollama) fully on-device.
- **Neural MT engines** — DeepL, Google Translate, Microsoft Translator,
  ModernMT, MyMemory. Fast, consistent, per-character bulk translation.

The generated [Tool reference](/reference/tools/translate) lists the current
parameters and default model for each provider.

:::tip Configuring a provider is a task, not a concept
Selecting a backend, supplying credentials, and setting a default are walked
step by step in the recipe
**[Choose a translation provider](/kapi/recipes/choose-a-translation-provider)** —
including on-device translation with Ollama. This page covers what translation
*is* and how it composes.
:::

## A single tool, two families

Because both families are values of `--provider` on the same `translate`
command, switching between them — or from an MT engine to an LLM — is a
configuration change only. Replace `provider: deepl` with `provider: anthropic`
and the rest of a flow is unchanged. The API key is never read from the recipe;
credentials are supplied out-of-band (see the recipe).

## Provider trade-offs

MT engines and LLM providers are both values of `--provider` on the one
`translate` command; the choice is a trade-off:

| Factor        | MT engines            | LLM providers        |
| ------------- | --------------------- | -------------------- |
| Speed         | Faster for bulk       | Slower per segment   |
| Cost          | Per-character pricing | Per-token pricing    |
| Quality       | Consistent            | Context-aware        |
| Customization | Limited               | Full prompt control  |
| Offline       | No                    | Yes (with Ollama)    |

Both approaches can be combined in a flow: an MT engine for bulk translation and
an LLM for quality review.

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

## Composing in flows

The `translate` tool composes into [flows](/framework/flows) like any other
stage. A production flow typically chains [TM leverage](/framework/translation-memory),
a translate pass, and a review step:

```yaml
steps:
  - tool: recycle
  - tool: translate
    config:
      provider: deepl
  - tool: review
  - tool: qa
```

Switching providers — `deepl` to `anthropic`, or vice versa — is a configuration
change; the surrounding steps are unchanged.

## Prompt engineering

Prompt templates in `core/ai/prompt/` are context-aware for LLM providers: they
include surrounding blocks, glossary constraints,
[TM matches](/framework/translation-memory), and format metadata. Templates are
centralized for tuning.
