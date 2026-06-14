---
sidebar_position: 5
title: kconv
description: kconv converts the content of any supported format into another — Markdown, HTML, DocLang — driven by the structural role layer rather than the source bytes.
keywords: [kconv, convert, markdown, html, doclang, docx, docling, format conversion, localization]
---

# kconv

Convert the content of any supported format into another. `kconv` reads a
document into kapi's content model and re-expresses it in a different format,
carrying the **structure** across — headings, lists, tables and inline
formatting — rather than the source bytes.

```bash
kconv [flags] [FILE...]
```

The target format comes from `--to` — a format id such as `markdown`, `html` or
`doclang`, or an extension such as `md` — or is inferred from the `-o` output
extension. With no `-o`, the result is written to standard output. With no file,
or when the file is `-`, standard input is read.

## How the conversion works

`kconv` is a projection of the content model. Each block carries a normalized
**role** — heading, list item, table cell, caption — and `kconv` renders that
role in the target format: a heading becomes `#`/`##` in Markdown or
`<h1>`/`<h2>` in HTML; a list becomes bullets or a `<ul>`; a table reconstructs
as an HTML `<table>` (Markdown lists the cells). Inline formatting is rendered
from each run's type, so a bold span becomes `**…**` or `<strong>…</strong>`
whatever spelling the source format used.

Which path runs depends on whether the source and target formats match:

- **Same format** (e.g. `.docx` → `.docx`): the document round-trips faithfully
  through its skeleton, so structure, styles and non-translatable content are
  preserved.
- **Cross format** (e.g. `.docx` → `.md`): the target is reconstructed from the
  content model; the source's byte skeleton is not carried into a foreign
  writer, so the output is clean in the target format.

By default `kconv` projects the **source** text. `--target LOCALE` projects an
existing translation instead — useful for emitting a translated document in a
new format.

## Examples

```bash
# A Word proposal as clean Markdown (to stdout)
kconv proposal.docx --to md

# A DocLang document to an HTML file (format from the extension)
kconv report.dclg.xml -o report.html

# A Docling-parsed scan (DoclingDocument JSON) as HTML
kconv scan.docling.json --to html

# Any supported format to DocLang
kconv guide.md -o guide.dclg.xml

# Emit the French translation of an XLIFF as Markdown
kconv messages.xliff --to md --target fr
```

## Options

| Flag | Meaning |
| ---- | ------- |
| `-t, --to FORMAT` | Target format — a format id (`markdown`, `html`, `doclang`) or an extension (`md`). |
| `-o, --output PATH` | Write to `PATH` (format inferred from its extension); default is standard output. |
| `--target LOCALE` | Convert the translation for `LOCALE` instead of the source. |
| `-f, --format` | Override input format detection (e.g. `-f docling`). |
| `--source-lang` | Source language (default `en`). |
| `--encoding` | Input/output encoding (default `UTF-8`). |

`-o` takes a single input file — convert files one at a time, or omit `-o` to
stream to standard output. The target needs a writer: read-only formats such as
PDF can be converted *from* but not *to*.

## Faithful vs. clean

A same-format conversion is a faithful round-trip — see [ksed](ksed.md) for the
same skeleton-backed fidelity. A cross-format conversion is deliberately *not*
byte-faithful: it is a clean projection in the target format, so a `.docx` →
`.md` keeps the document's structure and prose but not its Word-specific
packaging. Inline formatting is rendered from each run's vocabulary type, the
same model the rest of the toolbox uses (see
[Inline Formatting](/framework/inline-formatting)).
