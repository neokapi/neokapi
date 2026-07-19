---
id: content-parity
title: "Content-Model Parity Over the Sync Wire"
description: "The lossless round-trip invariant for a Block across the kapi↔bowrain sync wire (model ↔ proto ↔ store), what must round-trip, and the extend-without-breaking checklist gated by the kitchen-sink conformance test."
keywords: [content parity, sync wire, round-trip, overlays, SyncBlock, protoconvert, kitchen sink, conformance test, drift guard, neokapi]
---

# Content-Model Parity Over the Sync Wire

This note records the parity contract for the kapi↔bowrain sync wire: a
`model.Block` that kapi has locally must survive **push → store → pull**
losslessly. It is the operational companion to
[AD-034: Content-Model Wire Schema](/contribute/architecture/034-content-model-wire-schema)
(the canonical schema) and [AD-002: Content Model](/contribute/architecture/002-content-model)
(the model itself).

## The invariant

> A `model.Block` round-trips losslessly through every sync wire path:
> **model → proto → model**, **model → JSON → model**, and the full
> **model → wire → store → pull → model** chain.

The canonical wire schema is the `neokapi.content.v1` protobuf
(`core/proto/content/v1`), converted by `core/plugin/protoconvert`. Everything
else — the sync-push envelope, the REST pull JSON, the store columns — is an
explicitly-labeled projection. A projection may carry *fewer* fields than the
canonical schema, but for the fields it does carry the round-trip must be exact.

## The sync wire paths

There are two block projections on the sync wire, plus the content store between
them:

| Path | Direction | Where | Overlay carriage |
| --- | --- | --- | --- |
| **Proto push** | kapi → server | `bowrain/core/sync` (`BlockToProto`/`ProtoToBlock`), message `SyncBlock` in `bowrain/core/proto/sync/v1` | typed `neokapi.content.v1.OverlayMessage` (reuses `protoconvert.OverlayToProto`) |
| **JSON pull** | server → kapi | `bowrain/core/client` (`StoredBlockToSyncBlock`/`SyncBlockToBlock`), JSON `SyncBlock` | discriminated JSON blob via `bowrain/core/sync.MarshalOverlays` (matches the annotations blob idiom) |
| **Store** | server persistence | `bowrain/store` (`StoreBlocks`/`GetBlock`) | `overlays` column, `bowrain/store.MarshalOverlays` → delegates to `bowrain/core/sync` |

The overlay JSON codec lives once in `bowrain/core/sync` (`overlays_json.go`)
and is shared by the JSON pull path and the store, so the wire shape is defined
in one place rather than mirrored per consumer.

### Why the proto push carries segmentation explicitly

The canonical `BlockMessage` reconstructs segmentation from its multi-segment
source/target boundaries and therefore **excludes** segmentation from its
`overlays` field. A `SyncBlock` instead carries source/target as a **single wire
segment**, so segmentation is not reconstructable from segment boundaries — it
rides in the `SyncBlock.overlays` list explicitly, alongside term/entity/qa/…
This is the one intentional divergence from the `BlockMessage` overlay rule.

## What must round-trip

Populate every one of these in the kitchen-sink fixture
(`bowrain/core/synctest.KitchenSinkBlock`) and assert it survives:

- **Scalars**: `ID`, `Name`, `Type`, `MimeType`, `Translatable`,
  `PreserveWhitespace`, `IsReferent`, `SourceLocale`, `SourceStatus`.
- **Source**: a `[]Run` with **every Run kind** — Text, Ph, PcOpen, PcClose,
  Sub, Plural, Select — including `Run.Attrs` (href/src/alt/title) on Ph/PcOpen.
- **Targets**: multiple `VariantKey`s (locale-only and locale+tone), each with
  `Status`, `Score`, and a full `Origin` (kind, engine, tool, reference,
  timestamp, **confidence**). Tone/channel ride the target map key's text form;
  status/origin/score ride the wire segment's properties.
- **Overlays**: **every OverlayType** — segmentation (incl. an ignorable span),
  term, entity, qa, alignment (variant-scoped), term-candidate — with anchors,
  props, variant, and typed span `Value`. Typed values (`*EntityAnnotation`,
  `*TermAnnotation`, …) must rehydrate to their concrete type via the payload
  registry, not a generic map.
- **Annotations**: block-scoped typed payloads (`*Notes`, …) keyed by type name.
- **Properties**, **Skeleton** (incl. polymorphic `SkeletonText`/`SkeletonRef`
  parts), **DisplayHint**, **ContentRef**.
- **Unknown/future overlay kinds** degrade gracefully: an unregistered payload
  round-trips by type name + JSON as a `GenericAnnotation`, never dropped or
  panicking.

`Block.Identity` is **derived** (recomputed by `model.ComputeIdentity`; its
`ContentHash` rides in `SyncBlock.content_hash`) and is the only field the
completeness guard allow-lists as intentionally not carried.

## The conformance gate

The parity contract is enforced by tests, not by review vigilance:

- **Kitchen-sink round-trip** (`bowrain/core/sync/conformance_test.go`,
  `TestKitchenSinkRoundTrip`): model → proto → model is deep-equal for the fully
  populated fixture. The JSON pull equivalent lives in
  `bowrain/core/client/sync_conformance_test.go`.
- **Full chain** (`bowrain/store/sync_roundtrip_test.go`,
  `TestSyncOverlayFullChainRoundTrip`): model → proto → `StoreBlocks` →
  `GetBlock` → proto → model against a real Postgres testcontainer, proving the
  whole kapi→wire→store→pull chain preserves overlays and core content.
- **Reflect completeness guard** (`TestBlockFixtureIsComplete`): walks
  `model.Block`'s exported fields and fails if any is left zero in the fixture —
  so a **new Block field** trips the test until it is populated (or allow-listed
  as derived).
- **Kind tables** (`synctest.AllRunKinds`, `synctest.AllOverlayKinds`): iterated
  by the round-trip tests; a **new Run/Overlay kind** trips them until it is
  added to the table, the fixture, and the converter.

## Extend-without-breaking checklist

Adding a Block/Run/Overlay field, or a new Run/Overlay kind:

1. **`.proto`** — add the field to `SyncBlock` (or the canonical
   `content.proto` message) and run `make -C bowrain proto` (never hand-edit the
   generated `*.pb.go`).
2. **Converter, both directions** — wire it in `bowrain/core/sync`
   (`BlockToProto`/`ProtoToBlock`) **and** the JSON pull path
   (`bowrain/core/client/sync_convert.go`, both directions).
3. **Store** — if it must persist, add the column/serialization in
   `bowrain/store` (and `sqlitestore`) and its (de)serialization.
4. **Kitchen-sink fixture** — populate the new field/kind in
   `synctest.KitchenSinkBlock` (and add a new kind to `synctest.AllRunKinds` /
   `synctest.AllOverlayKinds`).
5. **Run the conformance tests** — they are the gate. Green means parity holds.

## Known, deliberate limitations

- The **store persists a subset** of block fields (id/name/type/mime/
  translatable, source runs, properties, overlays, plus targets/annotations in
  side tables). Skeleton, DisplayHint, ContentRef, SourceLocale, SourceStatus,
  IsReferent, and PreserveWhitespace are **not** store columns today, so they do
  not survive the store leg — they *do* survive the model↔proto and model↔JSON
  legs. The full-chain test therefore asserts overlays + core content, not full
  deep-equal.
- On the **proto push** path an *unregistered* overlay payload degrades to a
  `GenericAnnotation` whose `Fields` nests the payload's whole JSON (via
  `protoconvert`), whereas the JSON pull / store codecs reconstruct it exactly.
  No data is lost either way; registered (built-in) overlay kinds round-trip
  exactly on all paths.
