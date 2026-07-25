---
sidebar_position: 15
title: Translation
description: neokapi exposes translation as a single LLM-backed tool — Anthropic, OpenAI, Gemini, Azure OpenAI, or on-device Ollama — selected with one --provider flag under a shared command, flag, and credential model. Plugins can add machine-translation engines.
keywords: [translation, LLM, AI translation, Anthropic, OpenAI, Gemini, Ollama, provider, machine translation, multilingual content]
---

# Translation

neokapi exposes translation through a single `translate` tool. One `--provider`
flag selects the backend, and the command, flags, and credential model are the
same whichever backend you choose:

- **LLM providers** — Anthropic, OpenAI, Google Gemini, Azure OpenAI, Ollama.
  Context-aware, full prompt control, and (with Ollama) fully on-device.
- **The offline demo provider** — keyless, deterministic, clearly-marked
  illustrative output for trying flows without credentials.
- **Plugin-hosted MT engines** — classic machine-translation engines (DeepL,
  Google Translate, Microsoft Translator, and the like) are not built in; a
  plugin can register one and it appears under the same `--provider` flag.

The generated [Tool reference](/reference/tools/translate) lists the current
parameters and default model for each provider.

:::tip Configuring a provider is a task, not a concept
Selecting a model, supplying credentials, and setting a default are walked
step by step in the recipe
**[Choose a translation model](/kapi/recipes/choose-a-translation-provider)** —
including on-device translation with Ollama. This page covers what translation
*is* and how it composes.
:::

## A single tool

Because every backend is a value of `--provider` on the same `translate`
command, switching between them is a configuration change only. Replace
`provider: anthropic` with `provider: ollama` and the rest of a flow is
unchanged. The API key is never read from the recipe; credentials are supplied
out-of-band (see the recipe).

## Related AI tools

Translation composes with other LLM-backed tools in the same [flow](/framework/flows):

| Tool           | Purpose                                                               |
| -------------- | --------------------------------------------------------------------- |
| `translate`    | Translate untranslated blocks with the selected provider              |
| `qa`           | LLM-judged quality check (fluency, accuracy, terminology)             |
| `review`       | Detailed translation review with explanations                         |
| `term-extract` | Extract candidate terminology from source blocks                      |

The `qa` tool runs deterministic rule-based checks without `--provider`, and
switches to LLM-judged review when a provider is given. See
[QA checks](/framework/checks/qa-checks) for the full check catalogue.

## Composing in flows

The `translate` tool composes into [flows](/framework/flows) like any other
stage. A production flow typically chains
[memory leverage](/framework/content-memory), a translate pass, and a review step:

```yaml
steps:
  - tool: recycle
  - tool: translate
    config:
      provider: anthropic
  - tool: review
  - tool: qa
```

Switching providers — `anthropic` to `ollama`, or vice versa — is a
configuration change; the surrounding steps are unchanged.

## Prompts

Every prompt kapi sends is built in `core/ai/prompt/`, composed from framework
rules (return only the translation; preserve placeholders and inline tags) plus
the steering your project declares — an instruction, a
[brand voice profile](/framework/checks/brand-voice), and a
[terms store](/framework/terminology). The prompt is the same for every provider;
only the transport differs.

You do not have to take that on trust: `--explain-prompts` prints the exact text
sent to the model, attributed section by section. See [Prompts](/framework/prompts).

[Memory matches](/framework/content-memory) are *not* part of the prompt —
recycling is a separate deterministic step that runs before translation.
