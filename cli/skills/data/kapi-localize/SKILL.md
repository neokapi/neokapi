---
name: kapi-localize
description: Translate and publish content in other languages — on-brand and with consistent terminology — across many file formats (JSON, PO, properties, XLIFF, Markdown, HTML, DOCX/XLSX/PPTX, ODF, subtitles). Includes building and enforcing a glossary/termbase. Use to ship content in additional languages, localize an app/site/document, produce a localized deliverable, or keep terminology consistent. Triggers on "translate", "localize to fr/de/ja", "add Spanish", "multilingual", "publish in N languages", "export the translated docx/pptx", "glossary", "terminology", "TBX".
---

# kapi-localize

Translates content, enforces terminology, and round-trips the result back into
its original format with the local `kapi` CLI.

## Prerequisites

- A saved AI provider credential (`kapi credentials add`) or `--api-key` for
  AI translation. The format and terminology steps need no credential.
- Optional but recommended: a brand voice profile (see `kapi-brand`) and a
  termbase, so output is on-brand and terminologically consistent.

## Translate

```bash
kapi run ai-translate-qa -i ./locales/en.json --target-lang fr --json   # translate + QA
kapi ai-translate ./deck.pptx --target-lang ja -o ./out/deck.ja.pptx
```

`--target-lang` is single-valued, so run one command per locale. When a brand
profile is bound on the flow, translation is on-brand at generation time. Format
is detected from the extension and written back unchanged (round-trip),
preserving structure, tags, and placeholders.

## Keep terminology consistent

```bash
kapi termbase import glossary.csv --format csv -s en -t fr --local   # also: json, tbx
kapi termbase lookup "checkout" -s en -t fr --json
kapi term-check ./locales/fr.json --json                            # flag wrong/missing terms
```

Use the approved (preferred) term; avoid deprecated/forbidden ones. A bound
termbase also feeds the translation step.

## Publish (format round-trip)

```bash
kapi formats list --json                 # what reads and writes
kapi word-count ./report.docx --json     # translatable word/segment count
```

Direct round-trip, or a bilingual extract → translate → merge cycle for vendor
or human translation:

```bash
kapi ai-translate ./report.docx --target-lang fr -o ./out/report.fr.docx
kapi extract -p project.kapi --target-lang fr --format xliff2          # emit XLIFF
kapi merge -i ./out/*.fr.xlf -p project.kapi                          # merge back
```

Native readers/writers cover localization, document, data, and office formats;
more are available through the okapi-bridge (select with `--map '*.idml=okf_idml'`).

## How to apply

1. Confirm the format reads **and** writes (`kapi formats list`); for write-limited
   formats (e.g. PDF is read-only), extract to a bilingual format instead.
2. Bind a brand profile + termbase so output is on-brand and consistent.
3. Pre-flight with `kapi pseudo-translate <file> --target-lang qps` to surface
   hardcoded or untranslated strings before real translation.
