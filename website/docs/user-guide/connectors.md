---
sidebar_position: 5
title: Connectors
---

# Translation Connectors

gokapi integrates with external translation services as an alternative to LLM-based translation.

## Supported Services

| Service | Description |
|---------|-------------|
| **DeepL** | High-quality neural machine translation |
| **Google Translate** | Google Cloud Translation API |
| **Microsoft Translator** | Azure Cognitive Services |
| **MyMemory** | Free translation memory API |
| **ModernMT** | Adaptive machine translation |

## Configuration

Configure connectors in `gokapi.yaml`:

```yaml
connectors:
  deepl:
    apiKey: ${DEEPL_API_KEY}
    formality: more    # less, more, default
  google:
    apiKey: ${GOOGLE_TRANSLATE_API_KEY}
    project: my-project
  microsoft:
    apiKey: ${AZURE_TRANSLATOR_KEY}
    region: westus2
```

## Usage

```bash
# Translate using DeepL
kapi translate input.html -o output.html -s en -t de --connector deepl

# Translate using Google
kapi translate input.json -o output.json -s en -t ja --connector google
```

## Comparison with AI Translation

| Feature | Connectors | AI Translation |
|---------|-----------|----------------|
| Speed | Faster for bulk | Slower per segment |
| Cost | Per-character pricing | Per-token pricing |
| Quality | Consistent | Context-aware |
| Customization | Limited | Full prompt control |
| Offline | No | Yes (with Ollama) |

Both approaches can be combined in a flow: use connectors for bulk translation and AI for quality review.
