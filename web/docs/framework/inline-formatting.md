---
sidebar_position: 4
title: Inline Formatting
description: How neokapi preserves bold, italic, links, and other inline markup through the pipeline. Inline elements become typed inline-code runs so translations can reorder or omit them safely.
keywords: [inline formatting, runs, bold, italic, links, placeholders, inline codes, run sequence]
---

import { BlockPreview } from "@site/src/components/curated";

# Working with Inline Formatting

When documents are processed through the pipeline, neokapi preserves inline formatting like **bold**, _italic_, [links](https://example.com), and embedded values like variables and placeholders. The **Run** model does this: a block's content is a flat `[]Run` sequence, and each piece of inline markup becomes a typed inline-code run that means the same thing in every format.

> New to **Run**, **Block**, or **inline code**? Each term is defined once in the
> [Concepts](/framework/concepts); this page develops the inline-code model in full.

## Seeing inline codes in the content model

When kapi reads a file with inline markup, it moves the markup out of the text.
Each inline element becomes an **inline-code run** (a `PcOpen`/`PcClose` pair
for paired tags, a `Ph` for self-closing tokens) sitting in the run sequence
between text runs. Below, kapi parses an HTML page; in the parsed source text the
inline `<strong>` and `<a>` elements appear as inline-code runs, rendered here as
chips:

<BlockPreview
  sample="page.html"
  caption="kapi parsing HTML: inline elements become inline-code runs in the source text."
/>

Every source format produces this representation. The inline-code runs mark
_where_ inline formatting sits, and the original markup is held in the
run's `Data` field so the writer can reconstruct it exactly.

## How It Works

neokapi extracts inline formatting from any supported format and represents it as inline-code runs carrying semantic metadata. `<b>` (HTML), `**` (Markdown) and `<w:b/>` (DOCX) all produce a `PcOpen`/`PcClose` pair with the same semantic type (`fmt:bold`), so a tool processes them the same way.

Each inline-code run carries metadata that editors can use to guide translators:

- **Display text** (`Disp`): Human-readable labels (e.g., `[B]` for bold, `[/B]` for bold close)
- **Constraints** (`Constraints`): Whether a code is deletable, cloneable, or reorderable
- **Equivalent text** (`Equiv`): Plain text fallback (e.g., `\n` for line breaks)

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

## Editing text that carries inline codes

The constraints also govern what automated edits may do to a code when the
surrounding text changes. A tool that
rewrites a block's text (a find/replace such as [`ksed`](/toolbox/ksed), a
normalization or redaction transform) works on the text-only flattening of the
runs, in which inline codes have no width. It then re-anchors the codes that
survive. The rule, applied per code, is:

- A **paired code** (`PcOpen`/`PcClose`) is treated as a span over the text.
  After the edit its endpoints are re-anchored. If the span still covers text it
  is kept, balanced: editing a word inside a bold span keeps the span around the
  new word. If the edit empties the span, a **deletable** span (bold, italic, a
  link) is removed, so no empty `<b></b>` remains. A non-deletable span stays in
  place, empty, so a required code survives the edit.
- A **standalone code** (`Ph`, `Sub`) that sits in text the edit removes is
  dropped when **deletable** and kept (re-anchored to the edit boundary) when
  not, so a line break, a variable, or a subblock reference survives an edit
  that deletes the text around it.
- Codes outside the edited range are unaffected.

Edits and checks resolve each code's deletability from the same vocabulary, so
an edit and a translation follow one definition of which codes are required.
The framework primitive is `model.ApplyTextEdits`.

## Format-Independent Processing

### HTML Files

HTML inline elements (`<b>`, `<a href="...">`, `<br/>`) are extracted as inline-code runs: paired tags as `PcOpen`/`PcClose`, void elements as `Ph`. Block-level elements form Block boundaries.

**Example:**

> Click **here** to visit our _website_ for more information.

### Markdown Files

Markdown emphasis (`**`, `*`, backticks, `[]()`) maps to the same semantic types as HTML. `**bold**` and `<b>bold</b>` both resolve to `fmt:bold`.

**Example:**

> Run `kapi init` to set up your project. See the [documentation](https://docs.example.com) for details.

### JSON/YAML Message Catalogs

i18n variables like `{userName}` or `{count}` become `Ph` runs marked as non-deletable. They can be rearranged to match target language word order:

> Hello \{userName\}, you have \{count\} new messages.
> Bonjour \{userName\}, vous avez \{count\} nouveaux messages.

### XLIFF Exchange Files

XLIFF `<pc>` maps to a `PcOpen`/`PcClose` pair, `<ph>` to a `Ph` run, and `<sc>`/`<ec>` to a `PcOpen`/`PcClose` pair. XLIFF content therefore uses the same run model as every other format.

## Related

- [Vocabularies](/framework/vocabularies): the semantic types and constraints behind each code.
- [Content Model](/framework/content-model): where runs sit in a block.
- [ksed](/toolbox/ksed): find and replace that keeps inline codes.
