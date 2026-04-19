---
sidebar_position: 30
title: SessionTool authoring guide
---

# SessionTool authoring guide

A `tool.SessionTool` is any tool that wants random access to the
project's block state — block lookups by hash, sidecar reads for
"skip if already done", sidecar writes for cross-run annotations.
The existing `tool.Tool` streaming contract is unchanged;
`SessionTool` is additive.

This note walks through when to implement it and what the wire
conventions are. See [AD-046](/docs/ad/046-kapi-project-model) for
the design rationale.

## When to implement

Implement `SessionTool` when your tool:

- **Can skip expensive work** if a prior run already produced the
  output for a block. Canonical case: AI translation — re-calling
  the LLM for a block whose target is already cached is wasted
  money and latency.
- **Writes annotations** that a downstream tool (same flow or next
  run) wants to consult. TM fuzzy matches, term hits, QA findings.
- **Needs block-by-hash lookup** for cross-reference (rare, but
  e.g. "inline-code alignment against the last-known target").

Do **not** implement it when your tool:

- Is a pure stream transform (filter, identity, encoding convert,
  format read/write). The stream contract already gives you what
  you need.
- Produces output that's cheap to recompute — no caching benefit.
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
    sidecarKind := "targets/" + string(t.targetLocale)
    caps := sess.Capabilities()

    for {
        select {
        case <-ctx.Done():
            return ctx.Err()
        case part, ok := <-in:
            if !ok {
                return nil
            }
            // Skip logic, expensive work, sidecar write...
            if err := t.handle(sess, caps.RandomAccess, sidecarKind, part); err != nil {
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

The per-block helper checks capabilities, consults the sidecar,
runs the core work, writes the sidecar back:

```go
func (t *MyTool) handle(sess blockstore.Session, ra bool, kind string, part *model.Part) error {
    block, ok := part.Resource.(*model.Block)
    if !ok || !block.Translatable || block.ID == "" {
        _, err := t.doTheWork(part)
        return err
    }

    // Hydrate from cache when possible.
    if ra {
        if sc, err := sess.GetSidecar(kind, block.ID); err == nil && len(sc.Payload) > 0 {
            var cached mySidecar
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
        payload, _ := json.Marshal(mySidecar{Text: target})
        if err := sess.PutSidecar(blockstore.Sidecar{
            Kind:      kind,
            BlockHash: block.ID,
            Payload:   payload,
        }); err != nil && !errors.Is(err, blockstore.ErrReadOnly) {
            return fmt.Errorf("my-tool: write sidecar: %w", err)
        }
    }
    return nil
}
```

## Sidecar conventions

| Kind prefix | Used by | Payload shape |
|---|---|---|
| `targets/<locale>` | translators (ai-translate, mt-translate, pseudo-translate, human editor) | `{"text": "...", "provider": "..."}` |
| `annotations/<name>` | term-lookup, tm-leverage, qa checks | tool-specific JSON |
| `skeletons/<format>` | format writers (round-trip skeletons) | opaque payload |

The `targets/<locale>` shape is cross-tool: any translator writes
and reads the same key, so a session hydrated by one can be
continued by another. Keep the payload small and JSON-compatible.

## Read-only stores

The `FormatReaderStore` wraps a raw XLIFF / JSON / etc. file as a
read-only BlockStore. Its `PutSidecar` returns
`blockstore.ErrReadOnly`. Tools should ignore this error on the
sidecar-write path — the in-flight `*model.Block` already carries
the result, and caching is best-effort for the *next* run. See the
pattern in `core/tools/pseudo.go` and
`core/ai/tools/translate.go`.

## Batching + concurrency

If your tool has a concurrent / batched path (like `ai-translate`
with `batchSize > 1` or `concurrency > 1`), wrap the batched path
with session filtering at the **input** (skip cached) and
sidecar-write at the **output**. Example:
`core/ai/tools/translate.go::processBatchedWithSession`.

## Registered store providers

- `memory` — default when no store is declared. Snapshot-per-session,
  last-writer-wins on commit. Capabilities: RandomAccess + Concurrent
  + Writable.
- `cache` — SQLite at `.kapi/cache.db`. The default for kapi
  projects. Full ACID, persistent across runs.
- `format-reader` — wraps a `format.DataFormatReader` as a read-only
  store. Useful for ad-hoc CLI flows (`kapi ai-translate -i
  file.xliff`). RandomAccess=true, Writable=false.
- `bowrain` (forthcoming) — REST against a bowrain-server for
  collaborative projects.

The executor receives the store via `flow.WithBlockStore(s)`; tools
never open the store directly.
