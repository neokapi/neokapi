---
name: kapi-publish
description: Produce localized deliverables by round-tripping content through format-aware readers/writers — extract translatable text from an asset, translate it, and merge it back into the SAME format (DOCX, XLSX, PPTX, HTML, Markdown, JSON, PO, XLIFF, subtitles, and more). Use to publish a document/site/app/deck in multiple languages while preserving layout and structure. Triggers on "publish in N languages", "export the translated docx/pptx", "extract for translation then merge back", "localization round-trip".
---

# kapi-publish

Handles the format mechanics of multilingual publishing: extract → (translate) → merge back, keeping the original structure, markup, and placeholders intact. Built on neokapi's native format readers/writers, with more available through the okapi-bridge.

## When to use

The user wants a finished, localized artifact in its original format — not just raw translated strings. Examples: a French `report.docx`, a German `pitch.pptx`, a Japanese `app/locales/*.json`, localized subtitles, a translated docs site.

## Discover what's translatable

```bash
kapi formats list --json                 # supported formats + read/write capability
kapi word-count ./report.docx --json     # translatable word/segment count
```

## Bilingual extract → merge (vendor/human or AI in the middle)

```bash
# Emit XLIFF per target language (TM pre-fill applied)
kapi extract -p project.kapi --target-lang fr --format xliff2

# ... translate the XLIFF (AI or human) ...

# Merge translated XLIFF back onto the source using the saved skeleton
kapi merge -i ./out/*.fr.xlf -p project.kapi
```

## Direct round-trip (AI in one step)

```bash
kapi ai-translate ./report.docx --target-lang fr -o ./out/report.fr.docx
kapi run ai-translate-qa -i ./deck.pptx --target-lang de -o ./out/deck.de.pptx
```

## Demonstrating with native vs okapi-bridge formats

Native formats (openxml DOCX/XLSX/PPTX, html, markdown, json, yaml, po, properties, csv, xliff, subtitles) work out of the box. For bridge-only or config-preset formats (e.g. IDML/ICML, FrameMaker MIF, RESX, DITA), install the okapi-bridge plugin (`kapi plugins install okapi-bridge`) and select via `--map '*.idml=okf_idml'` or a format preset.

## How to apply

1. Confirm the format reads AND writes (`kapi formats list`); for write-back-limited formats (e.g. PDF is read-only), extract to a bilingual format instead.
2. Bind brand voice + termbase (see `kapi-translate`) so the localized output is on-brand.
3. Produce one deliverable per target language; verify with `kapi word-count`/QA before handing back.
