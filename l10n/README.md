# Dogfood localization seeds

This directory holds the committed inputs for localizing neokapi's own
surfaces with kapi (the root `neokapi.kapi` recipe). The `.kapi/` state
directory is gitignored and rebuilt from these seeds with `make l10n-seed`.

- `brand-voice.yaml` — the machine-readable encoding of
  [docs/internals/brand-communication.md](../docs/internals/brand-communication.md),
  bound project-wide via `defaults.brand_voice`. Keep the two in sync.
- `termbase.csv` — terminology decisions per target locale (currently
  Norwegian Bokmål, `nb`). Columns: `source_term, target_term, domain,
  definition, status`. Imported into `.kapi/termbase.db`.
- `tm/<surface>-<lang>.tmx` — reviewed translations as TMX seeds, one file
  per surface and locale (e.g. `builtins-nb.tmx`). Imported into
  `.kapi/tm.db`; every localized output is produced from the TM by
  `tm-leverage`, so generated catalogs only ever contain reviewed strings.

Workflow for a new or changed surface string:

1. Translate it (human, or `kapi ai-translate` with credentials — the brand
   voice profile and termbase are bound project-wide) and add the pair to
   the surface's TMX seed.
2. `make l10n-seed` to rebuild the TM, then the surface target
   (e.g. `make l10n-builtins`).
3. `make l10n-builtins-check` runs the terminology gate (`kapi term-check`)
   over the result.

Review happens in the TMX seeds — they are the human-owned artifact.
