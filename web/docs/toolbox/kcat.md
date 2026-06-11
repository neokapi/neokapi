---
sidebar_position: 4
title: kcat
description: kcat prints the translatable text of files block by block, regardless of format — a Word document, a JSON catalog and an XLIFF file all print as plain prose.
keywords: [kcat, cat, print, extract text, docx, json, xliff, localization]
---

# kcat

Print the human-readable text extracted from each file, one block per line,
regardless of the underlying format. A Word `.docx`, a JSON catalog and an
XLIFF file all print as their plain prose, with the markup and structure
stripped.

```bash
kcat [flags] [FILE...]
```

With no file, or when the file is `-`, standard input is read.

## Examples

```bash
# See the prose inside a Word document
kcat report.docx

# Number the blocks of a JSON catalog
kcat -n locales/en.json

# Print the French translations of an XLIFF file
kcat --target fr messages.xliff

# Pipe arbitrary text through, treating it as plain text
cat raw.txt | kcat -f plaintext
```

`kcat` pairs naturally with the shell tools you already have — pipe its output
into the real `grep`, `wc`, or `sort` when you want byte-level line behaviour
rather than the block-aware `kgrep`.

## Options

| Flag | Meaning |
| ---- | ------- |
| `-n, --number` | Number the output blocks. |
| `--id` | Prefix each block with its source ID. |
| `--target LOCALE` | Print the translation for `LOCALE` instead of the source. |
| `-f, --format` | Override format detection (e.g. `-f json`). |
| `--source-lang` | Source language (default `en`). |
| `--encoding` | Input encoding (default `UTF-8`). |
| `--json` | Emit blocks as JSON instead of plain text. |
