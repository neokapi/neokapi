---
id: 044-klf-klz-integration
sidebar_position: 44
title: "AD-044: KLF / KLZ Format Integration"
---

# AD-044: KLF / KLZ Format Integration

## Context

neokapi's long-tail of existing formats (HTML, Markdown, XLIFF, JSON,
...) cover most translation pipelines that start from a traditional
source file. They don't cover the case of an extractor walking a
React / Vue / Angular / Flutter tree and emitting a structured
bundle of translatable blocks with typed inline codes — the shape
`@neokapi/react` produces today and the shape several upcoming
extractors will produce next.

RFC 0001 defines two new formats to carry that shape:

- **`.klf`** — Kapi Localization Format. JSON document, one or more
  extracted Documents, flat `Block[]` with `Run[]` source / target
  content. Human-readable, git-diffable, PR-reviewable.
- **`.klz`** — Kapi Localization arcHive. ZIP of `.klf` files +
  skeletons + targets + annotation sidecars + a signed manifest.
  The exchange unit between extractors, CI, and downstream tools.

Plus an invisible runtime acceleration cache — an internal,
content-addressed SQLite database `core/klz` builds on demand per
`.klz` — that is not a file format and never crosses trust
boundaries.

See [AD-045](045-klf-klz-spec.md) for the normative spec.

## Decision

### Phase 1 scope (this AD)

neokapi grows three new framework-level packages and one vocabulary
file. Nothing already in the repository is refactored.

- **`core/klf`** — Go types mirroring `@neokapi/format/src/block.ts`,
  a deterministic JSON reader/writer, a placeholder-preservation
  validator, a well-formed-runs checker, a reference preview
  renderer (`RenderBlockHTML`), and the annotation sidecar layer
  (JSON-Lines encoder/decoder, `ResolveAnchor`, `ValidateAnchor`,
  six machine-readable failure reasons). Zero dependencies beyond
  the standard library.
- **`core/klz`** — a `Reader` / `Writer` pair that wraps `core/klf`
  in a ZIP layer with a stable manifest. Stamps per-part SHA-256
  hashes. Rejects ZIP slip, duplicate parts, empty components,
  non-NFC UTF-8, backslash separators, and parts that escape the
  archive root. Surfaces `VerifyAll()` for integrity checks and
  typed `Documents()` / `Targets(locale)` / `AnnotationFiles()`
  helpers. `BlockByID`, `SimilarSources`, and `TM()` are shaped as
  Phase-1 stubs so downstream tools can import the final API shape
  before the Phase-4 cache layer lands.
- **`core/klz/internal/db`** — Phase-1 scaffold behind the
  `klzcache` build tag. Stores the Phase-4 SQLite schema as a
  reviewable constant, exposes `CacheRoot` / `EntryDir` helpers
  for the `kapi cache` CLI, and stubs `Cache.BlockByID` /
  `Cache.SimilarSources` with `ErrNotImplemented`. Phase-1 binaries
  link without cgo or a SQLite binding.
- **`core/formats/jsx`** — a `DataFormatReader`/`DataFormatWriter`
  that routes `.klf` and `.klz` inputs through `core/klf` and
  `core/klz`, emits one `model.Block` per RFC-0001 Block, and
  stashes the canonical `Run[]` on every block as a
  `KLFAnnotation` so the writer can reconstruct the archive
  without going back to disk. Registers in `core/formats/register.go`
  alongside every other built-in format, so every downstream tool
  that already consumes `format.DataFormatReader` automatically
  gains `.klf`/`.klz` support.
- **`core/model/vocabularies/rich-jsx.json`** — three new span
  types (`jsx:element`, `jsx:var`, `jsx:node`) loaded by
  `VocabularyRegistry.LoadDefaults` alongside `common-formatting`,
  `rich-html`, and `code-tokens`. No other vocabulary work
  required.
- **`cli/klz.go`** — the shared CLI command factory. Both `kapi`
  and `bowrain` register `klz {inspect, verify, extract, pack,
  diff, merge, annotations, annotate, orphans}` and `cache {info,
  path, warm, gc, clear}` through one entry point.

Golden fixtures for the three canonical example blocks
(files-heading, tag-chip, shopping-cart) plus the example
annotation sidecar live in `core/klf/{klf_test.go,fixtures_test.go}`
and round-trip through the Go reader/writer byte-for-byte equivalent
to the TypeScript reference.

### What Phase 1 deliberately does NOT do

All of the following are explicitly gated to later phases in RFC
0001 and issue #368's execution protocol. They land under separate
PRs branched off the Phase-1 work.

- **Fragment → Run rewrite in `core/model`**. The existing
  `model.Block` + `Fragment` + `Span` shape stays in place. Blocks
  produced by the jsx reader carry the canonical Runs on a
  `KLFAnnotation` rather than on `model.Block` itself. Phase 2
  replaces this with first-class `Runs []Run` on `model.Block`
  and ports all 40+ existing format readers and writers.
- **Bowrain storage adaptation**. `bowrain/store/`,
  `bowrain/sievepen/`, `bowrain/termbase/`, and the server API
  handlers keep operating on the current shape. Phase 3 migrates
  them together with Phase 2.
- **Runtime acceleration cache (SQLite)**. `core/klz/internal/db`
  is a scaffold. The FTS5-backed `sources_fts` table, the
  atomic-rename build, the LRU GC, and the `BlockByID` /
  `SimilarSources` query paths all land in Phase 4 behind the
  `klzcache` build tag.
- **Extractor interface registry**. `core/klz merge` knows the
  generator id but cannot delegate — `format.Extractor` and
  `format.RegisterExtractor` aren't in this AD. `kapi klz merge`
  surfaces a deferred error pointing at RFC 0001 §Extractor
  interface.

### Two-faced API

`core/klz` exposes two surfaces per RFC 0001 §What the new packages
actually do:

1. **Iteration side** — `Reader.Documents()`,
   `Reader.Targets(locale)`, `Reader.AnnotationFiles()`. Zero
   SQLite involvement, cheap and streaming-friendly. Works with a
   build that omits `internal/db` entirely, producing a pure-JSON
   `kapi` binary with no cgo dependency for CI environments that
   don't want one.
2. **Query side** — `Reader.TM()`, `Reader.BlockByID(ctx, id)`,
   `Reader.SimilarSources(ctx, text, locale, limit)`. In Phase 1
   `BlockByID` is a linear scan over the archive's documents;
   `SimilarSources` returns empty; `TM()` returns nil. Phase 4
   fills in the SQLite-backed implementations behind the
   `klzcache` tag without changing the public signatures.

### Manifest = cache key

The runtime cache is content-addressed by the SHA-256 of the raw
`manifest.json` bytes exactly as stored in the ZIP. The writer's
`MarshalManifest` is deterministic (2-space indent, no HTML
escaping, trailing newline, sorted fields), which is what makes
this cache key stable. Any future change to the manifest
serialization is also a cache-key change, by design.

### Annotation sidecars

Annotations are non-authoritative. Four anchor shapes —
`block`, `run`, `range`, `form` — cover block-level metadata,
run-scoped metadata, character-range hits inside a text run, and
per-plural-form or per-select-case metadata respectively. The
resolver returns one of six machine-readable failure reasons on
anchor failure so orphan-detection validators can emit typed
diagnostics.

`ResolveAnchor` and `ValidateAnchor` are pure functions operating
on `klf.Block` and `klf.Annotation`. They do not read from or
write to the archive — callers plug them into whatever validation
pipeline they need (the `kapi klz orphans` CLI subcommand is one
such caller).

## Consequences

### Positive

- Every neokapi tool that reads `format.DataFormatReader` now
  reads `.klf` and `.klz` for free.
- Round-trip through a `.klz` is lossless when the source reader
  and destination writer both pass the blocks through the
  `KLFAnnotation` bridge, which is the common case.
- The CLI surface (`kapi klz`, `kapi cache`) is stable and
  discoverable before the Phase-4 cache layer is implemented,
  which unblocks CI pipelines and tool authors who want to wire
  the commands up early.
- `ResolveAnchor` lives in Go now, mirrored against the reference
  implementation in
  `packages/format/src/annotation.ts`. Both sides are exercised by
  shared golden fixtures.

### Negative

- Until Phase 2 lands, tools that want structured Runs per block
  must walk the `KLFAnnotation` sidecar attached to each
  `model.Block`. This is a small amount of ceremony but it's the
  price of keeping Phase 1 strictly additive.
- `.klz merge` and `.klz cache warm` are intentionally non-functional
  in Phase 1 and surface deferred errors. This is visible to users.

### Neutral

- `core/klz/internal/db` carries Phase-4 DDL as a constant in the
  Phase-1 tree so the Phase-4 agent has a ready-to-review starting
  point.

## References

- [AD-045: KLF / KLZ Format Specification](045-klf-klz-spec.md) — the normative spec.
- [KLF / KLZ walkthrough](/docs/notes/klf-klz-walkthrough) — end-to-end lifecycle with real file contents.
- [`packages/format/`](https://github.com/neokapi/neokapi/tree/main/packages/format) — `@neokapi/format`, the TypeScript schema.
- Issue [#368](https://github.com/neokapi/neokapi/issues/368)
