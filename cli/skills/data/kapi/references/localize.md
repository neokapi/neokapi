# Translate, enforce terminology, publish

Translate content, enforce terminology, and round-trip the result back into its
original format with the local `kapi` CLI. For ongoing work, bind the locales,
brand voice, and glossary in a project first — see [project.md](project.md).

## First decide: one-off file, or a project?

**A single file you just need translated** — a document, a deck, one catalog —
translate it directly. There is nothing to set up, and **do not run `kapi extract`**
(that reads a *project's* content config; on a loose file it fails with "no .kapi
project found"). Just round-trip it:

```bash
kapi pseudo-translate <file> --target-lang qps        # quick readiness pre-flight
kapi translate <file> --target-lang <lang> -o <out>  # reads the format, translates, writes it back
```

kapi preserves structure, tags, and placeholders (round-trip). Add `--credential
<name>` when it needs a model provider. That's the whole task for a one-off file.

**Ongoing / app localization, or translating it yourself under brand + terminology
guardrails** — bind a project first (`kapi init`), then `kapi extract → fill the
targets → kapi merge → kapi verify` (below). `kapi extract` and `kapi merge` operate
on a project; run them inside one (or with `-p <recipe>`), never on a bare file path.

## Commands at a glance (use these exact forms)

Run these as written — don't guess flags. When in doubt, `kapi <cmd> --help`.

```bash
kapi extract --target-lang fr                  # → out/<name>.en-to-fr.xliff (one --target-lang)
kapi merge -i out/*.xliff                       # -i is REQUIRED and repeatable; positional paths are ignored
kapi verify --json                              # the gate: brand + terminology + QA in one shot (prefer this)
kapi term-check ./locales/fr.json --target-lang fr   # file is POSITIONAL; there is no --source/--target
kapi termbase lookup "board" -t fr              # approved wording; termbase uses -s/-t, not --*-lang
kapi brand guide                                # the voice to follow (no flag inside a project)
```

Inside a project, prefer `kapi verify` over running `term-check`/QA by hand — it
runs every bound gate together and pairs source↔target for you.

## Translate the content yourself, through kapi (don't hand-translate files)

Translate the content yourself — don't reach for a provider — but route the
translation **through kapi** so the guardrails actually apply. Don't read the source
file, translate it in your head, and write the target file directly: that quietly
skips terminology, placeholder and format integrity, and the brand voice — the very
things kapi exists to enforce, and the things a human reviewer will later hold you to.
Instead, let kapi pull out the text and the rules, do the translating, and let kapi
write it back. (Inside a project, the kapi Claude Code plugin enforces this with a
PreToolUse hook that blocks direct edits to generated target files — route the
change through the round-trip below, or edit the source.)

```bash
kapi extract --target-lang fr        # bilingual file with source + empty targets (out/*.xliff)
kapi brand guide                     # the voice to follow (no flag inside a project)
kapi termbase lookup "<term>" -t fr  # the approved wording
```

Fill each unit's `<target>` following the brand guide and glossary, preserving
placeholders; reuse any TM-prefilled targets. Then merge it back, and treat the task
as unfinished until kapi confirms the result:

```bash
kapi merge -i out/*.xliff            # write translations back into the target files + TM
kapi verify --json                   # in a project: brand + terminology + QA in one gate
kapi term-check ./locales/fr.json    # one-off, no project: terminology check on the file
```

`kapi verify` is the gate inside a project — read its findings, fix them, and re-run
until it passes. For a one-off file with no project, `kapi term-check` (plus the QA in
`kapi run translate-qa`) plays the same role. Either way, a clean result, not a
written file, is the finish line.

## Or have kapi call a provider (unattended / CI)

When no assistant is in the loop, kapi can translate via a configured provider.
This needs a saved credential (`kapi credentials add`) or `--api-key`:

```bash
kapi run translate-qa -i ./locales/en.json --target-lang fr --json   # translate + QA
kapi translate ./deck.pptx --target-lang ja -o ./out/deck.ja.pptx
```

`--target-lang` is single-valued, so run one command per locale. A bound brand
profile and termbase still apply. Format is detected from the extension and
written back unchanged (round-trip), preserving structure, tags, and placeholders.

## Bring a project up to date (status → run → review)

In a project, don't translate file by file — converge. State is derived from the
files on every command (like `git status`), so always start by reading it:

```bash
kapi status                  # per-locale coverage + each scope's ship standing
kapi run                     # converge: run the project's default flow over ALL
                             #   content × every target language, in one pass
kapi run --until-gate        # loop the pass until each scope ships or "parks"
```

`kapi run` with no flow name needs `defaults.flow` in the recipe; it materializes
the localized files. Drift is never an error — a behind locale is *pending*, and
work a machine can't finish *parks* (reported, exit 0), so neither blocks you.

Review promotes a translation past `translated` to `reviewed`. The queue and the
approval are two commands:

```bash
kapi status --review         # translated units awaiting a human
# approve one — record its source→target as a correction (the review record):
kapi apply <<<'{"kind":"tm","source":"Save","target":"Lagre","source_locale":"en","target_locale":"nb"}'
```

An approved correction lands in the project TM, counts the unit as `reviewed`,
and the next `kapi run` recycles it for free. `kapi verify --ship` is the opt-in
release bar (the `ship_gate` / `source_gate`); plain `kapi verify` stays
non-blocking about coverage.

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
kapi translate ./report.docx --target-lang fr -o ./out/report.fr.docx
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
