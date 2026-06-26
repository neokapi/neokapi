---
sidebar_position: 6
title: kdiff
description: kdiff compares the translatable text of two files block by block, regardless of format — a reflowed Word document or a reordered JSON catalog shows only the prose that actually changed, not byte-level noise.
keywords: [kdiff, diff, compare, changeset, coverage, docx, json, xliff, localization]
---

# kdiff

Compare the human-readable text inside any supported format, **block by block**,
rather than byte by byte. A reflowed Word `.docx`, a re-zipped container or a
reordered JSON catalog do not register as a diff — only the prose that actually
changed does. This is the changeset a translator (or a translation engine) cares
about: what content moved, was added, was removed, or was rewritten.

```bash
kdiff [flags] FILE_A [FILE_B]
```

With no file extension to go by, the format is sniffed from the content. A file
of `-` reads standard input.

## Two modes

### Revision diff — two files

What translatable content changed between two versions of a document.

```bash
# What changed between two catalog versions
kdiff old.json new.json

# What changed in a Word proposal — ignoring re-save / formatting noise
kdiff proposal.docx proposal-v2.docx

# What changed in the French translation specifically
kdiff --target fr old.xliff new.xliff
```

### Coverage diff — one file against its own translation

Pass a single file with `--target LOCALE` to compare a translation against the
source *within that file* — a quick coverage report of which blocks are still
untranslated or are a verbatim copy of the source.

```bash
kdiff --target fr messages.xliff
```

```text
app [fr]: 18 translated, 3 untranslated, 1 identical to source
@@ "settings.title" (untranslated) @@
  Settings
@@ "brand.name" (identical) @@
  Acme
```

## How blocks are aligned

`kdiff` does not compare line positions — it aligns the document's **blocks**.
The alignment strategy is chosen automatically:

- **Keyed formats** (JSON, XLIFF, PO, `.resx`, … — anything with a stable block
  key) align **by key**. Reordering keys is therefore not a diff; renaming the
  value under a key is a *change*; a new key is an *addition*. Reordered keys are
  reported as `moved`, not as a wholesale rewrite the way a line diff would show
  them.
- **Prose formats** (Word, Markdown, HTML — whose block identities are
  positional) align **by content**, using a longest-common-subsequence match
  over the block text. Inserting a paragraph shows up as a single added block,
  not as a cascade of "everything after it changed".

Force a strategy with `--by id` or `--by content` when the automatic choice is
wrong for your file.

## Output

The default output is a block-oriented unified diff: a `@@ <key> (kind) @@`
header per changed block, then the removed (`-`) and added (`+`) text.

```text
--- a/old.json
+++ b/new.json
@@ "greeting" (changed) @@
- Hello
+ Hi there
@@ "welcome" (added) @@
+ Welcome!
@@ "farewell" (removed) @@
- Goodbye
```

`--json` emits the same changeset as structured data for scripts and tools, and
`--stat` prefixes a one-line summary (`2 changed, 1 added, 1 removed`). `-q,
--brief` prints only whether the inputs differ.

## Exit status

`kdiff` follows the classic `diff` convention, so shell scripts and CI can branch
on the result:

| Code | Meaning |
| ---- | ------- |
| `0` | The inputs are equivalent (or the translation is fully covered). |
| `1` | They differ (or there is untranslated / copied content). |
| `2` | An operational error (a file could not be read or parsed). |

## Examples

```bash
# Review what changed in a release's source catalog
kdiff v1/en.json v2/en.json

# Gate CI on a fully-translated file (exit 1 if anything is pending)
kdiff --target de -q messages.xliff || echo "German is incomplete"

# Machine-readable changeset for a re-translation pipeline
kdiff --json old.xliff new.xliff > changes.json

# Compare two drafts of a Word document, prose only
kdiff --by content draft.docx final.docx
```

## Options

| Flag | Meaning |
| ---- | ------- |
| `--by STRATEGY` | Alignment: `auto` (default), `id`, or `content`. |
| `--target LOCALE` | Compare the translation for `LOCALE`. With one file, a coverage report of source vs. translation. |
| `-q, --brief` | Report only whether the inputs differ, not the changes. |
| `--stat` | Print a one-line summary of the changes before the diff. |
| `--json` | Emit the diff as JSON. |
| `--color` | Colorize the diff: `auto`, `always`, `never`. |
| `-f, --format` | Override format detection (e.g. `-f json`). |
| `--source-lang` | Source language (default `en`). |
| `--encoding` | Input encoding (default `UTF-8`). |
