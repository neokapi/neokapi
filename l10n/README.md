# Dogfood localization seeds

This directory holds the committed inputs for localizing neokapi's own
surfaces with kapi (the root `neokapi.kapi` recipe). The `.kapi/` state
directory is gitignored and rebuilt from these seeds with `make l10n-seed`.

Seeds are committed in the **native KLF-family forms** — deterministic,
lossless JSON that preserves entry identity, so a wipe-and-reseed
reproduces the TM/termbase state exactly (see `sievepen/klftm` and
`termbase/klftb`). TMX/CSV/TBX are the lossy interchange tier; emit
disposable review views with `make l10n-review-export` (→ `l10n/review/`,
gitignored).

- `brand-voice.yaml` — the machine-readable encoding of
  [docs/internals/brand-communication.md](../docs/internals/brand-communication.md),
  bound project-wide via `defaults.brand_voice`. Keep the two in sync.
- `termbase.klftb` — terminology decisions per target locale (currently
  Norwegian Bokmål, `nb`): concept per decision with `en` + `nb` terms,
  domain, definition/usage note, and status. Imported into
  `.kapi/termbase.db`.
- `tm/<surface>-<lang>.klftm` — reviewed translations, one file per
  surface and locale (e.g. `builtins-nb.klftm`). Imported into
  `.kapi/tm.db`; every localized output is produced from the TM by
  `tm-leverage`, so generated catalogs only ever contain reviewed strings.

Workflow for a new or changed surface string:

1. Translate it (human, or `kapi ai-translate` with credentials — the brand
   voice profile and termbase are bound project-wide) and merge the pair
   into the surface's seed: import the seed plus the new pairs (any
   supported form, e.g. a small TMX) into a scratch TM, then
   `kapi tm export -o l10n/tm/<surface>-<lang>.klftm`. Small wording fixes
   can also be edited directly in the `.klftm` JSON — it is the source of
   truth.
2. `make l10n-seed` to rebuild the TM, then the surface target
   (e.g. `make l10n-builtins`, or `make l10n` for everything).
3. `make l10n-builtins-check` runs the terminology gate (`kapi term-check`)
   over the result.

Review happens in the seeds — they are the human-owned artifact. For a
reviewer-friendly view, `make l10n-review-export` writes TMX/CSV renderings
under `l10n/review/`; corrections still land in the `.klftm`/`.klftb`.
