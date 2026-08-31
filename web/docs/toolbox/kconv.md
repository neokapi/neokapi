---
sidebar_position: 5
title: kconv
description: "kconv converts the content of any supported format into another (Markdown, HTML, DocLang), driven by the structural role layer rather than the source bytes."
keywords: [kconv, convert, markdown, html, doclang, docx, docling, format conversion, multilingual content]
---

# kconv

Convert the content of any supported format into another. `kconv` reads a
document into kapi's content model and re-expresses it in a different format,
carrying the **structure** across (headings, lists, tables and inline
formatting) rather than the source bytes.

```bash
kconv [flags] [FILE...]
```

The target format comes from `--to` (a format id such as `markdown`, `html` or
`doclang`, or an extension such as `md`), or is inferred from the `-o` output
extension. With no `-o`, the result is written to standard output. With no file,
or when the file is `-`, standard input is read.

A conversion that produces a binary document (a same-format `.docx` round-trip,
say) is refused rather than streamed at a terminal. Pass `-o FILE`, redirect
stdout, or name standard output explicitly with `-o -` to ask for the bytes
anyway. Redirected and piped output is never inspected.

Several files convert in one run. Give `-o` a **directory** (a trailing slash,
or a path that already is one) and each input is written to its own file there,
named after the input and re-extensioned for the target format. With `-r`,
directory arguments are walked and the sub-tree is mirrored under the output
directory.

## How the conversion works

`kconv` is a projection of the content model. Each block carries a normalized
**role** (heading, list item, table cell, caption) and `kconv` renders that
role in the target format: a heading becomes `#`/`##` in Markdown or
`<h1>`/`<h2>` in HTML; a list becomes bullets or a `<ul>`; a table reconstructs
as an HTML `<table>` (Markdown lists the cells). Inline formatting is rendered
from each run's type, so a bold span becomes `**…**` or `<strong>…</strong>`
whatever spelling the source format used.

By default `kconv` projects the **source** text. `--target LOCALE` projects an
existing translation instead, useful for emitting a translated document in a
new format.

## Supported output formats

You can read (convert **from**) any supported format. You can write (convert
**to**) the document and data formats that are produced from content alone:

- **Documents**: Markdown, HTML, DocLang, AsciiDoc, plain text
- **Data and catalogs**: JSON, YAML, and the resource-string formats

Run `kapi formats` for the full set, or try the
[Conversion Lab](/lab/convert) to convert in your browser.

**Not conversion targets:**

- **Bilingual interchange** (XLIFF, PO, TMX, KBF): use
  [`kapi extract`](/kapi/cli) instead. Extract captures the source skeleton so
  [`kapi merge`](/kapi/cli) can round-trip translations back into the original
  file; a converted interchange file has no skeleton and cannot be merged back.
- **Packaged formats** (Word (`.docx`), ODT, InDesign, EPUB) and read-only
  formats such as PDF can be converted **from**, but not **to** (they are written
  by updating an existing file of that format).

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

# Everything in a folder to Markdown, one file each, in converted/
kconv ~/Downloads/* --to md -o converted/

# A whole documentation tree to HTML, sub-directories mirrored
kconv -r docs --to html -o site/
```

## Options

| Flag | Meaning |
| ---- | ------- |
| `-t, --to FORMAT` | Target format: a format id (`markdown`, `html`, `doclang`) or an extension (`md`). |
| `-o, --output PATH` | Write to `PATH`; a directory (`-o out/`) writes one file per input; `-` names standard output explicitly. Default is standard output. |
| `-r, --recursive` | Recurse into directory arguments. |
| `--target LOCALE` | Convert the translation for `LOCALE` instead of the source. |
| `-f, --format` | Override input format detection (e.g. `-f docling`). |
| `--source-lang` | Source language (default `en`). |
| `--encoding` | Input/output encoding (default `UTF-8`). |

## Writing to a directory

An output directory is created if it does not exist. Each input becomes
`<name>.<ext>` under it, where `<ext>` is the target format's own extension, and
sub-directories are mirrored relative to the deepest directory every input
shares, so `kconv 'docs/**/*.md' --to html -o site/` writes
`site/guide/intro.html` for `docs/guide/intro.md`, and two files of the same name
in sibling directories cannot collide.

Two inputs that *would* resolve to the same output file (`report.md` and
`report.html` both converted to DocLang) are a reported error rather than a
silent overwrite, and so is a conversion that would write over its own input.
The rest of the run continues; the exit status is 2.

Without a directory, `-o` names a single output file and takes a single input.

## Faithful vs. clean

Converting to the **same** format is a faithful round-trip: everything,
including styling and non-translatable content, is preserved (see [ksed](ksed.md)
for the same fidelity). Converting to a **different** format is deliberately a
*clean projection*: a `.docx` → `.md` keeps the document's structure and prose
but not its Word-specific packaging. Inline formatting is rendered from each
run's vocabulary type, the same model the rest of the toolbox uses (see
[Inline Formatting](/framework/inline-formatting)).
