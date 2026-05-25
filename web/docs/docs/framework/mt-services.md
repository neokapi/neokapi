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

Configure MT providers in `kapi.yaml`:

```yaml
mt:
  deepl:
    apiKey: ${DEEPL_API_KEY}
    formality: more # less, more, default
  google:
    apiKey: ${GOOGLE_TRANSLATE_API_KEY}
    project: my-project
  microsoft:
    apiKey: ${AZURE_TRANSLATOR_KEY}
    region: westus2
```

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
