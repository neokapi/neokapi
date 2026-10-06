---
sidebar_position: 6
title: SessionTool Authoring Guide
description: "Implementation note: how to implement a SessionTool that needs random access to the project's block state (lookups by hash, overlay reads and writes) on top of the standard streaming Tool contract."
keywords: [SessionTool, block state, overlay, hash lookup, authoring, implementation note, neokapi]
---

# SessionTool authoring guide

A `tool.SessionTool` is any tool that wants random access to the
project's block state: block lookups by hash, overlay reads for
"skip if already done", overlay writes for cross-run annotations.
The existing `tool.Tool` streaming contract is unchanged;
`SessionTool` is additive.

This note walks through when to implement it and what the wire
conventions are. See [C-01](/contribute/architecture/context/c-01-project-model) for
the design rationale.

## When to implement

Implement `SessionTool` when your tool:

- **Can skip expensive work** if a prior run already produced the
  output for a block. Canonical case: AI translation; re-calling
  the LLM for a block whose target is already cached is wasted
  money and latency.
- **Writes annotations** that a downstream tool (same flow or next
  run) wants to consult. Memory fuzzy matches, term hits, check findings.
- **Needs block-by-hash lookup** for cross-reference (rare, but
  e.g. "inline-code alignment against the last-known target").

Do **not** implement it when your tool:

- Is a pure stream transform (filter, identity, encoding convert,
  format read/write). The stream contract already gives you what
  you need.
- Produces output that is cheap to recompute, so caching gains nothing.
- Writes output exclusively to the in-flight `model.Block` and has
  no persistent state story.

## Minimal implementation

```go
import (
    "github.com/neokapi/neokapi/core/blockstore"
    "github.com/neokapi/neokapi/core/tool"
)

// Compile-time assertion catches accidental drift.
var _ tool.SessionTool = (*MyTool)(nil)

func (t *MyTool) SessionProcess(
    ctx context.Context,
    sess blockstore.Session,
    in <-chan *model.Part,
    out chan<- *model.Part,
) error {
    overlayKind := blockstore.TargetOverlayKind(t.targetLocale) // "targets/<locale>", canonical
    caps := sess.Capabilities()

    for {
        select {
        case <-ctx.Done():
            return ctx.Err()
        case part, ok := <-in:
            if !ok {
                return nil
            }
            // Skip logic, expensive work, overlay write...
            if err := t.handle(ctx, sess, caps.RandomAccess, overlayKind, part); err != nil {
                return err
            }
            select {
            case out <- part:
            case <-ctx.Done():
                return ctx.Err()
            }
        }
    }
}
```

The per-block helper checks capabilities, consults the overlay,
runs the core work, writes the overlay back. A stored target goes back on the
block through `tool.WriteAs`, so it is an operation `change.ApplyBlock` applies
as the tool, like any other write
([E-03](/contribute/architecture/engine/e-03-tool-system)); a model setter
called on the block directly would bypass that:

```go
func (t *MyTool) handle(ctx context.Context, sess blockstore.Session, ra bool, kind string, part *model.Part) error {
    block, ok := part.Resource.(*model.Block)
    if !ok || !block.Translatable || block.ID == "" {
        _, err := t.doTheWork(part)
        return err
    }

    // The overlay key: unique per source file inside a project, the block id
    // in a single-document run.
    key := blockstore.OverlayKey(ctx, block.ID, block.SourceText())

    // Hydrate from cache when the stored target answers this source.
    if ra {
        if sc, err := sess.GetOverlay(kind, key); err == nil && len(sc.Payload) > 0 {
            var cached blockstore.TargetOverlay
            if json.Unmarshal(sc.Payload, &cached) == nil &&
                cached.Source == blockstore.SourceStamp(block.SourceText()) && len(cached.Runs) > 0 {
                return tool.WriteAs(ctx, block, t.ToolName, func(v tool.VariantView) error {
                    v.SetTargetRuns(t.targetLocale, cached.Runs)
                    return nil
                })
            }
        }
    }

    // Do the expensive work.
    if _, err := t.doTheWork(part); err != nil {
        return err
    }

    // Cache the result for next time.
    if runs := block.TargetRuns(t.targetLocale); len(runs) > 0 {
        payload, _ := json.Marshal(blockstore.TargetOverlay{
            Runs:   runs,
            Source: blockstore.SourceStamp(block.SourceText()),
        })
        if err := sess.PutOverlay(blockstore.Overlay{
            Kind:      kind,
            BlockHash: key,
            Payload:   payload,
        }); err != nil && !errors.Is(err, blockstore.ErrReadOnly) {
            return fmt.Errorf("my-tool: write overlay: %w", err)
        }
    }
    return nil
}
```

A real producer also compares the configuration it would send now
(`TargetOverlay.Config`, `TargetOverlay.ReusableFor`) before it serves a stored
target, as `core/ai/tools/translate.go` does.

## Overlay conventions

| Kind prefix          | Used by                                                                  | Payload shape                        |
| -------------------- | ------------------------------------------------------------------------ | ------------------------------------ |
| `targets/<locale>`   | translators (`translate`, `pseudo-translate`) and the flow's `commit-targets` step | `blockstore.TargetOverlay`: `{"runs": [...], "text": "...", "status": "...", "provider": "...", "config": "...", "source": "...", "origin": {...}}` |
| `annotations/<name>` | term-lookup, recycle, qa checks                                      | tool-specific JSON                   |
| `skeletons/<format>` | format writers (round-trip skeletons)                                    | opaque payload                       |

The `targets/<locale>` shape is cross-tool: any translator writes
and reads the same key, so a session hydrated by one can be
continued by another. Build the kind with `blockstore.TargetOverlayKind`,
which writes the locale in canonical form, and the payload as a
`blockstore.TargetOverlay`; the stores canonicalize a `targets/` kind on every
read and write (`blockstore.CanonicalOverlayKind`). `source` is the content hash
of the source the translation was made from (`blockstore.SourceStamp`): the
overlay key names a block by file and id and says nothing about its wording,
so an overlay with no source stamp serves nothing.

`origin` carries the provenance the producer stamped, including the
`ContextFingerprint` of the governing context. Most target formats
have nowhere to keep it; a convergence records it beside each target it
writes, as the producer of the flow's `content.edit` in the block history,
which is where the staleness gate reads it, and the overlay keeps it for a
session that resumes the work.

`annotations/<name>` is cross-tool in the same way, one level down:
`<name>` is the block-annotation key, so a store that holds blocks
rather than loose overlays files the payload as that block's
annotation. A `<name>` the payload registry knows (`note`,
`quality.findings`, …) means the body has to match that
annotation's schema; anything else is kept verbatim and comes back
to your tool exactly as written. `skeletons/<format>` and any other
prefix stay opaque: they name no annotation and no store reads
them but the tool that wrote them.

## Read-only stores

The `FormatReaderStore` wraps a raw XLIFF / JSON / etc. file as a
read-only `blockstore.Store`. Its `PutOverlay` returns
`blockstore.ErrReadOnly`. Tools should ignore this error on the
overlay-write path: the in-flight `*model.Block` already carries
the result, and caching is best-effort for the _next_ run. See the
pattern in `core/tools/pseudo.go` and
`core/ai/tools/translate.go`.

## Batching + concurrency

If your tool has a concurrent / batched path (like `translate`
with `batchSize > 1` or `concurrency > 1`), wrap the batched path
with session filtering at the **input** (skip cached) and
overlay-write at the **output**. Example:
`core/ai/tools/translate.go::processBatchedWithSession`.

## Store providers

The providers are plain constructors in `core/blockstore`, not string-keyed
entries declared in a recipe. The caller (CLI, project runner, executor)
constructs the one it wants and hands it to the executor:

- `NewMemoryStore()`: the default when no store is passed. Snapshot-per-session,
  last-writer-wins on commit. Capabilities: RandomAccess + Concurrent + Writable;
  not Persistent.
- `sqlitestore.New(path)` / `sqlitestore.NewFromDB(db, …)`
  (`core/blockstore/sqlitestore`): the SQLite-backed store, in a kapi project the
  block-cache tables of `.kapi/work/store.db`. The default for kapi projects.
  Full ACID, persistent across runs. The browser build runs the same store on
  SQLite's WebAssembly build.
- `NewFormatReaderStore(factory)`: wraps a `format.DataFormatReader` factory as
  a read-only store over one file, for a Go caller that runs a flow over a
  document it does not want written back: RandomAccess=true, Writable=false.
  Its `PutOverlay` returns `blockstore.ErrReadOnly`. No CLI command constructs
  it.

The executor receives the store via the `flow.WithBlockStore(s)` option
(default `NewMemoryStore()`); tools never open the store directly.
