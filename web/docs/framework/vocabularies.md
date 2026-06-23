---
sidebar_position: 5
title: Vocabularies
description: Vocabularies are the semantic type system that classifies inline codes — mapping format-specific markup like HTML bold, Markdown emphasis, and DOCX bold to a common set of types so every tool treats them identically.
keywords: [vocabularies, semantic types, inline codes, spans, fmt:bold, localization, format-independent]
---

# Vocabularies

A **vocabulary** is the semantic type system that gives meaning to inline codes.
When a [reader](/framework/formats) lifts an inline element out of the text into a
[span](/framework/inline-formatting), it assigns that span a semantic type from a
vocabulary — `fmt:bold`, `link:hyperlink`, `code:variable`, and so on. The
vocabulary entry says what the type means, how it should be rendered and labeled,
and what a translator is allowed to do with it. This is the layer that makes
inline handling format-independent: `<b>` (HTML), `**` (Markdown), and `<w:b/>`
(DOCX) all resolve to the same `fmt:bold` type, so everything downstream treats
them identically. (See the [Glossary](/framework/glossary) for **Run**, **span**,
and **semantic type** in one place.)

## What a semantic type carries

Each type maps a span to a consistent set of metadata:

| Layer               | What it provides    | Example                             |
| ------------------- | ------------------- | ----------------------------------- |
| **Category**        | Logical grouping    | `formatting`, `code`, `structure`   |
| **Label**           | Human-readable name | `Bold`, `Variable`                  |
| **HTML rendering**  | Preview output      | `<b>`, `</b>`                       |
| **Display text**    | Editor chip label   | `[B]`, `[/B]`                       |
| **Color scheme**    | Visual styling      | Blue for bold, orange for variables |
| **Constraints**     | Editing rules       | Deletable, cloneable, reorderable   |
| **Text equivalent** | Plain text fallback | `\n` for line breaks                |

The **constraints** are the part that matters most for correctness. They encode
what a translator may do with a code:

- **Deletable** — may the code be removed? Formatting like bold is deletable;
  required elements like line breaks, variables, and placeholders are not.
- **Cloneable** — may the code be duplicated? Bold can be applied to more text;
  a variable must not be repeated.
- **Reorderable** — may the code move relative to others? A variable can move to
  match target word order; a fixed structural code may not.

Editors and [QA checks](/framework/checks/qa-checks) read these constraints to prevent
invalid changes — blocking deletion of a required tag, flagging a duplicated
variable, or warning about a missing code — without knowing anything about the
source file format.

## Layered vocabularies

Vocabularies are layered: a vocabulary can `extend` another, inheriting its
types and adding or overriding its own. The framework ships a base vocabulary of
the types common to all formats (bold, italic, underline, code, hyperlink,
image, line break) plus extensions for HTML-rich content (strikethrough,
sub/superscript, highlight, ruby, footnotes) and for code tokens and i18n
placeholders (variables, placeholders, functions, generic markup). A format
reader maps its native constructs to these shared types; an application can layer
a domain-specific vocabulary on top when it needs types the built-ins do not
cover.

## Why this enables format-independent reuse

Because every format reduces inline codes to the same semantic types, content
becomes comparable across formats. The same vocabulary that drives editor
rendering also feeds [translation-memory](/framework/translation-memory)
matching: an entry created from an HTML source can match a Markdown source
because both reduce to the same structural projection. One classification serves
preview, validation, and reuse.

## Related reading

- [Inline Formatting](/framework/inline-formatting) — how spans appear in the content model.
- [Content Model](/framework/content-model) — where spans and fragments live.
- [Authoring Vocabularies](/contribute/vocabularies) — the JSON file format, mapping native elements, and creating a custom vocabulary.
