---
sidebar_position: 3
title: ksed
description: ksed applies sed-style substitutions to the translatable text inside any supported format, then writes the document back in the same format with its structure intact.
keywords: [ksed, sed, substitution, replace, docx, json, xliff, localization]
---

# ksed

Apply `sed`-style substitutions to the human-readable text inside any supported
format, then write the document back in the same format. Only the editable text
changes — a `.docx` keeps its styles, a JSON catalog keeps its keys and shape.

```bash
ksed [flags] SCRIPT [FILE...]
```

`SCRIPT` is a substitution command, `s/regexp/replacement/flags`:

- Any single-byte delimiter works, so `s|a|b|` is equivalent to `s/a/b/` — handy
  when the text contains slashes.
- The `g` flag replaces every match in a block (otherwise only the first); the
  `i` flag makes the match case-insensitive.
- Replacements support backreferences `\1`…`\9` and `&` for the whole match.
- Pass several substitutions with repeated `-e`.

By default the edited document is written to standard output, like `sed`. Use
`-i` to edit files in place, optionally keeping a backup (`-i.bak`). With no
file, or when the file is `-`, standard input is read.

## Examples

```bash
# Normalise spelling across a Markdown guide (to stdout)
ksed 's/colour/color/g' guide.md

# Rebrand in place across every Word document
ksed -i 's/Inc\./LLC/' *.docx

# Two substitutions, keeping a .bak backup of each file
ksed -i.bak -e 's/v1/v2/g' -e 's/beta//' locales/en.json

# Edit the French translations rather than the source
ksed --target fr 's/Bonjour/Salut/g' messages.xliff

# Reorder with backreferences
ksed 's/(\w+), (\w+)/\2 \1/' names.txt
```

## Options

| Flag | Meaning |
| ---- | ------- |
| `-e, --expression SCRIPT` | Add a substitution script (repeatable). |
| `-i, --in-place[=SUFFIX]` | Edit files in place; append a backup `SUFFIX` if given (e.g. `-i.bak`). |
| `--target LOCALE` | Edit the translation for `LOCALE` instead of the source. |
| `-f, --format` | Override format detection (e.g. `-f json`). |
| `--source-lang` | Source language (default `en`). |
| `--encoding` | Input/output encoding (default `UTF-8`). |

## Faithful round-trips

`ksed` reuses kapi's reader/writer pipeline, so editing a structured format and
writing it back preserves everything that is not the edited text. For formats
with a skeleton — Office Open XML among them — the document's structure, styles
and non-translatable content round-trip unchanged; the fidelity is *semantic*
(the same text and structure), not a byte-for-byte copy. Only the `s///`
substitution command is supported; compose multiple with `-e`.
