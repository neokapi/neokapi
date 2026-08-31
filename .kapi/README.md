# The dogfood context graph

This directory is the committed context graph of the root `kapi.yaml` recipe:
the voice profile, terms and reviewed content memory neokapi translates its own
surfaces with. It is tracked in git, reviewed like any other source, and `kapi
up` compiles it into the project store on every run. Everything derived from it
lives in `.kapi/work/`, which is gitignored.

- `voice.yaml`: the machine-readable encoding of
  [docs/internals/brand-communication.md](../docs/internals/brand-communication.md),
  bound project-wide via `defaults.voice`. Keep the two in sync.
- `profiles/<name>/voice.yaml`: a voice profile per product where the
  project-wide one does not apply. `kapi.yaml` binds
  `profiles.bowrain.voice` to `profiles/bowrain/voice.yaml`; a collection whose
  channel names no profile falls back to `defaults.voice` above.
- `terms.json`: terminology decisions per target locale (currently Norwegian
  Bokmål, `nb`): a concept per decision with `en` + `nb` terms, domain,
  definition/usage note, and status. Bound by `defaults.terms_source`. It is
  both a source and a destination: the workspace's approved term decisions are
  merged back into it by the nightly's concept pull, upsert-only, so a concept
  it does not mention survives.
- `state/*.jsonl`: the committed decision record. `kapi commit` writes here
  from what a pull staged: one shard per scope, a row per unit carrying its
  review state and the hash of the target it applies to. This is where a
  reviewer's approval is recorded, and it is the only file under this directory
  that a human never writes by hand.
- `memory/<surface>-<lang>.memory.json`: reviewed pairs, one bundle per
  surface and locale (e.g. `builtins-nb.memory.json`). They are **read-only
  accelerants**: `kapi up` compiles every bundle here into the project store so
  a checkout with no credentials converges from git alone, and nothing in the
  loop writes back to them. Bundle filenames are their own naming, independent
  of the collection names in `kapi.yaml`: the compile globs the directory, and
  nothing maps a file name to a collection. Several deliberately differ:
  `builtins-nb` accelerates the `neokapi-engine` collection and `cli-nb` the
  `neokapi-cli` collection, while `libraries-nb` backs the shared frontend
  libraries, which have no collection of their own.

  The docs sites have one bundle each: `docs-nb.memory.json` for the kapi site
  (collection `neokapi-docs`) and `bowrain-docs-nb.memory.json` for the bowrain
  site (collection `bowrain-docs`); the terms store is shared. The bowrain UIs
  have one per surface, `bowrain-app-nb.memory.json` (the shared SPA in
  `bowrain/packages/{app,ui}` plus the web and desktop shells),
  `bowrain-ctrl-nb.memory.json` and `bowrain-pulse-nb.memory.json`, compiled
  into each shell's committed `public/translations/nb.json`.
  `desktop-nb.memory.json` backs the `neokapi-desktop` collection and
  `demo-narration-nb.memory.json` the `neokapi-demos` sidecars.
  `libraries-nb.memory.json` covers the shared frontend libraries
  (`packages/ui`, `packages/flow-editor`) whose strings reach the desktop app
  through the `neokapi-desktop` extraction. The transactional emails
  (collections `bowrain-email` + `bowrain-email-subjects`) use
  `emails-nb.memory.json`, and the landing page (`bowrain-landing`) uses
  `landing-nb.memory.json`.

The bundles are committed in the **native form**: deterministic, lossless JSON
that preserves entry identity. TMX/CSV/TBX are the lossy interchange tier;
`make l10n-review-export` writes disposable renderings under `l10n/review/`
(gitignored) for a human who wants one, and nothing reads them back.

No bundle names a Makefile target, because none of them needs one: `make l10n`
runs one `kapi up` over the whole recipe, so every collection is covered by
construction. See [docs/internals/l10n-ci.md](../docs/internals/l10n-ci.md).

## Where wording is decided

Not here. A correction to a translated string is a **decision**, and decisions
travel one path:

1. Fix it where it is read: the review queue on the server, or `kapi apply`
   locally for a term edit (which writes `terms.json` and recompiles it).
2. `kapi up` pulls the approved decisions into the project store on the next
   run, and `kapi commit` writes them into `.kapi/state/`, which git tracks.
3. The next convergence materializes the target from a store that now holds the
   approval.

Editing a `.memory.json` bundle to change a rendered string records a decision
nobody made, in the one place the loop treats as an input rather than an output.
`scripts/check-sync-backed.sh` refuses a sync that removed a derived artifact
with no `.kapi/` change behind it, and hand-editing a bundle to clear that gate
manufactures the backing rather than supplying it.

What legitimately lands in a bundle is a **new** reviewed pair for a string that
has none: a translation done outside the loop, folded in by importing the
bundle plus the new pairs into a scratch content memory and exporting the result
(`kapi memory export -o .kapi/memory/<surface>-<lang>.memory.json`). Everything
below is what such an entry has to satisfy.

## Bundle authoring rules

Markup tokens must be **run-structured, never literal text**. A KBF
runtime-projection token like `{=m0}`/`{/=m0}` is format-specific markup; baked
into a variant's text it leaks verbatim into every other surface that happens to
share the words (the docs "`{=m0} Installer`" class of bug). Store such tokens
as real inline-code runs (`ph` for a standalone element, `pcOpen`/`pcClose` for
a paired one, with the literal token text as `data`) in **both** the source and
target variants, so the content memory matches them structurally (same code
structure scores 1.0; a bare-text lookalike caps below it) and `recycle` fills
targets with the entry's runs, tokens intact.

Named parameters follow the **reader**, not a rule of thumb. Where the reader
lifts `{count}` out of the text into a `ph` run (as the JSX extraction does for
*every* `{expr}` child, emitting `ph` with `type: "jsx:var"` and `data:
"{count}"`), the entry must carry that same `ph` run in both variants. Only
where the reader leaves the braces in the text (a plain JSON or gettext surface,
where `{count}` is just characters) is it literal text. Storing a lifted
parameter as literal text produces exactly the asymmetry the next paragraph
forbids.

`kapi memory import` warns about entries whose variants disagree on their token
sets, literal `{=mN}` markup **or** inline-code runs; a clean import (no
warnings) is the gate. Where a plain-text surface legitimately shares the words
with a token-bearing UI string, keep a separate plain entry (the `…-plain`
companion entries) rather than reusing the structured one.

**A target variant must carry exactly the inline codes its source variant
carries.** `recycle` enforces this at fill time: a match whose target dropped a
placeholder is recorded as a review candidate but never written, so the locale
falls back to source rather than shipping a sentence with a hole where a count,
a name or a link belongs. The practical consequence for an author: an asymmetric
entry is not "partial credit", it is zero coverage for that string. When a
source string gains or renumbers a code (an extractor change, or new markup in
the JSX), the entry stops matching structurally and the string falls back to
English until the entry is re-anchored to the new source runs, visible as a
coverage drop, never as a corrupted string.

Why an asymmetric entry is *dangerous* rather than merely useless:
content-memory leverage has a plain tier that keys on the **flattened** source,
and inline codes contribute no characters to that flattening. So a code-free
entry is a near-exact match for a code-bearing source:
`[ph{documentedCount}][" documented formats"]` flattens to `" documented
formats"` and matches a plain `"documented formats"` entry at ~100%, and
filling it wrote a target with the placeholder simply gone. Nothing downstream
objected: target-language drift never fails the build, so the corrupted string
shipped and only the coverage number moved. Three layers stop it: `kapi
memory import` warns on the asymmetric entry, `recycle` refuses the fill, and
`placeholder-check` reports a critical finding if one ever reaches a target,
all keyed off the one predicate, `model.DiffRunCodes`. `make l10n-report` counts
what is left.

## Entries whose source string is gone

An entry outlives the string that motivated it. Source copy is rewritten, a
screen is redesigned, a fixture leak is plugged, and the reviewed pair
that translated the old wording stops matching anything. Such an entry is
**kept, not deleted**: review is the expensive part, and a string that comes
back should come back already translated. Deleting on every source change would
make the bundles a projection of today's source, which is what the derived
catalogs already are.

Keeping them is only safe while they are visible, because content memory is
keyed by **text, not by call site**. An entry with no source left is not inert:
the moment any surface anywhere in the recipe emits the same English string,
`recycle` fills it from this entry as reviewed wording. An entry carrying a
spelling the project has since retired is therefore a spelling waiting to be
re-approved somewhere else.

So each cycle reports them. `make l10n-orphans` runs the loop and then asks, of
every entry, whether its target text appears anywhere in the artifacts the
convergence materialized for that locale; `make l10n-orphans-report` asks the
same question over targets already on disk, for a walk that has just run the
loop and should not pay for it twice. It reports and never gates: an entry
producing nothing is pending or finished work, not a build break.

Reading the list is a review task, not a cleanup task. A large count usually
means a *source* change outran its translations rather than that anything is
wrong: when a source string gains or renumbers an inline code, its entry stops
matching structurally and shows up here until it is re-anchored. Retire an entry
only when its wording should never come back.

## Targets the source moved away from

The same rewrite lands differently on the other kind of catalog, and that one is
not drift. `core/i18n/catalogs/<lang>.json` and `host/i18n/catalogs/<lang>.json`
are addressed by **scope path**, not by source text, so a reworded string keeps
its scope and the previous translation stays attached to it. Nothing falls back:
the locale ships a translation of a sentence the source no longer contains, and
it is indistinguishable from a current one, the opposite of the drift the loop
is built to tolerate.

Two things narrow it. The record absorber refuses a pairing whose target does not
carry the source's placeholders, in either spelling, so a rewrite that moved a
parameter is not taught to the corpus as reviewed wording. And
`make l10n-stale-report` reads the rest out of git: for each translated entry,
the commit its text last changed at is the commit it was produced at, and the
source document there is the wording it translates. It reports and never gates:
re-translate the entry where it is reviewed, or remove it so the locale falls
back to English until the next convergence writes it again.

## Why not PO files? (decided, not overlooked)

The Go-surface catalogs (builtins, CLI help) are standard gettext at runtime
(embedded MO, msgid = English source, msgctxt = scope), and the repository
carries a per-locale catalog for them, but as JSON in the shape of its source
document, not as `.po`. The reviewable-text half of PO's argument is answered:
`core/i18n/catalogs/<lang>.json` and `host/i18n/catalogs/<lang>.json` diff line
by line, and the MO is compiled from them at build time. What is left of PO's
value is its translator-facing ecosystem (Poedit, Weblate, community PRs);
today's translator population is the maintainer plus kapi-driven agents, so
committed PO would be a second translation workflow with no audience, and a
second file format for the same content, one the recipe could not write from a
JSON source without a conversion nobody asked for.

If external locale contributions become a goal, PO enters **through the project,
not beside it**: `kapi extract --format po` emits the bilingual files for a
translator and `kapi merge -i` applies them back. There is exactly one loop, and
PO is an interchange format of it, never a parallel gettext workflow (no
committed `po/` tree, no msgmerge).

## What is committed where (and why)

Multilingual artifacts in git fall into four tiers; everything else is
gitignored ephemera (`.kapi/work/`, extraction batches, `i18n-*/` intermediates,
`l10n/review/`).

1. **Source: human-owned.** The English strings in React and Go source, the
   English harness narration in `harness/demos/*/demo.yaml`, and the context
   graph in this directory: the memory bundles, `terms.json`, `voice.yaml`. Nothing
   regenerates them.

2. **Build-derived: machine-owned, byte-gated.** A function of committed source
   alone: the `commands.json`/`metadata.json` inventories, the English email
   renders (`bowrain/mailer/templates/*.html`), and the whole `qps` probe tier
   (`core/i18n/catalogs/qps.json`, `bowrain/mailer/subjects/qps.json`, each
   surface's `translations/qps.json`, `bowrain/web/landing/head/qps.json`,
   `bowrain/mailer/templates/qps/`). `make l10n-verify` regenerates all of it,
   with no convergence in the walk, and fails on any byte drift; in CI that is the `l10n` workflow, which on a
   same-repo pull request commits the regeneration back instead of failing.
   Never hand-edit.

3. **Loop-owned: written by `kapi up`, gated on content rather than bytes.** The
   target-language tier: the Go-surface catalogs
   (`core/i18n/catalogs/<lang>.json`, `host/i18n/catalogs/<lang>.json`), the
   frontend runtime dictionaries (`public/translations/<lang>.json`), the
   subject catalogs, the rendered per-locale email templates, the landing
   runtime catalogs, and the demo narration sidecars
   (`harness/demos/*/demo.<lang>.yaml`). Committed because the apps ship them as
   static assets and because a catalog is what a reviewer reads. They come out
   of the project store, which holds the union of what git carries and what a
   venue pull brought home, so no byte gate can be laid over them: a checkout
   with no server cannot reproduce an approval made on one, and a gate that
   demanded it would overwrite the approval. Their coverage is reported instead
   (`make l10n-report`) and never gated.

   Their **content** is gated. Reproducibility cannot be asserted about this
   tier; soundness can, and it is a different question: an artifact must parse,
   must carry exactly the placeholders its source carries, and must not have
   translated a machine identifier the recipe never declared translatable.
   `scripts/check-derived-content.mjs` reads that: `make l10n-content-check`
   over the committed tier, `scripts/check-sync-backed.sh` over what a run wrote.
   `l10n-collapse-check` still asserts existence: a catalog that carried entries
   at HEAD may not regenerate to empty. Never hand-edit, though a defective
   entry may be **removed**, because a wrong translation is not an approved one
   and falling back to source is the correct pending state.

   The compiled MO the Go binaries embed is **not** committed: `make
   i18n-catalogs` produces `<lang>.mo` from the `<lang>.json` beside it, ahead
   of anything that compiles `core/i18n` or `host/i18n`. It is pure Go in the
   framework module and links neither package it writes for, so there is no
   bootstrap cycle and no kapi binary in the loop. A binary catalog in git is
   unreviewable; that is how a pull once wrote JSON bytes into the `.mo` files
   with nothing in the diff to see.

4. **Materialized targets: generated, never committed.** The translated docs
   pages for both Docusaurus sites, and the per-surface `i18n-<lang>/`
   intermediates the compilers read. Gitignored build artefacts, because
   committing the docs pages made every source-doc edit go stale and hard-fail
   the nb build (see CLAUDE.md "Target-language drift must never block the
   build"). Content-memory misses fall back to English, and both sites build
   with the tree absent.
