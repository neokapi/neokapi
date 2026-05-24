---
sidebar_position: 3
title: Inline Formatting
description: How neokapi preserves bold, italic, links, and other inline markup through the pipeline. Inline elements become typed Spans so translations can reorder or omit them safely.
keywords: [inline formatting, spans, bold, italic, links, placeholders, fragment, coded text]
---

import { BlockPreview } from "@site/src/components/curated";

# Working with Inline Formatting

When documents are processed through the pipeline, neokapi preserves inline formatting like **bold**, _italic_, [links](https://example.com), and embedded values like variables and placeholders. This is handled through the Fragment and Span model, which normalizes format-specific markup into a format-independent representation.

## Seeing inline codes in the content model

When kapi reads a file with inline markup, the markup does not stay in the text.
Each inline element is extracted into a **span** and replaced in the text by a
positional marker. Below, kapi parses an HTML page; in the parsed source text
the inline `<strong>` and `<a>` elements appear as span markers (rendered here as
chips) rather than as literal tags:

<BlockPreview
  sample="page.html"
  caption="kapi parsing HTML — inline elements become span markers in the source text."
/>

This is the same representation regardless of the source format: the markers
mark _where_ inline formatting sits, while the original markup is held alongside
in the span so the writer can reconstruct it exactly.

## How It Works

neokapi extracts inline formatting from any supported format and represents it using coded text with Span metadata. This means `<b>` (HTML), `**` (Markdown), and `<w:b/>` (DOCX) all produce the same semantic type (`fmt:bold`), enabling format-independent processing.

The Span model carries metadata that editors can use to guide translators:

- **Display text**: Human-readable labels (e.g., `[B]` for bold, `[/B]` for bold close)
- **Constraints**: Whether a tag is deletable, cloneable, or reorderable
- **Equivalent text**: Plain text fallback (e.g., `\n` for line breaks)

See [Vocabularies](/framework/vocabularies) for the full semantic type system.

## Constraint Rules

Each inline code carries constraint metadata that defines what translators may do with it:

**Flexible tags** (like bold, italic, underline):

- Deletable: can be removed if the target language doesn't need them
- Cloneable: can be duplicated to apply formatting to more text
- Reorderable: can be rearranged in the sentence

**Required elements** (like line breaks, variables, placeholders):

- Non-deletable: must appear in the translation
- Non-cloneable: cannot be duplicated
- Often non-reorderable: position relative to other codes is fixed

**Variables and placeholders** (like `{userName}` or `{count}`):

- Non-deletable and non-cloneable: must be preserved exactly
- Reorderable: can be moved to match target language grammar

Editors can use these constraints to prevent invalid changes and provide real-time validation (e.g., warning about missing required tags or duplicate non-cloneable tags).

## Format-Independent Processing

### HTML Files

HTML inline elements (`<b>`, `<a href="...">`, `<br/>`) are extracted as Spans. Block-level elements form Block boundaries.

**Example:**

> Click **here** to visit our _website_ for more information.

### Markdown Files

Markdown emphasis (`**`, `*`, backticks, `[]()`) maps to the same semantic types as HTML. `**bold**` and `<b>bold</b>` both resolve to `fmt:bold`.

**Example:**

> Run `kapi init` to set up your project. See the [documentation](https://docs.example.com) for details.

### JSON/YAML Localization Files

i18n variables like `{userName}` or `{count}` become placeholder Spans marked as non-deletable. They can be rearranged to match target language word order:

> Hello \{userName\}, you have \{count\} new messages.
> Bonjour \{userName\}, vous avez \{count\} nouveaux messages.

### XLIFF Exchange Files

XLIFF `<pc>`, `<ph>`, and `<sc>`/`<ec>` elements map to the same Span model, enabling consistent processing regardless of exchange format.
