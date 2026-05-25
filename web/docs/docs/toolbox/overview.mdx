---
sidebar_position: 1
title: Toolbox Overview
description: kgrep, ksed and kcat are format-aware reimaginings of grep, sed and cat. They operate on the translatable text inside any format kapi understands — Word documents, JSON catalogs, XLIFF, Markdown — instead of raw bytes.
keywords: [kgrep, ksed, kcat, grep, sed, cat, format-aware, docx, localization, CLI]
---

# Toolbox

The toolbox is a set of format-aware reimaginings of the classic Unix text
utilities. Where `grep`, `sed` and `cat` operate on raw bytes — lines of a file
as the operating system stores them — `kgrep`, `ksed` and `kcat` operate on the
**translatable text** that kapi extracts from a document, regardless of how that
text is encoded on disk.

The practical consequence: you can search the prose inside a Word `.docx`,
rewrite a phrase across a directory of JSON catalogs, or print the segments of
an XLIFF file as plain text — using muscle memory from tools you already know,
without first converting anything.

| Tool   | Classic analogue | Operates on                                  |
| ------ | ---------------- | -------------------------------------------- |
| `kcat` | `cat`            | the text of each block, one per line         |
| `kgrep`| `grep`           | matches a pattern against each block's text  |
| `ksed` | `sed`            | rewrites block text and saves the document   |

Each works on every format kapi can read — see the [Format
Reference](/formats) for the full list.

## Why blocks, not lines

A `.docx` has no "lines" in the byte sense; a JSON catalog's meaningful units
are keyed strings, not file rows. kapi parses a document into **blocks** of
translatable content and strips the surrounding markup and structure. The
toolbox operates on those blocks:

- `kcat` prints one block per output line.
- `kgrep` reports one matching block per line, and `-n` prefixes the block's
  ordinal position. For genuinely line-oriented formats such as plain text, one
  block is one line, so `-n` reads exactly like `grep -n`.
- `ksed` rewrites a block's text and asks the format writer to reconstruct the
  document, so structure is preserved.

## Installation

The toolbox ships inside the `kapi` binary as a multi-call ("busybox-style")
program: `kgrep`, `ksed` and `kcat` are alternate names for `kapi` that dispatch
to the matching subcommand. Installing the kapi CLI installs all three:

```bash
brew install neokapi/tap/kapi-cli
```

Every utility is also reachable as a kapi subcommand — `kapi grep`, `kapi sed`,
`kapi cat` — which is useful inside scripts that already invoke `kapi`. These
subcommands are hidden from `kapi --help` (to keep the focus on the dedicated
commands) but behave identically to `kgrep`/`ksed`/`kcat`.

## Conventions shared by all three

- **Standard input.** With no file argument, or when the file is `-`, input is
  read from standard input. Without a file extension to go by, the format is
  sniffed from the content and falls back to plain text.
- **Format selection.** `-f, --format` overrides format detection (for example
  `-f json`). Note this differs from `grep`/`sed`, where `-f` reads patterns or
  scripts from a file; in the toolbox `-f` always means *format*, matching the
  rest of the kapi CLI. Use `-e` for explicit patterns and scripts.
- **Source vs. translation.** By default the utilities read the source text.
  `--target LOCALE` operates on a committed translation instead — for example
  `kgrep --target fr "déconnexion" messages.xliff` searches the French
  translations.

## Subcommand and busybox forms are identical

`kapi grep`/`kapi sed`/`kapi cat` are flag-detached: they do not inherit kapi's
global flags, so the full set of short options — including `-v`, `-c` and `-q`,
which kapi's globals would otherwise claim — mean exactly what they mean in
`grep`/`sed`/`cat`. `kapi grep -v` inverts the match; it is not kapi's
`--verbose`. The dedicated `kgrep`/`ksed`/`kcat` commands and their `kapi …`
aliases run the same code with the same options.
