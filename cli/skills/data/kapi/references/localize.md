# Translate, enforce terminology, publish

Translate content, enforce terminology, and round-trip the result back into its
original format with the local `kapi` CLI.

## Prerequisites

- A saved AI provider credential (`kapi credentials add`) or `--api-key` for AI
  translation. The format and terminology steps need no credential.
- Optional but recommended: a brand voice profile (see [brand.md](brand.md)) and
  a termbase, so output is on-brand and terminologically consistent.

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

Native readers/writers cover localization, document, data, and office formats —
offline, with no plugin. This includes mobile/app catalogs (Apple String Catalog
`.xcstrings`, `.strings`/`.stringsdict`, Android `strings.xml`, Flutter `.arb`,
i18next JSON, `.resx`) and content formats like Markdown and MDX. A few
specialized or legacy formats are available through the okapi-bridge (select
with `--map '*.sdlppx=okf_sdlpackage'`). When the bridge is installed it can
shadow a shared extension (e.g. `.strings`, `.xml`, `.resx`); pass
`--format <name>` to force the native reader.

## How to apply

1. Confirm the format reads **and** writes (`kapi formats list`); for write-limited
   formats (e.g. PDF is read-only), extract to a bilingual format instead.
2. Bind a brand profile + termbase so output is on-brand and consistent.
3. Pre-flight with `kapi pseudo-translate <file> --target-lang qps` to surface
   hardcoded or untranslated strings before real translation.
