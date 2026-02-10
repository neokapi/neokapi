---
sidebar_position: 2
title: Supported Formats
---

# Supported Formats

gokapi includes 15 built-in format readers and writers, plus access to 40+ additional formats via the Java bridge.

## Built-in Formats

| Format | Extensions | Description |
|--------|-----------|-------------|
| **HTML** | `.html`, `.htm`, `.xhtml` | HTML documents with inline code support |
| **XML** | `.xml` | Generic XML with configurable translatable elements |
| **XLIFF** | `.xlf`, `.xliff` | XLIFF 1.2 bilingual exchange format |
| **XLIFF 2** | `.xlf`, `.xliff` | XLIFF 2.0/2.1 with segment support |
| **JSON** | `.json` | JSON files with configurable key patterns |
| **YAML** | `.yaml`, `.yml` | YAML files with configurable key patterns |
| **PO** | `.po`, `.pot` | GNU gettext translation files |
| **Properties** | `.properties` | Java properties files |
| **Plaintext** | `.txt` | Plain text with paragraph segmentation |
| **Markdown** | `.md`, `.markdown` | Markdown with inline code preservation |
| **CSV** | `.csv`, `.tsv` | Comma/tab-separated with configurable columns |
| **SRT** | `.srt` | SubRip subtitle format |
| **VTT** | `.vtt` | WebVTT subtitle format |
| **TMX** | `.tmx` | Translation Memory eXchange |

## Java Bridge Formats

With the Okapi Java bridge plugin installed, these additional formats are available:

- Microsoft Office (DOCX, XLSX, PPTX)
- OpenDocument (ODT, ODS, ODP)
- Adobe InDesign (IDML)
- EPUB
- DITA
- FrameMaker (MIF)
- PDF (text extraction)
- And many more

See [Plugins](/docs/user-guide/cli/plugins) for installation instructions.

## Format Detection

gokapi automatically detects formats using a cascade strategy:

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
