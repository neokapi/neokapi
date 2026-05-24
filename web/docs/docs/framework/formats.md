---
sidebar_position: 5
title: Formats
description: neokapi formats are paired readers and writers that convert documents to and from the Part stream. Built-in formats span localization, document, data, subtitle, and office families; more are available through the Okapi bridge plugin.
keywords: [formats, format reader, format writer, XLIFF, JSON, DOCX, Markdown, localization formats, okapi bridge]
---

import { BlockPreview } from "@site/src/components/curated";

# Formats

A **format** in neokapi is a paired reader and writer for a document type. The
[reader](/framework/content-model) turns a file into a stream of Parts —
translatable [blocks](/framework/content-model) and the surrounding structure —
and the writer turns that stream back into a file. This read/process/write
symmetry is what lets the same [tools](/framework/tools) and
[flows](/framework/flows) operate on any format: by the time a tool sees a Block,
it no longer matters whether it came from JSON, XLIFF, or DOCX. A format is the
neokapi analogue of an Okapi _filter_.

neokapi ships built-in readers and writers spanning several families:

- **Localization** — XLIFF 1.2 and 2.x, PO/POT, TMX, Qt TS, ICU MessageFormat,
  Trados TTX/TXML, and translation tables.
- **Document & markup** — HTML, XML (with configurable translatable elements),
  Markdown, wiki markup, TeX/LaTeX, DTD, RTF.
- **Data & configuration** — JSON and YAML (with regex- and key-path-based
  extraction rules), Java properties, CSV/TSV, fixed-width, PHP, and generic
  regex extraction.
- **Office & desktop publishing** — Office Open XML, OpenDocument, Adobe
  ICML/IDML, FrameMaker MIF, EPUB, PDF.
- **Subtitles** — SubRip (SRT), WebVTT, TTML/DFXP.
- **Plain text variants** — paragraph, Moses, versified, and spliced-line text.

Each format exposes its own configuration (extraction rules, segmentation,
inline-code handling). Rather than maintain a list by hand, the
[Format Reference](/formats) is generated directly from the format registry —
it always reflects the formats and parameters in the current build.

## How kapi reads a file

The clearest way to see what a format reader does is to watch it parse a file.
Below, kapi reads an Android `strings.xml` resource and produces the content
model — the translatable blocks, their identifiers, and their source text. This
is the reader stage of the pipeline, with no transformation applied:

<BlockPreview
  sample="strings.xml"
  caption="kapi parsing an Android strings.xml resource into translatable blocks."
/>

The same parser, pointed at a different format, produces blocks of the same
shape. Here an XLIFF bilingual file resolves to the same kind of block stream:

<BlockPreview
  sample="app.xliff"
  caption="The same content model from an XLIFF file — identifiers and source text."
/>

## Okapi bridge formats

With the Okapi bridge [plugin](/contribute/plugins) installed, kapi can also
dispatch to the Java-based filters of the Okapi Framework — covering additional
formats such as DITA that the native readers do not — without rewriting them in
Go.

## Format Detection

neokapi automatically detects formats using a cascade strategy:

1. Explicit MIME type (if provided)
2. File extension mapping
3. Magic bytes / content sniffing

You can override detection with the `--format` flag on any command.

## Listing Formats

```bash
kapi formats
```

Use `--mime` or `--ext` to filter:

```bash
kapi formats --mime text/html
kapi formats --ext .docx
```

## Interactive Format Reference

See the [Format Reference](/formats) page for interactive documentation of all formats with configurable parameters.
