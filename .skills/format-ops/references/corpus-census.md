# corpus-census — verify provenance, re-fetch wild files, police licenses

## Purpose

Keep every `corpus.yaml` manifest true: sha256-verify all tiers, externally
re-verify `origin: url|archive-member` entries (provenance is verified, not
trusted — only externally-verified entries count toward the C3 wild-files
floor), enforce the licensing red lines, regenerate `SOURCES.md` views, and
shortlist harvests for thin formats.

## Due when

- 90-day cadence elapsed; or
- any `corpus.yaml` sha mismatch (due.mjs hashes the manifests); or
- the corpus release was respun (`release_tag` watermark vs
  `gh release view`).

## Inputs

- Manifests: `core/formats/<id>/corpus.yaml` (canonical for all tiers; entry
  fields per `docs/internals/format-maturity.md` §2.5).
- Fetched store: `corpus/<version>/<id>/` via
  `scripts/fetch-corpus.sh` (`make fetch-corpus [FORMAT=<id>]`).
- Release: `gh release view format-corpus-vN --json tagName,assets`
  (needs-network).
- Publisher: `scripts/publish-corpus.sh` (merge-never-drop idiom).

## Steps

1. **Integrity**: for every manifest entry, recompute sha256 of the file at
   `path` (fetch Tier B first; a missing fetch is a skip-with-command, not a
   failure) and compare against the manifest. Mismatch → finding; fix the
   manifest only when the file change is explained (git history), otherwise
   treat as corruption.
2. **External verification**: for each `origin: url|archive-member` entry,
   re-fetch `source_url` and verify `sha256(fetched) == manifest.sha256`.
   Record per-file results in the ledger:
   `corpus-census.external_verification = {<path>: {verified_at, ok}}`.
   A failed re-fetch (gone URL) keeps the file but marks `ok: false` — it no
   longer counts toward C3.
3. **Licensing red lines** (hard rules):
   - Tier A committed files: **CC0 / own-created / US-gov only**.
   - `LicenseRef-Unverified` is legal **only** on Tier B (never vendored).
   - **Common Crawl grants no license to crawled content: fetch-on-demand
     only, `redistributable: false`, never vendored.**
   - GPL-licensed sources are *read-about*, never harvested.
   - Any committed file with no covering manifest entry → C0 finding.
   - A legacy `testdata/corpus/SOURCES.md` existing **without** a covering
     `corpus.yaml` → census FAILS for that format.
4. **SOURCES.md regeneration**: regenerate the human-readable `SOURCES.md`
   views from `corpus.yaml` (they are generated views, never hand-edited).
5. **Harvest shortlist** for thin formats (< C2 with no countersigned `na`),
   drawn from the standing source list:
   - govdocs1 by-type tarballs (US-gov, redistributable);
   - NapierOne (attribution license);
   - SafeDocs CC-MAIN PDF shards + UNSAFE-DOCS as the hostile set;
   - bug-tracker attachments (LibreOffice's
     `get-bugzilla-attachments-by-mimetype`, adapted to GitHub issues);
   - producer-variance matrices: the same document saved by
     Word/LibreOffice/Google Docs → self-created → CC0 → Tier A.
   Each shortlist row: format, source, expected tier, license class.
6. **Release update**: if manifests changed, respin per-format assets via
   `scripts/publish-corpus.sh` (merge-never-drop; per-format
   `corpus-<id>.tar.gz` so a one-format respin does not re-ship every
   binary); update `watermarks.release_tag`.

## Verification (→ `runs[].evidence`)

`{check: "corpus-census sha sweep", exit, output_sha: sha256(census report
listing per-entry ok/mismatch/refetch results)}`.

## Ledger updates

`last_run`; `watermarks.manifest_shas = {<format>: sha256(corpus.yaml)}` for
every manifest verified; `watermarks.release_tag`;
`external_verification` per-file map; `runs[]` entry — same commit as any
manifest/SOURCES.md edits.

## Outputs

Verified manifests, refreshed `external_verification`, regenerated
SOURCES.md, harvest shortlist (→ `remediate` carryover or followup issues),
respun release when needed.

## Failure → blocked

Offline (no re-fetch possible) → run the integrity half on committed files,
mark external verification `needs-network`, outcome `blocked` with the split
stated. Never mark `ok: true` without an actual re-fetch.
