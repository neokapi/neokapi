---
sidebar_position: 2
title: formats
---

# kapi formats

List supported file formats and show format details.

## Synopsis

```bash
kapi formats [flags]
kapi formats info <name>
kapi formats schema <name>
```

## Description

Lists all file formats that kapi can read and write. Use `--mime` or `--ext` to filter by MIME type or file extension.

## Examples

```bash
# List all supported formats
kapi formats

# Filter by file extension
kapi formats --ext .json

# Filter by MIME type
kapi formats --mime text/html

# Show details for a specific format
kapi formats info json

# Output JSON schema for a format
kapi formats schema json
```

## Subcommands

| Command | Description |
|---------|-------------|
| `info <name>` | Show detailed information about a format |
| `schema <name>` | Output JSON Schema for a format's configuration |

## Flags

| Flag | Description |
|------|-------------|
| `--ext` | Filter by file extension (e.g., `.docx`) |
| `--mime` | Filter by MIME type (e.g., `text/html`) |

## Supported Formats

| Format | Extensions | Description |
|--------|-----------|-------------|
| HTML | .html, .htm | HTML documents |
| XML | .xml | XML documents |
| XLIFF | .xlf | XLIFF 1.2 translation files |
| XLIFF 2 | .xlf | XLIFF 2.0/2.1 translation files |
| JSON | .json | JSON key-value and nested structures |
| YAML | .yaml, .yml | YAML documents |
| PO | .po | GNU gettext translation files |
| Properties | .properties | Java properties files |
| Plaintext | .txt | Plain text files |
| Markdown | .md | Markdown documents |
| CSV | .csv | Comma-separated values |
| SRT | .srt | SubRip subtitle files |
| VTT | .vtt | WebVTT subtitle files |
| TMX | .tmx | Translation Memory eXchange |
| TBX | .tbx | TermBase eXchange |

Additional formats available via plugins (e.g., DOCX, XLSX via [Okapi bridge](/docs/kapi-cli/commands/plugins)).
