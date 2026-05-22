---
name: kapi-translate
description: Translate or localize content into other languages on-brand and with consistent terminology, across many file formats (JSON, PO, properties, XLIFF, Markdown, HTML, DOCX/XLSX/PPTX, subtitles, and more). Use when the user wants to ship content in additional languages or localize an app/site/document. Triggers on "translate this", "localize to fr/de/ja", "add Spanish", "multilingual".
---

# kapi-translate

Translates files with `kapi ai-translate` (and composed flows like `ai-translate-qa`). When a brand voice profile and termbase are bound, the translation is on-brand and terminologically consistent at generation time — not just checked afterwards.

## When to use

The user wants content in additional languages: app resource files, website copy, docs, marketing assets, slide decks, spreadsheets, subtitles. Works for source content the assistant just produced or existing files in the repo.

## Prerequisites

- A saved AI credential (`kapi credentials list`) or pass `--api-key`.
- Optional but recommended: a brand voice profile and a termbase for on-brand, consistent output.

## How to run

```bash
# Single file → French, with QA, JSON result
kapi run ai-translate-qa -i ./locales/en.json --target-lang fr --json

# Just translate, multiple targets
kapi ai-translate ./content/page.md --target-lang de --json
kapi ai-translate ./deck.pptx --target-lang ja -o ./out/deck.ja.pptx

# With a glossary/termbase and brand profile bound via a .kapi project
kapi run translate -p myproject.kapi --target-lang fr
```

The format is detected from the extension and the same format is written back (round-trip), preserving structure, tags, and placeholders. Native formats include JSON/YAML/XML, PO/.properties/.strings, XLIFF, Markdown/HTML, CSV, DOCX/XLSX/PPTX, ODF, subtitles; more are available via the okapi-bridge plugin.

## Output

`--json` reports the input/output paths and per-flow results. The translated file is written to `-o`/`--output` or a sibling `out/` path.

## How to apply

1. Detect what's translatable first if unsure (`kapi word-count <file> --json`, `kapi formats list`).
2. Bind brand voice + terminology when available so output is on-brand and consistent.
3. Translate to each target language; run QA (`ai-translate-qa` or `kapi-brand-check` on the output).
4. For a full localization round-trip of an app/site, pair with `kapi-publish`.
