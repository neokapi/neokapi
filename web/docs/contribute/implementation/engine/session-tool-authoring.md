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
    overlayKind := "targets/" + string(t.targetLocale)
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
            if err := t.handle(sess, caps.RandomAccess, overlayKind, part); err != nil {
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
runs the core work, writes the overlay back:

```go
func (t *MyTool) handle(sess blockstore.Session, ra bool, kind string, part *model.Part) error {
    block, ok := part.Resource.(*model.Block)
    if !ok || !block.Translatable || block.ID == "" {
        _, err := t.doTheWork(part)
        return err
    }

    // Hydrate from cache when possible.
    if ra {
        if sc, err := sess.GetOverlay(kind, block.ID); err == nil && len(sc.Payload) > 0 {
            var cached myOverlay
            if json.Unmarshal(sc.Payload, &cached) == nil && cached.Text != "" {
                block.SetTargetText(t.targetLocale, cached.Text)
                return nil
            }
        }
    }

    // Do the expensive work.
    if _, err := t.doTheWork(part); err != nil {
        return err
    }

    // Cache the result for next time.
    if target := block.TargetText(t.targetLocale); target != "" {
        payload, _ := json.Marshal(myOverlay{Text: target})
        if err := sess.PutOverlay(blockstore.Overlay{
            Kind:      kind,
            BlockHash: block.ID,
            Payload:   payload,
        }); err != nil && !errors.Is(err, blockstore.ErrReadOnly) {
            return fmt.Errorf("my-tool: write overlay: %w", err)
        }
    }
    return nil
}
```

## Overlay conventions

| Kind prefix          | Used by                                                                  | Payload shape                        |
| -------------------- | ------------------------------------------------------------------------ | ------------------------------------ |
| `targets/<locale>`   | translators (translate, pseudo-translate, human editor) | `{"text": "...", "provider": "..."}` |
| `annotations/<name>` | term-lookup, recycle, qa checks                                      | tool-specific JSON                   |
| `skeletons/<format>` | format writers (round-trip skeletons)                                    | opaque payload                       |

The `targets/<locale>` shape is cross-tool: any translator writes
and reads the same key, so a session hydrated by one can be
continued by another. Keep the payload small and JSON-compatible.

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
  Full ACID, persistent across runs.
- `NewPersistentMemoryStore()`: a memory store that advertises `Persistent` and
  whose `Close` is a no-op, so it survives repeated open and close across
  commands in one process. It stands in for the SQLite cache on the wasm build,
  where SQLite is unavailable.
- `NewFormatReaderStore(factory)`: wraps a `format.DataFormatReader` factory as
  a read-only store. Useful for ad-hoc CLI flows (`kapi translate -i
  file.xliff`): RandomAccess=true, Writable=false. Its `PutOverlay` returns
  `blockstore.ErrReadOnly`.

The executor receives the store via the `flow.WithBlockStore(s)` option
(default `NewMemoryStore()`); tools never open the store directly.
