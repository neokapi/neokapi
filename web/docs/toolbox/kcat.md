---
sidebar_position: 4
title: kcat
description: kcat prints the text of files block by block, regardless of format, so a Word document, a JSON catalog and an XLIFF file all print as plain prose.
keywords: [kcat, cat, print, extract text, docx, json, xliff, multilingual content]
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

For byte-level line behaviour, pipe `kcat` output into the real `grep`, `wc` or
`sort`. For block-aware search, use `kgrep`.

## Options

| Flag | Meaning |
| ---- | ------- |
| `-n, --number` | Number the output blocks. |
| `--id` | Prefix each block with its source ID. |
| `-r, --recursive` | Recurse into directory arguments. |
| `--target LOCALE` | Print the translation for `LOCALE` instead of the source. |
| `-f, --format` | Override format detection (e.g. `-f json`). |
| `--source-lang` | Source language (default `en`). |
| `--encoding` | Input encoding (default `UTF-8`). |
| `--json` | Emit blocks as JSON instead of plain text. |
