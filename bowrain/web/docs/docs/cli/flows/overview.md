---
sidebar_position: 1
title: Overview
---

# Translation Flows

Flows are composable pipelines that process localization files through a sequence of tools. With 41+ format readers and 46+ processing tools, flows can handle everything from AI translation to source QA to terminology enforcement.

## What Are Flows?

A flow is a multi-step processing pipeline where each step transforms the content:

```
Input Files -> [Tool 1] -> [Tool 2] -> [Tool 3] -> Output Files
              |          |          |
          Translate    QA Check   Enforce Terms
```

Flows automatically:

- Read files matching the recipe's `content:` collections
- Process through each tool in sequence
- Write results back to local files

## Built-In Flows

kapi includes several built-in flows:

| Flow               | Description                                                       |
| ------------------ | ----------------------------------------------------------------- |
| `ai-translate`     | Translate with AI/LLM (Anthropic, OpenAI, Ollama)                 |
| `ai-translate-qa`  | AI translation + quality checks                                   |
| `pseudo-translate` | Generate pseudo-translations for UI testing                       |
| `qa-check`         | Rule-based quality checks (whitespace, punctuation, placeholders) |
| `tm-leverage`      | Pre-fill translations from translation memory                     |
| `segmentation`     | Split source text into sentence segments                          |

### Running Built-In Tools and Flows

```bash
# List available tools and flows
kapi tools
kapi flows

# Run a tool directly (top-level command)
kapi ai-translate

# Standalone mode (without a .kapi project)
kapi ai-translate -i input.html -o output.html --source-lang en --target-lang fr

# Run a composed flow
kapi run ai-translate-qa -i input.html -o output.html --source-lang en --target-lang fr
```

## Custom Flows

Create custom flows in `.kapi/flows/` as YAML files.

### Example: Translation with QA

`.kapi/flows/translate-with-qa.yaml`:

```yaml
name: translate-with-qa
description: AI translation with quality checks and terminology enforcement

steps:
  - tool: term-lookup
    config:
      termbase: .kapi/termbase.db

  - tool: ai-translate
    config:
      provider: anthropic
      model: claude-sonnet-4.5
      temperature: 0.3

  - tool: term-enforce
    config:
      termbase: .kapi/termbase.db
      required: true

  - tool: qa-check
    config:
      rules:
        - whitespace
        - punctuation
        - placeholders
        - terminology
```

Run with:

```bash
kapi run translate-with-qa
```

## Tool Catalog (46+ Tools)

All tools can be used as flow steps. They are organized into the following categories.

### Content Analysis

| Tool                     | Description                                                                   |
| ------------------------ | ----------------------------------------------------------------------------- |
| `word-count`             | Count words in source and target content                                      |
| `char-count`             | Count characters in source and target content                                 |
| `segment-count`          | Count translatable segments                                                   |
| `scoping-report`         | Generate a detailed scoping report (word counts, repetitions, file breakdown) |
| `repetition-analysis`    | Identify repeated segments across files for TM leverage                       |
| `chars-listing`          | List all distinct characters used in source and/or target                     |
| `translation-comparison` | Compare translations across locales or versions                               |

### Translation

| Tool               | Description                                                                     |
| ------------------ | ------------------------------------------------------------------------------- |
| `ai-translate`     | LLM-based translation (Anthropic, OpenAI, Ollama)                               |
| `mt-translate`     | Machine translation (DeepL, Google, Microsoft, ModernMT, MyMemory)              |
| `tm-leverage`      | Pre-fill from translation memory with fuzzy matching                            |
| `diff-leverage`    | Leverage translations from a previous version using diff analysis               |
| `pseudo-translate` | Generate pseudo-translations for UI testing and internationalization validation |
| `create-target`    | Create target segments from source (copy source to target)                      |
| `remove-target`    | Remove target segments                                                          |

### Terminology

| Tool             | Description                                                |
| ---------------- | ---------------------------------------------------------- |
| `term-lookup`    | Find terms in source text and annotate blocks              |
| `term-enforce`   | Validate that required terminology is used in translations |
| `term-check`     | Check terminology consistency across content               |
| `ai-terminology` | Extract terminology using AI/LLM                           |

### Quality Assurance

| Tool                  | Description                                                               |
| --------------------- | ------------------------------------------------------------------------- |
| `qa-check`            | Rule-based quality checks (whitespace, punctuation, placeholders, length) |
| `ai-qa`               | LLM-based quality review and error detection                              |
| `ai-review`           | LLM-based translation review with scoring                                 |
| `xml-validation`      | Validate XML/HTML structure in source and target                          |
| `inconsistency-check` | Detect inconsistent translations of identical source strings              |
| `length-check`        | Validate string length against configured limits                          |
| `chars-check`         | Check for invalid or unexpected Unicode characters                        |
| `pattern-check`       | Validate content against custom regex patterns                            |

### Text Processing

| Tool                 | Description                                                             |
| -------------------- | ----------------------------------------------------------------------- |
| `search-replace`     | Find and replace patterns (literal or regex)                            |
| `case-transform`     | Transform text case (upper, lower, title)                               |
| `linebreak-convert`  | Normalize line endings (LF, CRLF, CR)                                   |
| `whitespace-correct` | Normalize spaces, match source whitespace, remove zero-width characters |
| `fullwidth-convert`  | Convert between fullwidth and halfwidth characters (CJK)                |
| `uri-convert`        | Encode or decode URI components                                         |
| `bom-convert`        | Add or remove byte order marks                                          |
| `segmentation`       | Split text into sentence-level segments                                 |

### Inline Formatting

| Tool                  | Description                                              |
| --------------------- | -------------------------------------------------------- |
| `span-classify`       | Classify inline spans by type (bold, italic, link, etc.) |
| `tag-protect`         | Protect inline tags from modification during translation |
| `inline-codes-remove` | Remove inline formatting codes from content              |
| `layer-processor`     | Process embedded content layers (e.g., HTML inside JSON) |

### Encoding and Format

| Tool               | Description                               |
| ------------------ | ----------------------------------------- |
| `encoding-detect`  | Detect character encoding of source files |
| `encoding-convert` | Convert between character encodings       |
| `xslt-transform`   | Apply XSLT transformations to XML content |

### Metadata and Properties

| Tool             | Description                                             |
| ---------------- | ------------------------------------------------------- |
| `properties-set` | Set or update block properties (state, notes, metadata) |

### External Integration

| Tool               | Description                                             |
| ------------------ | ------------------------------------------------------- |
| `external-command` | Run an external command on block content (stdin/stdout) |

## Supported Formats (41+)

Flows can process files in any of the 41+ supported formats:

| Category               | Formats                                                                                                               |
| ---------------------- | --------------------------------------------------------------------------------------------------------------------- |
| **Localization**       | XLIFF 1.2, XLIFF 2.0, PO (GNU gettext), Qt TS, Java Properties, TMX                                                   |
| **Structured Data**    | JSON, YAML, CSV, TSV, XML, DTD, ICU MessageFormat                                                                     |
| **Documents**          | HTML, Markdown, OpenXML (DOCX/PPTX/XLSX), ODF, RTF, PDF, TeX/LaTeX, EPUB                                              |
| **Desktop Publishing** | InDesign IDML, InCopy ICML, FrameMaker MIF                                                                            |
| **Subtitles**          | SRT, TTML, WebVTT                                                                                                     |
| **Wiki**               | MediaWiki/DokuWiki                                                                                                    |
| **CAT Tools**          | Trados TTX, Trados TXML, Translation Table                                                                            |
| **Specialized**        | Regex, Doxygen, PHP Content, Moses Text, Fixed-Width, Paragraph Plain Text, Spliced Lines, Versified Text, R Vignette |
| **Archives**           | ZIP Archive                                                                                                           |
| **Plain Text**         | Plain Text                                                                                                            |

## How Flows Work

1. **File Discovery**: kapi reads files matching the recipe's `content:` collections
2. **Parsing**: Each file is parsed into blocks (translatable units)
3. **Processing**: Blocks stream through tools in sequence
4. **Writing**: Results are written back to local files

### Streaming Pipeline

Flows use a streaming architecture for efficiency:

```
Read File -> Parse -> [Tool 1] -> [Tool 2] -> [Tool 3] -> Write
            |         |          |          |
         Channel   Channel    Channel    Channel
```

Benefits:

- **Low memory**: Blocks stream through tools, not loaded entirely
- **Parallelism**: Multiple tools can process different blocks concurrently
- **Cancellation**: Ctrl+C stops immediately (context cancellation)

## Next Steps

- [Create Custom Flows](/cli/flows/custom-flows)
- [Configure Hooks](/cli/flows/hooks)
- [Available Formats](https://neokapi.github.io/web/neokapi/docs/features/formats)
- [Run Command Reference](/cli/commands/run)
- [Server-Side Flows](/server/flows)
