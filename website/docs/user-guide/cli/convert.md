---
sidebar_position: 1
title: convert
---

# kapi convert

Convert documents between formats.

## Synopsis

```bash
kapi convert <input> -o <output> [flags]
```

## Description

The `convert` command reads a document in one format and writes it in another. Format detection is automatic based on file extensions, or can be overridden with flags.

## Examples

```bash
# Convert HTML to XLIFF
kapi convert input.html -o output.xliff -s en -t fr

# Convert with explicit format
kapi convert input.txt --format markdown -o output.xliff

# Convert XLIFF back to original format
kapi convert translations.xliff -o output.html
```

## Flags

| Flag | Short | Description |
|------|-------|-------------|
| `--output` | `-o` | Output file path (required) |
| `--source-lang` | `-s` | Source language (BCP 47 tag) |
| `--target-lang` | `-t` | Target language (BCP 47 tag) |
| `--format` | `-f` | Override input format detection |
| `--output-format` | | Override output format detection |
| `--encoding` | `-e` | Output encoding (default: UTF-8) |
