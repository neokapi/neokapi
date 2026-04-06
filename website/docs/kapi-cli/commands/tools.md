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

Lists all processing tools. Each tool is available as a top-level kapi command (e.g., `kapi ai-translate`, `kapi pseudo-translate`, `kapi qa-check`). Tools can also be composed into multi-tool flows executed via `kapi run`.

## Example

```bash
kapi tools
```

## Available Tools

### Translation

| Tool               | Description                                                              |
| ------------------ | ------------------------------------------------------------------------ |
| `ai-translate`     | LLM-based translation (Anthropic, OpenAI, Ollama)                        |
| `mt-translate`     | Machine translation (DeepL, Google, Microsoft, ModernMT, MyMemory)       |
| `tm-leverage`      | Translation memory matching and pre-fill                                 |
| `diff-leverage`    | Preserve translations from previous document versions for unchanged text |
| `pseudo-translate` | Generate pseudo-translations for testing                                 |
| `create-target`    | Create target segments, optionally copying source text                   |
| `remove-target`    | Remove target segments for a locale or all locales                       |

### Terminology

| Tool           | Description                                          |
| -------------- | ---------------------------------------------------- |
| `term-lookup`  | Find terminology matches in source text              |
| `term-enforce` | Validate required terminology usage                  |
| `term-check`   | Term glossary checking with source-to-target mapping |

### Quality Assurance

| Tool                     | Description                                                           |
| ------------------------ | --------------------------------------------------------------------- |
| `qa-check`               | Rule-based quality checks (whitespace, punctuation, span constraints) |
| `ai-qa`                  | LLM-based quality review                                              |
| `ai-review`              | LLM-based translation review with explanations                        |
| `length-check`           | Verify character count, word count, and target/source length ratio    |
| `chars-check`            | Detect forbidden characters, mojibake corruption, control characters  |
| `pattern-check`          | Validate regex patterns in translations (e.g., printf placeholders)   |
| `inconsistency-check`    | Flag same source with different translations across documents         |
| `translation-comparison` | Compare translations across two target locales                        |
| `xml-validation`         | Validate XML structure in source and/or target text                   |

### Analysis & Reporting

| Tool                  | Description                                                                   |
| --------------------- | ----------------------------------------------------------------------------- |
| `word-count`          | Count words and characters per locale                                         |
| `char-count`          | Count characters (with/without spaces) per locale                             |
| `segment-count`       | Count source and target segments                                              |
| `repetition-analysis` | Track repeated segments with group keys and occurrence counts                 |
| `scoping-report`      | Classify blocks by match category (new, repetition, exact-match, fuzzy-match) |
| `chars-listing`       | List all unique characters and frequencies for font subsetting                |

### Text Processing

| Tool                  | Description                                                |
| --------------------- | ---------------------------------------------------------- |
| `search-replace`      | Find and replace patterns (regex or literal)               |
| `case-transform`      | Transform text to upper, lower, or title case              |
| `linebreak-convert`   | Normalize line endings (LF, CRLF, CR)                      |
| `bom-convert`         | Control Unicode BOM presence                               |
| `fullwidth-convert`   | Convert between half-width and full-width characters (CJK) |
| `uri-convert`         | Encode or decode URI escape sequences                      |
| `whitespace-correct`  | Normalize whitespace, remove zero-width characters         |
| `encoding-convert`    | Tag target encoding for downstream writers                 |
| `inline-codes-remove` | Strip inline markup to produce clean plain text            |
| `properties-set`      | Set key-value properties on blocks programmatically        |
| `external-command`    | Execute external CLI programs on block text                |

### Segmentation & Structure

| Tool              | Description                                       |
| ----------------- | ------------------------------------------------- |
| `segmentation`    | Split text into sentence segments                 |
| `xslt-transform`  | Regex-based tag transformation                    |
| `tag-protect`     | Protect tags matching patterns from modification  |
| `span-classify`   | Reclassify markup spans into semantic types       |
| `layer-processor` | Apply format-specific tool chains to child layers |

Each tool runs as a top-level command (e.g., `kapi pseudo-translate`, `kapi qa-check`). For composed multi-tool pipelines, see [kapi run](/docs/kapi-cli/commands/flow).
