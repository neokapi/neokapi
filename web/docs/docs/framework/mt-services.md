---
sidebar_position: 12
title: Machine Translation Services
description: neokapi integrates neural machine translation services — DeepL, Google Translate, Microsoft Translator, ModernMT, and MyMemory — as pipeline tools that plug in alongside TM leverage and AI translation.
keywords: [machine translation, MT, DeepL, Google Translate, Microsoft Translator, ModernMT, MyMemory]
---

# Machine Translation Services

neokapi integrates with external machine translation (MT) services as an alternative to LLM-based translation.

## Supported Services

| Service                  | Description                             |
| ------------------------ | --------------------------------------- |
| **DeepL**                | High-quality neural machine translation |
| **Google Translate**     | Google Cloud Translation API            |
| **Microsoft Translator** | Azure Cognitive Services                |
| **MyMemory**             | Free translation memory API             |
| **ModernMT**             | Adaptive machine translation            |

## Configuration

Each MT service is exposed as its own `<provider>-translate` tool, so the
provider is fixed by the tool you run — `deepl-translate` uses DeepL,
`google-translate` uses Google, and so on. Non-secret options (region, project,
formality) are set as step config in a flow definition, which is safe to commit:

```yaml
steps:
  - tool: microsoft-translate
    config:
      region: westus2
```

The API key is never read from the `.kapi`/`kapi.yaml` recipe. Supply it the
same way as for [AI translation](/framework/ai-translation#supplying-the-api-key)
(if more than one is present, an inline `--api-key` wins, then a saved
`--credential`, then the environment variable):

1. **Saved credential** — store the key once and reference it by name:

   ```bash
   kapi credentials add my-deepl --provider deepl --api-key …
   kapi deepl-translate -i input.html --target-lang fr --credential my-deepl
   ```

2. **Inline flag** — `--api-key …` for a one-off run.

3. **Per-provider environment variable** — used when neither `--credential` nor
   `--api-key` is given (see [Environment variables](#environment-variables)).

### Environment variables

When no `--credential` and no `--api-key` are supplied, the key falls back to
the standard environment variable for the tool's provider. Where two are listed,
the first non-empty one wins:

| Variable                                              | Provider              |
| ----------------------------------------------------- | --------------------- |
| `DEEPL_API_KEY`                                       | DeepL                 |
| `GOOGLE_TRANSLATE_API_KEY` (then `GOOGLE_API_KEY`)    | Google Translate      |
| `MICROSOFT_TRANSLATOR_KEY` (then `AZURE_TRANSLATOR_KEY`) | Microsoft Translator |
| `MODERNMT_API_KEY`                                    | ModernMT              |
| `MYMEMORY_API_KEY`                                    | MyMemory (optional)   |

MyMemory works without a key. For the full set of per-provider parameters, see
the generated [Tool Reference](/tools).

## Usage

Each MT service is exposed as its own [tool](/framework/tools), named
`<provider>-translate` — `deepl-translate`, `google-translate`,
`microsoft-translate`, `modernmt-translate`, `mymemory-translate`. They compose
into [flows](/framework/flows) like any other stage. A typical production flow
chains TM leverage, an MT pass, and AI refinement:

```yaml
steps:
  - tool: tm-leverage
  - tool: deepl-translate
  - tool: ai-review
  - tool: qa-check
```

Switching providers is a configuration change — replace `deepl-translate` with
`google-translate` (or `ai-translate`) and the rest of the flow is unchanged.

## Comparison with AI Translation

| Feature       | MT Services           | AI Translation      |
| ------------- | --------------------- | ------------------- |
| Speed         | Faster for bulk       | Slower per segment  |
| Cost          | Per-character pricing | Per-token pricing   |
| Quality       | Consistent            | Context-aware       |
| Customization | Limited               | Full prompt control |
| Offline       | No                    | Yes (with Ollama)   |

Both approaches can be combined in a flow: use MT services for bulk translation and AI for quality review.
