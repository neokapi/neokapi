# The dogfood context graph

This directory is the committed context graph of the root `kapi.yaml` recipe —
the voice profile, terms and reviewed content memory neokapi translates its own
surfaces with. It is tracked in git, reviewed like any other source, and it is
the truth. Everything derived from it lives in `.kapi/work/`, which is
gitignored and rebuilt with `make l10n-seed`.

Seeds are committed in the **native bundle form** — deterministic,
lossless JSON that preserves entry identity, so a wipe-and-reseed
reproduces the content memory and terms state exactly (`.memory.json` and
`.terms.json`). TMX/CSV/TBX are the lossy interchange tier; emit
disposable review views with `make l10n-review-export` (→ `l10n/review/`,
gitignored).

- `voice.yaml` — the machine-readable encoding of
  [docs/internals/brand-communication.md](../docs/internals/brand-communication.md),
  bound project-wide via `defaults.voice`. Keep the two in sync.
- `terms.json` — terminology decisions per target locale (currently
  Norwegian Bokmål, `nb`): concept per decision with `en` + `nb` terms,
  domain, definition/usage note, and status. Imported into the project store,
  `.kapi/work/store.db`.
- `memory/<surface>-<lang>.memory.json` — reviewed translations, one file per
  surface and locale (e.g. `builtins-nb.memory.json`). Imported into
  the same store; every target-locale output is produced from the content memory
  by `recycle`, so generated catalogs only ever contain reviewed strings.
  The docs sites have one seed each: `docs-nb.memory.json` for the kapi site
  (collection `neokapi-docs`: `web/docs/**` → `web/i18n/nb/...`) and
  `bowrain-docs-nb.memory.json` for the bowrain site (collection
  `bowrain-docs`: `bowrain/web/docs/docs/**` → `bowrain/web/docs/i18n/nb/...`);
  the terms store is shared. The bowrain UIs have one seed per surface —
  `bowrain-app-nb.memory.json` (the shared SPA in `bowrain/packages/{app,ui}`
  plus the web and desktop shells), `bowrain-ctrl-nb.memory.json` and
  `bowrain-pulse-nb.memory.json` — compiled into each shell's committed
  `public/translations/nb.json`. `libraries-nb.memory.json` covers the shared
  frontend libraries (`packages/ui`, `packages/flow-editor`) whose strings
  reach the desktop app through the `neokapi-desktop` extraction; it backs no
  collection of its own. The transactional emails (collections
  `bowrain-email` + `bowrain-email-subjects`) use `emails-nb.memory.json`, and
  the landing page (`bowrain-landing`) uses `landing-nb.memory.json`.

  No seed names a Makefile target, because none of them needs one: `make l10n`
  runs one kapi pass over the whole recipe, so every collection is covered by
  construction. See
  [docs/internals/l10n-ci.md](../docs/internals/l10n-ci.md).

  Seed filenames are their own naming, independent of the collection names in
  `kapi.yaml`: `l10n-seed` imports every `.kapi/memory/*.memory.json` by
  glob, and nothing maps a file name to a collection. Several deliberately
  differ — `builtins-nb` seeds the `kapi-engine` collection, and `cli-nb` and
  `libraries-nb` back surfaces that have no collection at all.

Workflow for a new or changed surface string:

1. Translate it (human, or `kapi translate` with credentials — the voice
   profile and terms are bound project-wide) and merge the pair
   into the surface's seed: import the seed plus the new pairs (any
   supported form, e.g. a small TMX) into a scratch content memory, then
   `kapi memory export -o .kapi/memory/<surface>-<lang>.memory.json`. Small wording
   fixes can also be edited directly in the `.memory.json` bundle — it is the
   source of truth.
2. `make l10n` to rebuild the store from the seeds and regenerate every
   surface from it. To iterate on one surface without the full pass, scope the
   same command the loop runs: `kapi run tm-recycle -i <path> --target-lang nb`.
3. `kapi check` runs the bound terminology and voice checks over the
   result.

Review happens in the seeds — they are the human-owned artifact. For a
reviewer-friendly view, `make l10n-review-export` writes TMX/CSV renderings
under `l10n/review/`; corrections still land in the `.memory.json`/`.terms.json`
seeds.

## Seed authoring rules

Markup tokens must be **run-structured, never literal text**. A KBF
runtime-projection token like `{=m0}`/`{/=m0}` is format-specific markup;
baked into a variant's text it leaks verbatim into every other surface
that happens to share the words (the docs "`{=m0} Installer`" class of
bug). Store such tokens as real inline-code runs — `ph` for a standalone
element, `pcOpen`/`pcClose` for a paired one, with the literal token text
as `data` — in **both** the source and target variants, so the content memory matches
them structurally (same code structure scores 1.0; a bare-text lookalike
caps below it) and `recycle` fills targets with the entry's runs,
tokens intact.

Named parameters follow the **reader**, not a rule of thumb. Where the reader
lifts `{count}` out of the text into a `ph` run — as the JSX extraction does
for *every* `{expr}` child, emitting `ph` with `type: "jsx:var"` and
`data: "{count}"` — the seed must carry that same `ph` run in both variants.
Only where the reader leaves the braces in the text (a plain JSON or gettext
surface, where `{count}` is just characters) is it literal text. Storing a
lifted parameter as literal text produces exactly the asymmetry the next
paragraph forbids.

`kapi memory import` warns about entries whose variants disagree
on their token sets — literal `{=mN}` markup **or** inline-code runs; a clean
import (no warnings, `make l10n-seed`) is the gate. Where a plain-text
surface legitimately shares the words with a token-bearing UI string, keep a
separate plain entry (the `…-plain` companion entries) rather than reusing
the structured one.

**A target variant must carry exactly the inline codes its source variant
carries.** `recycle` enforces this at fill time: a match whose target dropped
a placeholder is recorded as a review candidate but never written, so the
locale falls back to source rather than shipping a sentence with a hole where
a count, a name or a link belongs. The practical consequence for a seed
author: an asymmetric entry is not "partial credit", it is zero coverage for
that string. When a source string gains or renumbers a code (an extractor
change, or new markup in the JSX), the seed entry stops matching structurally
and the string falls back to English until the entry is re-anchored to the
new source runs — visible as a coverage drop, never as a corrupted string.

Why an asymmetric entry is *dangerous* rather than merely useless:
content-memory leverage has a plain tier that keys on the **flattened** source,
and inline codes contribute no characters to that flattening. So a code-free
entry is a
near-exact match for a code-bearing source — `[ph{documentedCount}]["
documented formats"]` flattens to `" documented formats"` and matches a plain
`"documented formats"` entry at ~100% — and filling it wrote a target with the
placeholder simply gone. Nothing downstream objected: target-language drift
never fails the build, so the corrupted string shipped and only the coverage
number moved. Three layers now stop it — `kapi memory import` warns on the
asymmetric entry, `recycle` refuses the fill, and `placeholder-check` reports a
critical finding if one ever reaches a target — all keyed off the one
predicate, `model.DiffRunCodes`.

## Why not PO files? (decided, not overlooked)

The Go-surface catalogs (builtins, CLI help) are standard gettext at
runtime — embedded MO, msgid = English source, msgctxt = scope — and the
repository carries a per-locale catalog for them, but as JSON in the shape of
its source document, not as `.po`. The reviewable-text half of PO's argument is
answered: `core/i18n/catalogs/<lang>.json` and `host/i18n/catalogs/<lang>.json`
diff line by line, and the MO is compiled from them at build time. What is left
of PO's value is its translator-facing ecosystem (Poedit, Weblate, community
PRs); today's translator population is the maintainer plus kapi-driven agents,
so committed PO would be a second translation workflow with no audience — and
a second file format for the same content, one the recipe could not write from
a JSON source without a conversion nobody asked for.

If external locale contributions become a goal, PO enters **through the
project, not beside it**: `kapi extract --format po` emits the bilingual
files for a translator and `kapi merge -i` applies them back (updating the
content memory, which updates the seeds). There is exactly one translation loop —
seeds → content memory → extract/merge — and PO is an interchange format of that loop,
never a parallel gettext workflow (no committed `po/` tree, no msgmerge).

## What is committed where (and why)

Translation artifacts in git fall into three tiers; everything else is
gitignored ephemera (`.kapi/work/`, extraction batches, `i18n-*/`
intermediates, `l10n/review/`).

1. **Source — human-owned.** The seeds here (`memory/*.memory.json`,
   `terms.json`, `voice.yaml`), the Docusaurus theme JSONs under
   `web/i18n/<locale>/`, and the English harness narration in
   `harness/demos/*/demo.yaml`. Tooling may have written the first
   draft, but humans own the content; nothing regenerates them.
   Corrections land here.
2. **Committed-generated — machine-owned, drift-gated.** The Go-surface
   catalogs (`core/i18n/catalogs/<lang>.json`,
   `host/i18n/catalogs/<lang>.json`), the `commands.json`/`metadata.json`
   inventories, the frontend runtime catalogs
   (`public/translations/<locale>.json`), the demo narration sidecars
   (`harness/demos/*/demo.<lang>.yaml`), the rendered email templates and
   subject catalogs (`go:embed`ed into the server), and the landing runtime
   catalogs (the web-landing workflow builds the nb variant from them without a
   kapi toolchain). Committed because the apps ship them as static assets, and
   because a catalog is what a reviewer reads. `make l10n-verify` regenerates
   all of it and fails on any byte drift; in CI that is the `l10n` workflow,
   which on a same-repo pull request commits the regeneration back instead of
   failing. Never hand-edit.

   The compiled MO the Go binaries embed is **not** in this tier and not in
   git: `make i18n-catalogs` produces `<lang>.mo` from the `<lang>.json` beside
   it, ahead of anything that compiles `core/i18n` or `host/i18n`. It is pure
   Go in the framework module and links neither package it writes for, so there
   is no bootstrap cycle and no kapi binary in the loop. A binary catalog in
   git is unreviewable — that is how a pull once wrote JSON bytes into the
   `.mo` files with nothing in the diff to see.
3. **Materialized targets — generated, never committed.** The translated
   docs pages for both Docusaurus sites. Derived from source + content memory;
   gitignored build artefacts, because committing them made every source-doc
   edit go stale and hard-fail the nb build (see CLAUDE.md "Target-language
   drift must never block the build"). Content memory misses fall back to
   English, and both sites build with the tree absent. Corrections land in the
   seeds, never in the pages. Nothing gates them: coverage is reported to the
   `l10n` workflow's job summary and never fails it, because pending target
   work is the normal state, not an error.
