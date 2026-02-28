---
sidebar_position: 3
title: tools
---

# kapi tools

List available processing tools.

## Synopsis

```bash
kapi tools
```

## Description

Lists all processing tools available for use in flows. Tools are the building blocks of translation pipelines.

## Example

```bash
kapi tools
```

## Available Tools

### Translation
| Tool | Description |
|------|-------------|
| `ai-translate` | LLM-based translation (Anthropic, OpenAI, Ollama) |
| `mt-translate` | Machine translation (DeepL, Google, Microsoft, ModernMT, MyMemory) |
| `tm-leverage` | Translation memory matching and pre-fill |
| `pseudo-translate` | Generate pseudo-translations for testing |

### Terminology
| Tool | Description |
|------|-------------|
| `term-lookup` | Find terminology matches in source text |
| `term-enforce` | Validate required terminology usage |

### Quality Assurance
| Tool | Description |
|------|-------------|
| `qa-check` | Rule-based quality checks (whitespace, punctuation, placeholders) |
| `ai-qa` | LLM-based quality review |

### Utilities
| Tool | Description |
|------|-------------|
| `segmentation` | Split text into sentence segments |
| `word-count` | Count words and characters |
| `search-replace` | Find and replace patterns |

Tools are used in flows via `kapi flow run`. See [flow command](/docs/kapi-cli/commands/flow) for usage.
