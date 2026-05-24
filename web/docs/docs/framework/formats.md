---
title: Formats
---

# Formats

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
- **Plain text variants and containers** — paragraph, Moses, versified, and
  spliced-line text, plus glob-filtered ZIP archives.

Each format exposes its own configuration (extraction rules, segmentation,
inline-code handling). Rather than maintain a list by hand, the
[Format Reference](/formats) is generated directly from the format registry —
it always reflects the formats and parameters in the current build.

## Okapi bridge formats

With the [Okapi bridge plugin](/commands?id=plugin) installed, kapi can
also dispatch to the Java-based filters of the Okapi Framework — covering
additional formats such as DITA that the native readers do not — without
rewriting them in Go.

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
