---
sidebar_position: 3
title: merge
---

# kapi merge

Apply one or more bilingual files returned by a translator back onto the
project's source locales, using the skeleton captured by
[`kapi extract`](./extract). Introduced by [AD-017: Bilingual Format
Interop](/architecture/017-bilingual-format-interop); the full round-trip
is explained in the [Bilingual Workflow](../bilingual-workflow) guide.

## Synopsis

```bash
kapi merge -i <input> [-i <input> …] [flags]
```

`-i` is repeatable and accepts a **file**, a **glob**, or a
**directory** (all `.xliff` / `.xlf` / `.po` files inside are
processed — mixed XLIFF + PO per batch is fine; format is detected
per input).

## Options

| Flag                     | Default                  | Description                                            |
| ------------------------ | ------------------------ | ------------------------------------------------------ |
| `-p`, `--project <path>` | (auto-discovered)        | Project recipe path. Walks upward from cwd if omitted. |
| `-i`, `--input <path>`   | _(required, repeatable)_ | Input XLIFF file, glob, or directory.                  |
| `--no-tm-update`         | off                      | Skip TM write-back (dry-run / review workflows).       |

## How merge finds the right extraction

Every XLIFF produced by `kapi extract` carries a file-level `<notes>`
entry with the extraction batch id:

```xml
<file id="...">
  <notes>
    <note category="kapi" id="batch-id">6f2e8a1c-...</note>
    <note category="kapi" id="source-file">src/locales/en/app.json</note>
    <note category="kapi" id="source-hash">sha256:...</note>
  </notes>
  ...
</file>
```

Merge reads that id, loads
`.kapi/extractions/<batch-id>/manifest.yaml`, finds the entry for this
source file, and uses the captured skeleton at
`.kapi/extractions/<batch-id>/skel-<hash>.bin` to reconstruct the
target file byte-exactly. The **filename does not need to stay
stable** through the vendor round-trip — batch id alone is enough.

## Stale segment detection

Merge re-reads the current source file and compares every incoming
block's `<source>` against the block of the same ID in the source.

- **Source text matches** → apply the translator's target per conflict
  policy, absorb into TM.
- **Source text differs** → the block has drifted since extract. Count
  as **stale**, skip both the file application and the TM absorb. A
  stale segment is **never silently applied**.

File-level hash drift (the whole source file changed but some blocks
still match) does not block per-block application — non-drifted blocks
still merge cleanly.

## Conflict policy

Controlled by `defaults.merge.conflict_policy` in the recipe:

| Policy            | On-disk target                                             | TM write-back                     |
| ----------------- | ---------------------------------------------------------- | --------------------------------- |
| `translator-wins` | Translator's target replaces existing. **(default)**       | Translator's target replaces TU.  |
| `existing-wins`   | Existing target preserved; translator's skipped.           | Existing TU preserved.            |
| `newest-wins`     | Compare source file mtime vs. XLIFF mtime; pick the newer. | Compare TU `UpdatedAt` vs. XLIFF. |

Merge is non-interactive by design — scriptable in CI without
prompting.

## TM absorb

By default, every applied block is written to the project TM with
provenance:

- `Origin.Source = "merge"`
- `Origin.Reference = <batch-id>`
- `Origin.Key = <source-file-path>`
- `Properties[kapi-merge:xliff-original]` = the returning XLIFF filename
- `Properties[kapi-merge:block-content-hash]` = the block's content hash

Later you can use [`kapi tm audit`](./tm#audit) to trace every TU
back to the merge that introduced it.

Disable with `--no-tm-update` for dry-run / review workflows.

## Multi-file in one pass

```bash
# Full vendor return in one invocation.
kapi merge -i vendor-return/

# Glob pattern.
kapi merge -i 'vendor-return/*.xliff'

# Explicit list, mixed locales.
kapi merge -i fr-FR-batch.xliff -i de-DE-batch.xliff -i es-ES-batch.xliff
```

Per-input outcomes (applied / stale / skipped / tm_new / tm_updated)
are reported independently. A failure on one input does not abort the
rest; exit code reflects any failure.

## Stdout summary

```
Merging vendor-return/app.en-US-to-fr-FR.xliff
  applied=410 stale=2 skipped=0 tm_new=400 tm_updated=10
Merging vendor-return/app.en-US-to-de-DE.xliff
  applied=412 stale=0 skipped=0 tm_new=412 tm_updated=0

Merge complete. applied=822 stale=2 skipped=0 tm_new=812 tm_updated=10 (conflict_policy=translator-wins)
```

## Examples

```bash
# Single file, default conflict policy.
kapi merge -i out/app.en-US-to-fr-FR.xliff

# Entire vendor return directory, skip TM absorb.
kapi merge -i vendor/ --no-tm-update

# Explicit project path (rare — auto-discovery handles most cases).
kapi merge -p /path/to/app.kapi -i return.xliff
```

## See also

- [kapi extract](./extract) — produce bilingual files
- [kapi tm audit](./tm#audit) — trace a merge's TM impact
- [Bilingual Workflow](../bilingual-workflow) — end-to-end narrative
- [AD-017: Bilingual Format Interop](/architecture/017-bilingual-format-interop)
