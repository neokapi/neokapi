---
sidebar_position: 2
title: extract
---

# kapi extract

Emit bilingual XLIFF 2.x files for a translator, pre-filled from the
project TM. Introduced by [AD-017: Bilingual Format
Interop](/architecture/017-bilingual-format-interop); the full
round-trip is explained in the [Bilingual Workflow](../bilingual-workflow)
guide.

## Synopsis

```bash
kapi extract [flags]
```

## Options

| Flag                     | Default                   | Description                                                                           |
| ------------------------ | ------------------------- | ------------------------------------------------------------------------------------- |
| `-p`, `--project <path>` | (auto-discovered)         | Project recipe path. Walks upward from cwd if omitted.                                |
| `--target-lang <csv>`    | recipe `target_languages` | Comma-separated subset of target locales.                                             |
| `--only <collection>`    | all                       | Restrict to one content collection by name.                                           |
| `--pattern <glob>`       | all                       | Extra glob filter on source files.                                                    |
| `--format <name>`        | `xliff2`                  | Output bilingual format: `xliff2` (default) or `po` (gettext).                        |
| `--xliff-version <v>`    | `2.2`                     | XLIFF 2.x version to emit: `2.0`, `2.1`, or `2.2`.                                    |
| `--no-tm`                | off                       | Skip TM pre-fill.                                                                     |
| `--out-dir <dir>`        | `out`                     | Directory for emitted bilingual files. Relative paths resolve under the project root. |

## What it writes

Every run creates a new **extraction batch** under
`.kapi/extractions/<batch-id>/`:

| Artifact                                       | Purpose                                                                                              |
| ---------------------------------------------- | ---------------------------------------------------------------------------------------------------- |
| `manifest.yaml`                                | Source→output mapping, per-file SHA-256, TM leverage counts.                                         |
| `skel-<source-hash>.bin`                       | Per-source skeleton for byte-exact merge reconstruction (when the source format supports skeletons). |
| `<out-dir>/<source-slug>.<src>-to-<tgt>.xliff` | One bilingual file per source → target pair.                                                         |

Each emitted XLIFF carries file-level `<notes>` stamping the batch id,
source file (relative to project root), and source hash — so `kapi
merge` resolves the right extraction without needing the filename to
stay stable through the vendor round-trip. PO output carries the same
metadata as extracted comments on the header entry:

```po
#. kapi-batch: <batch-id>
#. kapi-source-file: src/locales/en/app.json
#. kapi-source-hash: sha256:...
msgid ""
msgstr ""
```

Each translatable entry carries `#. kapi-block: <block-id>` as the
join key for merge, and `#, fuzzy` when TM pre-filled a fuzzy match.

## TM pre-fill

When `--no-tm` is not set (the default), every segment is looked up
against the project TM:

- **Exact** match → pre-filled `<target>` with kapi-internal state
  tracking; the translator sees a high-confidence match in their CAT
  tool.
- **Fuzzy** match ≥ `tm.fuzzy_threshold` (recipe default 75) →
  pre-filled as a fuzzy match. The translator reviews and fixes up.
- Below threshold → not pre-filled; recorded in the batch's
  `suggestions.jsonl` for later analysis.

`--no-tm` short-circuits the lookup entirely (cold-translation
workflows, dry runs).

## Multi-target in one pass

Omitting `--target-lang` extracts every locale declared in the
recipe's `defaults.target_languages`. Passing
`--target-lang fr-FR,de-DE` subsets the list. One XLIFF is emitted per
source → target pair, and all pairs share a single extraction batch
id / manifest so merge can resolve any returning file back to the
same batch.

## Examples

```bash
# From inside a project directory — auto-discovery finds the recipe.
kapi extract

# Explicit project + single target.
kapi extract -p app.kapi --target-lang fr-FR

# Scoped extraction.
kapi extract --only mobile
kapi extract --pattern 'src/marketing/**/*.html'

# Pin an older XLIFF namespace for a legacy tool.
kapi extract --xliff-version 2.0

# Skip TM pre-fill entirely.
kapi extract --no-tm
```

## Stdout summary

```
Extracting batch <id> (format=xliff2, targets=[fr-FR de-DE], sources=1)
  fr-FR: 1 files, 412 blocks, TM exact=108 fuzzy=67 new=237
  de-DE: 1 files, 412 blocks, TM exact=0 fuzzy=0 new=412

Batch <id> complete. Manifest: .kapi/extractions/<id>/manifest.yaml
Aggregate TM leverage: exact=108 fuzzy=67 new=649 (total=824)
```

## See also

- [kapi merge](./merge) — apply the translator's return
- [kapi tm audit](./tm#audit) — trace a merge's TM impact
- [Bilingual Workflow](../bilingual-workflow) — end-to-end narrative
- [AD-017: Bilingual Format Interop](/architecture/017-bilingual-format-interop)
