---
sidebar_position: 6
title: Formats
description: neokapi formats are paired readers and writers that convert documents to and from the Part stream. Built-in formats span localization, document, data, subtitle, and office families; more are available through the Okapi bridge plugin.
keywords: [formats, format reader, format writer, XLIFF, JSON, DOCX, Markdown, localization formats, okapi bridge]
---

import { BlockPreview } from "@site/src/components/curated";
import { ContentLab } from "@site/src/components/Lab";

# Formats

A **format** in neokapi is a paired reader and writer for a document type. The
[reader](/framework/content-model) turns a file into a stream of Parts —
translatable [blocks](/framework/content-model) and the surrounding structure —
and the writer turns that stream back into a file. This read/process/write
symmetry is what lets the same [tools](/framework/tools) and
[flows](/framework/flows) operate on any format: by the time a tool sees a Block,
it no longer matters whether it came from JSON, XLIFF, or DOCX. A format is the
neokapi analogue of an Okapi _filter_.

:::tip See the skeleton preserved
Reading a file splits it into translatable [blocks](/framework/content-model) and
a non-translatable _skeleton_ — every tag, key, attribute, and delimiter the
writer needs to reproduce the original structure. Run a file through
`pseudo-translate` below and compare the source with the round-tripped output:
only the leaf text changes, while the skeleton comes back byte-for-byte. This
runs the real `kapi` reader and writer in your browser via WebAssembly.
:::

<ContentLab lessonIds={["roundtrip"]} defaultSampleId="page-html" />

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
- **Images** — PNG and JPEG, as localizable assets.
- **Plain text variants** — paragraph, Moses, versified, and spliced-line text.

An **image** is read as a localizable asset: the picture itself is the unit a
workflow can replace with a per-locale variant. With the `kapi-vision` plugin
installed (and the `ocr`/`layout` options on), the reader also extracts in-image
text and document layout — regions, reading order, tables — turning a screenshot
or scanned page into structured, translatable content. The design, and the full
set of image-localization modes, are described in
[AD-029](/contribute/architecture/029-vision-and-image-localization).

PDF is read by Google's PDFium rather than a built-in reader: on the desktop and
CLI through the `kapi-pdfium` plugin, and in the browser through PDFium compiled
to WebAssembly. Beyond text, it recovers each fragment's position on the page
(geometry) and the document's structure — headings, paragraphs, and tables — from
the PDF's own tags where present and by geometric inference otherwise. You can try
it on your own files in the [Structure & Layout lab](/lab/structure); the design is described in
[AD-028](/contribute/architecture/028-pdf-reader-plugin).

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

The block shape is the same, but bilingual formats carry more. A monolingual
format (JSON, YAML, properties) produces whole-block source content with no
internal segment structure. A bilingual format (XLIFF, TMX) additionally
populates stand-off [segmentation and alignment overlays](/framework/content-model):
the file's existing segment boundaries and source↔target pairings are recorded as
overlays over the runs rather than baked into structure, so they survive a
round-trip when present and are simply absent when a format doesn't define them.
Tools and writers read those overlays; a format that emits none works at
whole-block granularity.

## Content fidelity: context for ingestion

The split above — translatable blocks plus an inert skeleton — is not the whole
story. A document carries text that should not be *translated* but is still
*meaningful*: code listings, image captions and alt-text, formulas, strings
explicitly marked do-not-translate, and values a config rule excluded from
translation. For a translation run this is noise; for feeding a document to an
LLM or a retrieval index, it is exactly the context you want to keep.

By default, neokapi readers **surface** this contextual content as
non-translatable blocks rather than hiding it in the skeleton. Such a block is
visible to anything that reads the Part stream — the editor, an export to
Markdown, an ingestion pipeline — and is tagged with a role (code, formula,
caption, …) so consumers know what it is, but machine translation skips it and
the round-trip is unaffected (its original bytes are still replayed verbatim).
Comments and similar metadata surface as data or notes alongside the content.

Each reader that supports this exposes an `extractNonTranslatableContent` option
(on by default) in the [Format Reference](/formats); set it false to restore the
older skeleton-only behavior. The design — and why it leaves translation output
and Okapi parity unchanged — is described in
[AD-031](/contribute/architecture/031-content-fidelity-surfacing). Equations are
a notable case: Word/OMML formulas are converted to LaTeX/MathML and rendered on
cross-format export, and the natural-language prose inside an equation is
translatable — see [kconv](/toolbox/kconv) and
[AD-032](/contribute/architecture/032-math-and-equations).

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
