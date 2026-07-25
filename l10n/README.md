# Dogfood localization seeds

This directory holds the committed inputs for localizing neokapi's own
surfaces with kapi (the root `kapi.yaml` recipe). The `.kapi/` state
directory is gitignored and rebuilt from these seeds with `make l10n-seed`.

Seeds are committed in the **native Kapi-family forms** — deterministic,
lossless JSON that preserves entry identity, so a wipe-and-reseed
reproduces the TM/termbase state exactly (see `sievepen/kmb` and
`termbase/ktb`). TMX/CSV/TBX are the lossy interchange tier; emit
disposable review views with `make l10n-review-export` (→ `l10n/review/`,
gitignored).

- `brand-voice.yaml` — the machine-readable encoding of
  [docs/internals/brand-communication.md](../docs/internals/brand-communication.md),
  bound project-wide via `defaults.brand_voice`. Keep the two in sync.
- `termbase.ktb` — terminology decisions per target locale (currently
  Norwegian Bokmål, `nb`): concept per decision with `en` + `nb` terms,
  domain, definition/usage note, and status. Imported into
  `.kapi/termbase.db`.
- `tm/<surface>-<lang>.kmb` — reviewed translations, one file per
  surface and locale (e.g. `builtins-nb.kmb`). Imported into
  `.kapi/tm.db`; every localized output is produced from the TM by
  `recycle`, so generated catalogs only ever contain reviewed strings.
  The docs sites have one seed each: `docs-nb.kmb` for the kapi site
  (surface `docs`: `web/docs/**` → `web/i18n/nb/...`, `make l10n-docs`)
  and `bowrain-docs-nb.kmb` for the bowrain site (surface
  `docs-bowrain`: `bowrain/web/docs/docs/**` →
  `bowrain/web/docs/i18n/nb/...`, `make l10n-bowrain-docs`); the
  termbase is shared. The bowrain UIs have one seed per surface —
  `bowrain-app-nb.kmb` (surface `bowrain-app-ui`: the shared SPA in
  `bowrain/packages/{app,ui}` + web/desktop shells,
  `make l10n-bowrain-app`), `bowrain-ctrl-nb.kmb` and
  `bowrain-pulse-nb.kmb` (`make l10n-bowrain-{ctrl,pulse}`) — compiled
  into each shell's committed `public/translations/nb.json`.
  `libraries-nb.kmb` covers the shared frontend libraries
  (`packages/ui`, `packages/flow-editor`) whose strings reach the desktop
  app through the `kapi-desktop-ui` extraction — it has no Make target of
  its own; `l10n-desktop` leverages it. The transactional emails
  (surfaces `emails` + `email-subjects`: `bowrain/emails/src` templates
  and `bowrain/mailer/subjects/en.json` → per-locale renders under
  `bowrain/mailer/`, `make l10n-emails`) use `emails-nb.kmb`; the
  bowrain landing page (surface `landing`: `bowrain/web/landing/src` →
  committed `bowrain/web/landing/translations/<lang>.json`,
  `make l10n-landing`) uses `landing-nb.kmb`.

Workflow for a new or changed surface string:

1. Translate it (human, or `kapi translate` with credentials — the brand
   voice profile and termbase are bound project-wide) and merge the pair
   into the surface's seed: import the seed plus the new pairs (any
   supported form, e.g. a small TMX) into a scratch TM, then
   `kapi tm export -o l10n/tm/<surface>-<lang>.kmb`. Small wording fixes
   can also be edited directly in the `.kmb` JSON — it is the source of
   truth.
2. `make l10n-seed` to rebuild the TM, then the surface target
   (e.g. `make l10n-builtins`, or `make l10n` for everything).
3. `make l10n-builtins-check` runs the terminology gate (`kapi term-check`)
   over the result.

Review happens in the seeds — they are the human-owned artifact. For a
reviewer-friendly view, `make l10n-review-export` writes TMX/CSV renderings
under `l10n/review/`; corrections still land in the `.kmb`/`.ktb`.

## Seed authoring rules

Markup tokens must be **run-structured, never literal text**. A KBF
runtime-projection token like `{=m0}`/`{/=m0}` is format-specific markup;
baked into a variant's text it leaks verbatim into every other surface
that happens to share the words (the docs "`{=m0} Installer`" class of
bug). Store such tokens as real inline-code runs — `ph` for a standalone
element, `pcOpen`/`pcClose` for a paired one, with the literal token text
as `data` — in **both** the source and target variants, so the TM matches
them structurally (same code structure scores 1.0; a bare-text lookalike
caps below it) and `recycle` fills targets with the entry's runs,
tokens intact. Named parameters like `{count}` are not markup and stay
literal text. `kapi tm import` warns about entries whose variants disagree
on their token sets; a clean import (no warnings, `make l10n-seed`) is the
gate. Where a plain-text surface legitimately shares the words with a
token-bearing UI string, keep a separate plain entry (the `…-plain`
companion entries) rather than reusing the structured one.

## Why not PO files? (decided, not overlooked)

The Go-surface catalogs (builtins, CLI help) are standard gettext at
runtime — embedded MO, msgid = English source, msgctxt = scope — but the
repository does not carry per-locale `.po` files. PO's value is its
translator-facing ecosystem (Poedit, Weblate, community PRs); today's
translator population is the maintainer plus kapi-driven agents, so
committed PO would be a second translation workflow with no audience.

If external locale contributions become a goal, PO enters **through the
project, not beside it**: `kapi extract --format po` emits the bilingual
files for a translator and `kapi merge -i` applies them back (updating the
TM, which updates the seeds). There is exactly one translation loop —
seeds → TM → extract/merge — and PO is an interchange format of that loop,
never a parallel gettext workflow (no committed `po/` tree, no msgmerge).

## What is committed where (and why)

Localization artifacts in git fall into three tiers; everything else is
gitignored ephemera (`.kapi/` state, extraction batches, `i18n-*/`
intermediates, `l10n/review/`).

1. **Source — human-owned.** The seeds here (`tm/*.kmb`,
   `termbase.ktb`, `brand-voice.yaml`), the Docusaurus theme JSONs under
   `web/i18n/<locale>/`, and the English harness narration in
   `harness/demos/*/demo.yaml`. Tooling may have written the first
   draft, but humans own the content; nothing regenerates them.
   Corrections land here.
2. **Committed-generated — machine-owned, drift-gated.** The embedded MO
   catalogs, `commands.json`/`metadata.json` inventories, the frontend
   runtime catalogs (`public/translations/<locale>.json`), the demo
   narration sidecars (`harness/demos/*/demo.<lang>.yaml`, from
   `make l10n-demos`), the rendered email templates + subject catalogs
   (`bowrain/mailer/templates/<lang>/*.html`,
   `bowrain/mailer/subjects/<lang>.json`, from `make l10n-emails` —
   `go:embed`ed into the server), and the landing runtime catalogs
   (`bowrain/web/landing/translations/<lang>.json`, from
   `make l10n-landing` — the web-landing workflow builds the nb variant
   from them without a kapi toolchain). Committed because `go:embed`
   needs them at build time (regenerating needs a built kapi — a
   bootstrap cycle) and the apps ship them as static assets.
   `make l10n-verify` (CI: the l10n-drift job) fails on any byte drift
   from the seeds; the node-dependent email/landing surfaces are gated
   by `make emails-l10n-verify` / `make landing-l10n-verify` (CI: the
   emails-landing-l10n job). Never hand-edit.
3. **Materialized targets — generated, never committed.** The translated
   docs pages under `web/i18n/<locale>/.../current/` (kapi docs site,
   `make l10n-docs`) and `bowrain/web/docs/i18n/<locale>/.../current/`
   (bowrain docs site, `make l10n-bowrain-docs`). Derived from source +
   TM; gitignored build artefacts, because committing them made every
   source-doc edit go stale and hard-fail the nb build (see CLAUDE.md
   "Target-language drift must never block the build"). TM misses fall
   back to English, and both Docusaurus sites build with the tree absent.
   Corrections land in the seeds, never in the pages. The docs gate (the
   `docs-l10n-drift` job in `reference-data-drift.yml`) re-materializes
   both surfaces twice and fails only if the runs are not byte-identical
   (or on a hard error, e.g. a corrupted seed failing import); it also
   writes a per-surface nb coverage summary to the job summary, which
   never gates — drift is pending work, not an error.
