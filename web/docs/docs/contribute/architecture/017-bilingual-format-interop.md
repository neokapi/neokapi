---
id: 017-bilingual-format-interop
sidebar_position: 17
title: "AD-017: Bilingual Format Interop"
description: "Architecture decision: kapi supports a full bilingual round-trip — extract emits a bilingual file for a translator or reviewer, merge applies their changes back, and TM is updated on every merge for compounding leverage. neokapi's native interchange format is the lossless bilingual .klz (for kapi-equipped recipients); XLIFF 2.x / PO is the industry-interop tier for third-party CAT tools."
keywords: [bilingual format, XLIFF, PO, CAT tool, extract, merge, TM, architecture decision, neokapi]
---

# AD-017: Bilingual Format Interop

## Summary

Kapi ships an end-to-end bilingual round-trip — `kapi extract` emits a
bilingual file for a translator or reviewer, `kapi merge` applies the
returned file back onto the source — with the project TM participating
on both sides of the loop (pre-fill on extract, absorb on merge).
neokapi's **native interchange format is the lossless bilingual `.klz`**
(for recipients working in kapi or the standalone neokapi review tool);
**XLIFF 2.x / PO** is the industry-interop tier for third-party CAT
tools. Block identity is the merge key; segmentation is an opt-in
overlay. The project's `.kapi/cache/extractions/<id>/` directory
is the source of truth for skeleton bookkeeping so the round-trip is
deterministic and portable via `git`.

## Context

Serious localization shops run on bilingual exchange formats. XLIFF
2.x (OASIS; 2.0 in 2014, 2.1 in 2018, 2.2 in 2023) and PO (gettext)
are the lingua franca between authored sources, CAT tools (Trados,
memoQ, OmegaT, Smartcat, Phrase, Crowdin, Lokalise, Weblate, Poedit,
…) and everything else a translation project touches. Kapi already
had high-quality _format support_ for bilingual formats — full
readers and writers with byte-exact roundtripping (AD-005) — but no
workflow glue tying extract, translate, merge, and the TM
touchpoints on each into one integrated feature.

Every data exchange kapi touches falls into one of six categories:

| #   | Boundary               | Direction                | Format(s)                                                                                            |
| --- | ---------------------- | ------------------------ | ---------------------------------------------------------------------------------------------------- |
| 1   | Authored source        | In                       | JSON, YAML, HTML, .strings, .properties, .docx, .xml, .md, …                                         |
| 2   | Translated output      | Out                      | Same as #1                                                                                           |
| 3   | **Bilingual exchange** | Out/In                   | **Bilingual `.klz`** (neokapi-native, lossless — for a kapi-equipped translator/reviewer); **XLIFF 2.x, PO** (industry interop); XLIFF 1.2, Qt TS, XLSX-bilingual, SRT, TTML as format support |
| 4   | **Translation memory** | In (loop) / Out/In (TMX) | Project TM (`sievepen/`), TMX for interop                                                            |
| 5   | Terminology            | In (loop) / Out/In       | Project termbase (`termbase/`), TBX/CSV/JSON                                                         |
| 6   | Project portability    | Out/In                   | `.kapi` folder (YAML recipe + `.kapi/` state)                                                        |

Boundary 3 is the headline gap this AD closes; **boundary 4 is the
silent one**. The TM exists, TMX ships in and out, but the loop that
matters — _leverage on extract, absorb on merge_ — has to be wired
or the most valuable asset a translation project accumulates over
time goes unused.

## Decision

### CLI surface: extract and merge as top-level commands

`kapi extract` and `kapi merge` are top-level kapi commands, not
built-in flows dispatched through `kapi run`. Matches AD-013's
existing command tree and keeps discoverability high.

```bash
kapi extract -p myapp.kapi                   # all target locales from recipe
kapi extract -p myapp.kapi --target-lang fr  # single target
kapi extract -p myapp.kapi --target-lang fr,de,es
kapi extract -p myapp.kapi --only mobile
kapi extract -p myapp.kapi --pattern 'src/**/*.json'
kapi extract -p myapp.kapi --xliff-version 2.0
kapi extract -p myapp.kapi --format po
kapi extract -p myapp.kapi --no-tm

kapi merge   -p myapp.kapi -i out/myapp-en-to-fr.xliff
kapi merge   -p myapp.kapi -i file1.xliff -i file2.xliff -i file3.po
kapi merge   -p myapp.kapi -i 'vendor-return/*.xliff'
kapi merge   -p myapp.kapi -i vendor-return/
kapi merge   -p myapp.kapi -i ... --no-tm-update
```

### Multi-target in one pass

Both commands handle N target locales in a single invocation.
Real projects always ship to multiple languages at once; forcing a
loop around a single-target CLI is a non-starter.

- **Extract**: omitting `--target-lang` defaults to the recipe's
  `target_languages`; `--target-lang fr,de,es` subsets it. One XLIFF
  / PO per source→target pair (e.g. `out/myapp-en-to-fr.xliff`),
  sharing a single extraction batch id / manifest.
- **Merge**: `-i` is repeatable and accepts file paths, globs, or a
  directory. A vendor's full N-language return is one invocation.
  Mixed XLIFF + PO per batch is fine — format is detected per input.
- **Partial-failure UX**: a failure on one pair / input is reported
  per-item; the rest still apply. Exit code reflects any failure.

### Project context auto-discovery

Every project-aware kapi command resolves `-p` in this order:

1. Explicit `-p <path>` flag
2. `KAPI_PROJECT` env var
3. `project.ResolveLayout(cwd)` — git-style upward walk for the
   `{name}.kapi` recipe + adjacent `.kapi/` state directory
4. Fallthrough: one-shot mode (commands that support it) or error
   "not a kapi project" (commands that require one, e.g. `merge`)

`ErrAmbiguousLayout` (multiple `*.kapi` files in the same directory)
surfaces as a CLI error asking for explicit `-p`.

The helper lives in `cli/` once and is reused by `run`, `extract`,
`merge`, and any future project-aware command.

### Exchange formats

Interchange has two tiers, chosen by **who receives the file**:

- **neokapi-native — the bilingual `.klz`.** A task-scoped profile of the `.klz`
  container ([AD-025](025-klf-package.md) §7): one source→target pair, the blocks
  with faithful inline codes, the segmentation/alignment overlays, the per-source
  skeleton for round-trip, and the relevant TM-match + termbase context — one
  lossless, deterministic, content-addressed file. This is the format kapi
  distributes to a translator or reviewer working in kapi or the standalone
  neokapi review tool. It is lossless where XLIFF is lossy, carries TM and term
  context inline rather than as separate TMX/TBX attachments, and — being
  Merkle-hashable — gives **integrity-verified, diffable** review (exactly what
  changed is visible and tamper-evident). It is *ecosystem* interchange (both ends
  need a neokapi reader); making it a cross-vendor standard is an open-spec +
  second-implementation effort, not a property of the bytes.
- **Industry interop — XLIFF 2.x / PO.** For any recipient on a third-party CAT
  tool. neokapi stays an excellent citizen of the existing standard: maximally
  faithful, lossless-as-the-format-allows round-trip. You can never opt out of
  XLIFF, only offer something better alongside it.

Both tiers flow through the same `extract` / `merge` verbs; `--format` selects the
carrier (`klz` native, `xliff` / `po` interop). `kapi extract` emits **XLIFF 2.2
by default** for safe interop with any recipient; the bilingual `.klz` is selected
with `--format klz` for recipients working in kapi or the neokapi review tool.

**XLIFF 2.x** is the default *industry-interop* carrier. The reader accepts all
three 2.x namespaces as a compatible family (`…:2.0`, `…:2.1`,
`…:2.2`) and preserves unknown 2.x attributes round-trip via the
layer property map. The writer emits **2.2** by default, with
`--xliff-version 2.0|2.1|2.2` as an opt-out for consumers stuck on
older tooling.

**PO (gettext)** is the day-one alternate. Selected with `--format
po`. One entry per segment span (the whole block when unsegmented).
Kapi-specific bookkeeping rides in developer comments
(`#. kapi-block: <hash>/<sN>`).

XLIFF 1.2, Qt TS, XLSX-bilingual, SRT, and TTML remain available as
format support (byte-exact roundtrip via their readers/writers) but
do not get extract/merge integration in v1.

### Block is the merge key; segmentation is an overlay

The merge key is the block content hash (`core/model/identity.go`
`ComputeContentHash`, SHA-256 over the normalized source runs).
Segmentation is a stand-off overlay over those runs (AD-002), not a
rewrite of them, so it never changes the hash — a block's identity is
stable across segmentation on/off toggles between extractions.

With no segmentation overlay, a Block emits one XLIFF `<segment>` /
one PO entry over its whole content. When the recipe sets
`segmentation.source: true`, the `segment` annotator runs as a
pipeline stage and attaches a segmentation overlay with stable span
ids (`s1`, `s2`, …); the writer materializes one `<segment>` /
`<ignorable>` (or PO entry) per span and gap. Merge maps each
returned target back to its source span via the alignment overlay and
splices the target runs into place — the block hash is the join key,
so a project can flip segmentation on or off between extractions
without breaking TM, QA overlays, or manifest bookkeeping.

Per-segment TM lookup is the matching widening: `sievepen.Lookup`
keys on the whole block when there is no segmentation overlay. When
one is present, extract iterates its spans and looks each up
independently for sentence-level leverage via the `LookupSegment`
method on the `TranslationMemory` interface.

### Skeleton portability (project-state only, v1)

V1 stores skeletons in project state (`.kapi/cache/extractions/<id>/`).
Merge therefore requires the same `.kapi` project that produced the
extraction. The emitted XLIFF / PO is clean, CAT-friendly, and small.

The door is left open for a future `--embed-skeleton` flag on
`kapi extract` that would produce a self-contained XLIFF 2.0 with
the skeleton embedded; data structures don't assume project-state
forever.

### Extraction manifest

Each `kapi extract` run writes a manifest at
`.kapi/cache/extractions/<batch-id>/manifest.yaml`:

```yaml
schemaVersion: 1
kind: kapi-extraction
batchId: 6f2e8a1c-...
generator: { id: kapi, version: v1.x }
createdAt: 2026-04-24T10:00:00Z
sourceLocale: en-US
options: { format: xliff2, xliffVersion: "2.2", noTM: false }
pairs:
  - targetLocale: fr-FR
    output: out/myapp-en-to-fr.xliff
    files:
      - source: src/locales/en-US/app.json
        sourceHash: sha256:...
        blocks: 412
        leverage: { exact: 108, fuzzy: 67, new: 237 }
        skeleton: skel-<source-hash>.bin
  - targetLocale: de-DE
    output: out/myapp-en-to-de.xliff
    files: [...]
```

The batch id is stamped in each emitted XLIFF / PO so merge can
resolve any returning file back to the right manifest without
guessing from file name.

- **XLIFF 2**: `<file>`-level `<notes>` entry with category `kapi`,
  content `batch:<uuid>`.
- **PO**: file-header extracted comment `#. kapi-batch: <uuid>`.

Sub-threshold TM matches are written to
`.kapi/cache/extractions/<batch-id>/suggestions.jsonl` for later analysis
without touching the emitted target.

### TM-in on extract (v1, on by default)

`kapi extract` queries the project TM for every segment it emits.

- Exact match → pre-fill `<target>` with `state="translated"`.
- Fuzzy match ≥ `tm.fuzzy_threshold` (default 75) → pre-fill with
  `state="fuzzy"` and a `matchQuality` sub-state attribute.
- Sub-threshold matches → `suggestions.jsonl`, not inlined.

Disable with `--no-tm`. Additional read-only TMs can be declared in
the recipe via `tm.read: [path, …]` and are consulted alongside the
project TM during pre-fill.

### TM-out on merge (v1, on by default)

`kapi merge` writes every accepted target segment into the project
TM. TUs carry provenance:

- `Origin.Source = "merge"`
- `Origin.Reference = <batch-id>`
- `Origin.Key = <source-file-path>`
- Block-hash and originating XLIFF filename as properties

Disable with `--no-tm-update`. Combined with TM-in on the next
extract, this is the "memory" part of translation memory — without
it TM leverage decays to zero.

**Write-back only to the project TM.** Imported read-only TMs
(`tm.read`) are never written to; this keeps imported TMX
reproducible from its source.

### Conflict policy

`merge.conflict_policy` (project recipe field) governs:

- Applying the translator's target to the source file when an
  existing target is present on disk;
- Writing back to the TM when a TU already carries a translation.

Values:

- `translator-wins` (default) — the translator's target always
  replaces the existing one.
- `existing-wins` — existing on-disk / on-TM target is preserved;
  translator's target is skipped with a warning.
- `newest-wins` — compare timestamps (file mtime / TU
  `UpdatedAt`) and pick the newer.

No interactive prompting — keeps merge scriptable in CI.

### Stale segment handling

Merge detects stale segments by comparing the incoming XLIFF's
recorded source hash (captured at extract time) against the current
source. Stale segments are **reported**, **not silently applied**,
and **not TM-absorbed** even when the conflict policy would
otherwise accept the target.

Partial returns are fine: merge finds the extraction manifest by
batch id and applies translated segments, leaving untranslated ones
alone.

### Recipe schema

Three new sections on `Defaults`:

```yaml
defaults:
  source_language: en-US
  target_languages: [fr-FR, de-DE, es-ES]
  merge:
    conflict_policy: translator-wins # | existing-wins | newest-wins
  tm:
    fuzzy_threshold: 75 # int 0..100
    read: # optional read-only TMs
      - /path/to/corporate.tmx
  segmentation:
    source: false # opt-in
    srx: rules.srx # optional SRX override
```

Unknown fields are rejected with a clear error. Enum values are
validated against the allowed set. Defaults apply when the section
is absent.

## Relationships to other ADs

- **AD-005 (Format System)** — XLIFF 2.x writer/reader and PO
  reader/writer are the carriers; skeleton store is how byte-exact
  roundtrip works.
- **AD-008 (Project Model)** — extract adds the extraction manifest
  under `.kapi/cache/extractions/<id>/`. Conflict policy is a new
  `Defaults.Merge` section. Auto-discovery uses the existing
  `project.ResolveLayout` entry point.
- **AD-009 (Translation Memory)** — `Lookup` becomes load-bearing
  for extract pre-fill. A `LookupSegment` method is added for
  per-span matching when a segmentation overlay is present. Merge
  extends the `Origin` provenance story with a `"merge"` source and
  the batch id in `Reference`.
- **AD-010 (Terminology)** — termbase-informed glossary hints on
  extract are a natural follow-up (analogous to TM pre-fill). Not
  in v1.
- **AD-013 (Kapi CLI)** — adds `extract` and `merge` top-level
  commands and describes the auto-discovery resolution order.
- **AD-015 (Testing & Documentation)** — extract/merge each ship
  with an interactive walkthrough embed and a unified workflow guide on the
  docs site.

## Rationale

**Why extract/merge as top-level commands rather than flows?**
Discoverability. A localization engineer looking at `kapi --help`
sees them immediately alongside `run`. Building them as flows would
hide them a layer deeper and make their specialized flag shapes
(`--only`, `--pattern`, `-i` repeatable, conflict policy) awkward to
express.

**Why block hash as merge key, not segment position?** Segmentation
is a stand-off overlay, not a structural change. The same Block may
emit 1 or N `<segment>`s depending on the recipe; using block hash as
the stable key means the project can change segmentation settings
between extractions without breaking merge. Segment span ids (`s1`,
`s2`, …) ride _inside_ the segmentation overlay.

**Why project-state skeletons (not embedded)?** Keeps the emitted
XLIFF small and CAT-friendly. Makes TM absorb cheap (no skeleton
to unpack on merge). The project folder is already the unit of
portability — `git push` ships it. Embedded skeleton remains an
opt-in escape hatch for workflows that can't ship the project.

**Why TM-in + TM-out on by default?** Without the loop, TM is a
sidecar — interesting, not load-bearing. The moment the loop is on,
the TM is the memory: each merge makes the next extract cheaper.
Making it opt-in would leave most users' TMs empty or stale.

**Why write-back only to the project TM?** Imported TMX should be
reproducible from its source file; writing into it turns it into a
living artifact that drifts from the TMX on disk. The project TM
is the editable thing.
