# Translate, enforce terminology, publish

Translate content, enforce terminology, and round-trip the result back into its
original format with the local `kapi` CLI. For ongoing work, bind the locales,
voice profile, and terms in a project first; see [project.md](project.md).

## First decide: one-off file, or a project?

**A single file you just need translated** (a document, a deck, one catalog):
translate it directly. There is nothing to set up. `kapi extract` with no `-o`
reads a *project's* content config (on a loose file it fails with "no .kapi
project found"); the ad-hoc form `kapi extract <files> -o work.kpz --target-lang
<lang>` builds a `.kpz` workspace instead and needs no project. For one file,
just round-trip it:

```bash
kapi pseudo-translate <file> --target-lang qps        # quick readiness pre-flight
kapi translate <file> --target-lang <lang> -o <out>  # reads the format, translates, writes it back
```

kapi preserves structure, tags, and placeholders (round-trip). Add `--credential
<name>` when it needs a model provider. That's the whole task for a one-off file.

**Ongoing work (a whole app, the same locales repeatedly), or translating it
yourself under voice + terminology guardrails**: bind a project first
(`kapi init`), then `kapi extract → fill the targets → kapi merge →
kapi check --ship` (below). Inside a project, `kapi extract` and `kapi merge`
operate on the project's content; run them inside one (or with `-p <recipe>`).

## Commands at a glance (use these exact forms)

Run these as written; don't guess flags. When in doubt, `kapi <cmd> --help`.

```bash
kapi extract --target-lang fr                  # → out/<name>.en-to-fr.xliff (comma-separate for several locales)
kapi merge -i out/*.xliff                       # XLIFF/PO come via -i (repeatable); only a .kpz may be positional
kapi check --ship --json                              # the gate: voice + terminology + rule-based checks in one shot (prefer this)
kapi exec term-check ./locales/fr.json --target-lang fr   # file is POSITIONAL; there is no --source/--target
kapi terms lookup "board" -t fr              # approved wording; terms uses -s/-t, not --*-lang
kapi voice guide                                # the voice to follow (no flag inside a project)
```

Inside a project, prefer `kapi check --ship` over running `term-check` or the
rule-based checks by hand: it runs every bound gate together and pairs
source↔target for you.

## Translate the content yourself, through kapi (don't hand-translate files)

Translate the content yourself (no provider needed), but route the
translation **through kapi** so the guardrails actually apply. Don't read the source
file, translate it in your head, and write the target file directly: that quietly
skips terminology, placeholder and format integrity, and the voice profile, the very
things kapi exists to enforce, and the things a human reviewer will later hold you to.
Instead, let kapi pull out the text and the rules, do the translating, and let kapi
write it back. (Inside a project, the kapi Claude Code plugin enforces this with a
PreToolUse hook that blocks direct edits to generated target files; route the
change through the round-trip below, or edit the source.)

```bash
kapi extract --target-lang fr        # bilingual file with source + empty targets (out/*.xliff)
kapi voice guide                     # the voice to follow (no flag inside a project)
kapi terms lookup "<term>" -t fr  # the approved wording
```

Fill each unit's `<target>` following the voice guide and the approved terminology,
preserving placeholders; reuse any targets prefilled from content memory. Then merge
it back, and treat the task as unfinished until kapi confirms the result:

```bash
kapi merge -i out/*.xliff            # write translations back into the target files + content memory
kapi check --ship --json                   # in a project: voice + terminology + rule-based checks in one gate
kapi exec term-check ./locales/fr.json --termstore <store>   # one-off, no project: name the terms store
```

`kapi check --ship` is the gate inside a project: read its findings, fix them, and re-run
until it passes. For a one-off file with no project, `kapi exec term-check` (plus the
checks in `kapi run translate-qa`) plays the same role. Either way, a clean result, not a
written file, is the finish line.

## Or have kapi call a provider (unattended / CI)

When no assistant is in the loop, kapi can translate via a configured provider.
This needs a saved credential (`kapi credentials add`) or `--api-key`. The two
are not interchangeable when the provider is self-hosted: `--base-url` belongs
to the saved credential and applies only on the `--credential` route, because an
inline `--api-key` resolves before an endpoint is attached and calls the public
host.

```bash
kapi run translate-qa -i ./locales/en.json --target-lang fr --json   # translate + checks
kapi translate ./deck.pptx --target-lang ja -o ./out/deck.ja.pptx
```

`--target-lang` is single-valued, so run one command per locale. A bound voice
profile and terms still apply. Format is detected from the extension and
written back unchanged (round-trip), preserving structure, tags, and placeholders.

When a source block has changed since it was last translated, `translate` sends
the block's own prior approved translation to the model as reference
(`--reuse prior`, the default), so the model revises the settled wording rather
than starting over; `--reuse none` turns that off. Fuzzy memory matches are
never sent to the model; `recycle` applies them before translation.

## Bring a project up to date (status → up → review)

In a project, don't translate file by file; bring the whole project up to date.
State is derived from the files on every command (like `git status`), so always
start by reading it:

```bash
kapi status                  # per-locale coverage + each scope's ship standing
kapi up                      # catch up: loop the project's default flow over ALL
                             #   content × every target language until each scope
                             #   ships or "parks"; runs locales concurrently
kapi up --plan               # dry run: pending work, content-memory leverage, token estimate
kapi up --json               # NDJSON event stream (one event per line, final
                             #   record = the result); use this to drive the loop
```

`kapi up` is the one verb that runs the loop. With no `defaults.flow` in the recipe it
runs the built-in default flow (content memory recycle → AI translate) and materializes the
translated files. Drift is never an error: a behind locale is *pending*, and work
a machine can't finish *parks* (reported, exit 0), so neither blocks you. Use
`--json` for the machine-readable event stream; the `up` and `up_plan` MCP tools
expose the same loop and dry run to an assistant. `kapi run <flow>` is only for a
*custom* one-off pipeline (one named flow, one pass); the daily loop is `kapi up`.

In a server-connected project (recipe has a `bowrain:` block), `kapi up` runs on
the Bowrain server by default and streams progress back; `kapi up --local`
runs the loop on this machine and pushes the results. `kapi push` / `kapi pull` are
**transport only**: they move project state and never translate. There is no
`kapi sync`.

Review promotes a translation past `translated` to `reviewed`. The queue and the
approval are two commands:

```bash
kapi status --review         # translated units awaiting a human
# approve one: a `review` change-set addressed by the unit's file/id/locale
kapi apply <<<'{"kind":"review","file":"src/nb.json","id":"save.label","locale":"nb","status":"reviewed"}'
```

The unit state lands in the project store and counts the unit as `reviewed`,
so the next `kapi up` sees it shipped. `kapi check --ship` is the opt-in release
bar: it runs the project's voice, terminology and rule-based gates plus the
`ship_gate` / `source_gate` coverage gates and exits non-zero only when you ask
for it; ordinary target drift never blocks.

## Keep terminology consistent

```bash
kapi terms import terms.csv --format csv -s en -t fr --local   # also: json, tbx
kapi terms lookup "checkout" -s en -t fr --json
kapi exec term-check ./locales/fr.json --json                            # flag wrong/missing terms
kapi terms occurrences "checkout"                                        # where the term (or a concept id) is used in the extracted content
```

`occurrences` reads the project's block cache, so the project must have been
extracted (`kapi up` or `kapi extract`); a concept id is searched under every
term it carries, in every language.

Use the approved (preferred) term; avoid deprecated/forbidden ones. A bound
terms store also feeds the translation step, and so does a `term_rules:` list in
the translate step's config (one term, its replacement, a severity), the same
shape the voice profile's `vocabulary:` writes.

## Publish (format round-trip)

```bash
kapi formats --json                 # what reads and writes
kapi stats ./report.docx --json          # translatable word/segment count
```

Direct round-trip, or a bilingual extract → translate → merge cycle for vendor
or human translation:

```bash
kapi translate ./report.docx --target-lang fr -o ./out/report.fr.docx
kapi extract -p kapi.yaml --target-lang fr --format xliff2          # emit XLIFF
kapi merge -i ./out/*.fr.xlf -p kapi.yaml                          # merge back
```

Native readers/writers cover document, data, catalog, and office formats,
offline, with no plugin. This includes mobile/app catalogs (Apple String Catalog
`.xcstrings`, `.strings`/`.stringsdict`, Android `strings.xml`, Flutter `.arb`,
i18next JSON, `.resx`) and content formats like Markdown and MDX. A few
specialized or legacy formats are available through the okapi-bridge; map an
extension to a bridge format on `kapi exec <tool>` with
`--map '*.sdlppx=okf_sdlpackage'`. When the bridge is installed it can
shadow a shared extension (e.g. `.strings`, `.xml`, `.resx`); pass
`--format <name>` to force the native reader.

## How to apply

1. Confirm the format reads **and** writes (`kapi formats`); for write-limited
   formats (e.g. PDF is read-only), extract to a bilingual format instead.
2. Bind a voice profile + terms so output is on-brand and consistent.
3. Pre-flight with `kapi pseudo-translate <file> --target-lang qps` to surface
   hardcoded or untranslated strings before real translation.
