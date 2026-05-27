---
sidebar_position: 2
title: kgrep
description: kgrep searches the translatable text inside any supported format for a regular expression, skipping markup and structure. It mirrors grep's options and exit status.
keywords: [kgrep, grep, search, regex, docx, json, xliff, localization]
---

# kgrep

Search the human-readable text inside any supported format for a regular
expression — the prose of a Word `.docx`, the values of a JSON catalog, the
segments of an XLIFF file — skipping markup and structure. Output mirrors
`grep`: one matching block per line, optionally prefixed with the file name and
the block number.

```bash
kgrep [flags] PATTERN [FILE...]
```

The pattern is a [Go regular expression](https://pkg.go.dev/regexp/syntax). With
no file, or when the file is `-`, standard input is read. Exit status is `0` if
any block matched, `1` if none did, `2` on error — the same convention as
`grep`, so `kgrep` composes in shell conditionals.

## Examples

```bash
# Find a word inside a Word document
kgrep "Tervetuloa" report.docx

# Case-insensitive search across JSON catalogs
kgrep -i todo locales/*.json

# Recurse a content tree, searching French translations
kgrep -r --target fr "déconnexion" ./content

# Count occurrences per file
kgrep -c "©" *.md

# Use kgrep's exit status in a script
if kgrep -q "DRAFT" manual.docx; then
  echo "manual still contains a DRAFT marker"
fi
```

## Options

| Flag | Meaning |
| ---- | ------- |
| `-i, --ignore-case` | Case-insensitive matching. |
| `-v, --invert-match` | Select blocks that do **not** match. |
| `-c, --count` | Print a count of matching blocks per file. |
| `-n, --line-number` | Prefix each match with its block number. |
| `-o, --only-matching` | Print only the matched text, not the whole block. |
| `-l, --files-with-matches` | Print only the names of files containing matches. |
| `-L, --files-without-match` | Print only the names of files with no match. |
| `-w, --word-regexp` | Match only whole words. |
| `-F, --fixed-strings` | Treat the pattern as a literal string, not a regexp. |
| `-r, --recursive` | Recurse into directory arguments. |
| `-H, --with-filename` | Print the file name for each match. |
| `--no-filename` | Suppress file-name prefixes. |
| `-e, --regexp PATTERN` | Add a pattern (repeatable); a block matches if any matches. |
| `-q, --quiet` | Suppress output; report the result through exit status only. |
| `--color MODE` | Highlight matches: `auto`, `always`, `never`. |
| `--target LOCALE` | Search the translation for `LOCALE` instead of the source. |
| `--source-lang LOCALE` | Source language of the content (default `en`). |
| `-f, --format` | Override format detection (e.g. `-f json`). |
| `--encoding NAME` | Input encoding (default `UTF-8`). |
| `--json` | Emit matches as JSON. |
