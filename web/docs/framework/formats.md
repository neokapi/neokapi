---
sidebar_position: 6
title: Formats
description: neokapi formats are paired readers and writers that convert documents to and from the Part stream. Built-in formats span message catalogs, documents, data, subtitles, office files, and bilingual interchange; more are available through plugins.
keywords: [formats, format reader, format writer, XLIFF, JSON, DOCX, Markdown, message catalogs, format plugins]
---

import { BlockPreview } from "@site/src/components/curated";
import { ContentLab } from "@site/src/components/Lab";

# Formats

A **format** in neokapi is a paired reader and writer for a document type. The
[reader](/framework/content-model) turns a file into a stream of Parts (the
content [blocks](/framework/content-model) and the surrounding structure), and the
writer turns that stream back into a file. This read/process/write symmetry is
what lets the same [tools](/framework/tools) and [flows](/framework/flows) operate
on any format: by the time a tool sees a Block, it no longer matters whether it
came from JSON, HTML, or DOCX.

:::tip See the skeleton preserved
Reading a file splits it into content [blocks](/framework/content-model) and
a non-translatable _skeleton_: every tag, key, attribute, and delimiter the
writer needs to reproduce the original structure. Run a file through
`pseudo-translate` below and compare the source with the round-tripped output:
only the leaf text changes, while the skeleton comes back byte-for-byte. This
runs the real `kapi` reader and writer in your browser via WebAssembly.
:::

<ContentLab lessonIds={["roundtrip"]} defaultSampleId="page-html" />

neokapi ships built-in readers and writers spanning several families:

- **Software i18n catalogs**: JSON (including i18next, ARB, and design
  tokens), YAML, Java properties, Android `strings.xml`, Apple
  `.strings`/`.xcstrings`, RESX, Qt TS, gettext PO/MO, ICU MessageFormat.
- **Documents & markup**: Markdown and MDX, HTML, AsciiDoc, XML (with
  configurable translatable elements), plain text.
- **Office & publishing**: Office Open XML (`.docx`, `.xlsx`, `.pptx`),
  OpenDocument, EPUB, PDF.
- **Data**: CSV/TSV, with column-role configuration.
- **Subtitles & media**: SubRip (SRT), WebVTT; images, audio, and video as
  per-locale assets.
- **Source code**: the prose inside source files, read with a grammar (a
  package cask's description, a string literal in Go or TypeScript), provided by
  the `kapi-sourcecode` plugin. It is read-only: a collection in this format is
  declared `source_only: true` and is checked rather than translated. See
  [Source code](/reference/formats/sourcecode) and
  [Verify content in source files](/kapi/recipes/verify-content).
- **Bilingual interchange**: XLIFF 1.2 and 2.x, PO, TMX, and the native
  [Kapi format family](/reference/serialization/overview), the translator-handoff layer.
- **Containers & archives**: ZIP, TAR, and gzip-compressed TAR, treated as a
  folder of sub-documents (see below).

An **image** is read as an asset in its own right: the picture itself is the unit
a workflow can replace with a per-locale variant. With the `kapi-vision` plugin
installed (and the `ocr`/`layout` options on), the reader also extracts in-image
text and document layout (regions, reading order, tables), turning a screenshot
or scanned page into structured, translatable content. The design, and the full
set of image-adaptation modes, are described in
[M-03](/contribute/architecture/multilingual/m-03-multimodal-content).

PDF is read by Google's PDFium rather than a built-in reader: on the desktop and
CLI through the `kapi-pdfium` plugin, and in the browser through PDFium compiled
to WebAssembly. Beyond text, it recovers each fragment's position on the page
(geometry) and the document's structure (headings, paragraphs, and tables) from
the PDF's own tags where present and by geometric inference otherwise. You can try
it on your own files in the [Structure & Layout lab](/lab/structure); the design is described in
[E-08](/contribute/architecture/engine/e-08-document-structure-tiers).

An **archive** (a ZIP, TAR, or gzip-compressed TAR) is a *namespace* of inner
documents rather than one document, so it is handled as a **container binding**
([E-04](/contribute/architecture/engine/e-04-flows-and-io-binding)) rather than a format
with a writer. Pointed at an output-producing command, each entry is processed
as its **own file run** (through that entry's normal reader and writer with full
skeleton round-trip) and the results are repacked over the original container,
copying every other member byte-for-byte. Because each entry is a real,
standalone file run, a **packaged format such as a DOCX or EPUB *inside* the
archive round-trips faithfully**, and each entry resolves its own format
configuration. Binary assets, nested containers, and bilingual interchange files
pass through unchanged. So `kapi pseudo-translate bundle.zip -o out.zip`
pseudo-translates every recognised file inside it, nested Office
documents included, in one pass. For inspection, a read-only `archive` reader surfaces each entry's
content so `kapi inspect bundle.zip` shows what is inside.

Each format exposes its own configuration (extraction rules, segmentation,
inline-code handling). Rather than maintain a list by hand, the
[Format Reference](/formats) is generated directly from the format registry;
it always reflects the formats and parameters in the current build.

## How kapi reads a file

The clearest way to see what a format reader does is to watch it parse a file.
Below, kapi reads an Android `strings.xml` resource and produces the content
model: the translatable blocks, their identifiers, and their source text. This
is the reader stage of the pipeline, with no transformation applied:

<BlockPreview
  sample="strings.xml"
  caption="kapi parsing an Android strings.xml resource into translatable blocks."
/>

The same parser, pointed at a different format, produces blocks of the same
shape. Here an XLIFF bilingual file resolves to the same kind of block stream:

<BlockPreview
  sample="app.xliff"
  caption="The same content model from an XLIFF file: identifiers and source text."
/>

The block shape is the same, but bilingual formats carry more. A monolingual
format (JSON, YAML, properties) produces whole-block source content with no
internal segment structure. A bilingual format (XLIFF, TMX) additionally
populates stand-off [segmentation and alignment overlays](/framework/content-model):
the file's existing segment boundaries and source↔target pairings are recorded as
overlays over the runs rather than baked into structure, so they survive a
round-trip when present and are absent when a format doesn't define them.
Tools and writers read those overlays; a format that emits none works at
whole-block granularity.

## Content fidelity: context for ingestion

The split above (translatable blocks plus an inert skeleton) is only part of
the story. A document carries text that should not be *translated* but is still
*meaningful*: code listings, image captions and alt-text, formulas, strings
explicitly marked do-not-translate, and values a config rule excluded from
translation. For a translation run this is noise; for feeding a document to an
LLM or a retrieval index, it is exactly the context you want to keep.

By default, neokapi readers **surface** this contextual content as
non-translatable blocks rather than hiding it in the skeleton. Such a block is
visible to anything that reads the Part stream (the editor, an export to
Markdown, an ingestion pipeline) and is tagged with a role (code, formula,
caption, …) so consumers know what it is, but machine translation skips it and
the round-trip is unaffected (its original bytes are still replayed verbatim).
Comments and similar metadata surface as data or notes alongside the content.

Each reader that supports this exposes an `extractNonTranslatableContent` option
(on by default) in the [Format Reference](/formats); set it false to restore the
older skeleton-only behavior. The design, and why it leaves translation output
and the round-trip guarantee unchanged, is described in
[E-02](/contribute/architecture/engine/e-02-format-system). Equations are
a notable case: Word/OMML formulas are converted to LaTeX/MathML and rendered on
cross-format export, and the natural-language prose inside an equation is
translatable; see [kconv](/toolbox/kconv) and
[M-04](/contribute/architecture/multilingual/m-04-math-and-equations).

## Plugin formats

[Plugins](/contribute/plugins) can register additional readers and writers
alongside the built-in set: the `kapi-pdfium` plugin adds the PDF reader, for
example. Once installed, a plugin's formats participate in detection, `kapi
formats`, and flows exactly like the built-in ones.

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
