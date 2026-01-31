---
sidebar_position: 3
title: extract & merge
---

# kapi extract / kapi merge

Extract translatable content and merge translations back.

## Extract

### Synopsis

```bash
kapi extract <input> -o <output> -s <source> -t <target> [flags]
```

### Description

Extract translatable content from a document into a bilingual format (typically XLIFF). The extracted file can be translated using any external tool, then merged back.

### Examples

```bash
# Extract to XLIFF
kapi extract input.html -o translations.xliff -s en -t fr

# Extract multiple files
kapi extract docs/*.html -o translations/ -s en -t fr

# Extract with specific output format
kapi extract input.json -o translations.po -s en -t de
```

## Merge

### Synopsis

```bash
kapi merge <translations> -o <output> [flags]
```

### Description

Merge translated content back into the original document format. The merge command reads the bilingual file (XLIFF, PO, etc.) and reconstructs the original document with translations applied.

### Examples

```bash
# Merge XLIFF translations back to HTML
kapi merge translations.xliff -o output.html

# Merge with encoding specification
kapi merge translations.xliff -o output.html --encoding UTF-8
```

## Flags

### Extract Flags

| Flag | Short | Description |
|------|-------|-------------|
| `--output` | `-o` | Output file/directory (required) |
| `--source-lang` | `-s` | Source language (required) |
| `--target-lang` | `-t` | Target language (required) |
| `--format` | `-f` | Override format detection |

### Merge Flags

| Flag | Short | Description |
|------|-------|-------------|
| `--output` | `-o` | Output file path (required) |
| `--encoding` | `-e` | Output encoding (default: UTF-8) |
