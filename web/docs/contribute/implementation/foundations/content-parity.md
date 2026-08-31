---
sidebar_position: 1
id: content-parity
title: "Content-Model Parity Across Wire Projections"
description: "The lossless round-trip invariant for a Block across the canonical content schema and every projection of it, what must round-trip, and the extend-without-breaking checklist gated by conformance tests."
keywords: [content parity, wire schema, projection, round-trip, overlays, protoconvert, kitchen sink, conformance test, drift guard, neokapi]
---

# Content-Model Parity Across Wire Projections

This note records the parity contract that keeps a `model.Block` intact
wherever it is serialized. It is the operational companion to
[F-04: The content-model wire schema](/contribute/architecture/foundations/f-04-wire-schema)
(the canonical schema and the projection rule) and
[F-02: Content Model](/contribute/architecture/foundations/f-02-content-model)
(the model itself).

## The invariant

> A `model.Block` round-trips losslessly through the canonical schema
> (**model → proto → model** and **model → canonical JSON → model**) and
> through every projection of it for the fields that projection carries,
> including the full **model → wire → store → read-back → model** chain of a
> projection that persists.

F-04 admits exactly one canonical serialization: the `neokapi.content.v1`
protobuf in `core/proto/content/v1`, converted by `core/plugin/protoconvert`.
Every other serialized shape (a transport envelope, a REST payload, a set of
store columns) is an explicitly-labeled **projection**. A projection may carry
*fewer* fields than the canonical schema, and it may add envelope properties of
its own for the two dimensions F-04 keeps out of the canonical messages (target
tone/channel, and per-target status, origin and score). What it may not do is
change the meaning of a field it does carry: for those, the round-trip must be
exact.

That distinction is what makes parity testable. The canonical leg is proved
once, in the framework, by `core/plugin/protoconvert`'s compat corpus and the
canonical-JSON golden files. Each projection then proves only its own legs, and
proves them the same way: a fully populated fixture, deep-equal after the trip,
with reflect-driven guards that fail when a new model field is left unpopulated.

## A worked projection: the sync wire

The venue sync wire exercises more of the contract than any other projection
(two block shapes with a content store between them), so it serves as the worked
example. A connector, an archive format, or any other projection
is held to the same three obligations: declare what it carries, convert it in
both directions, and gate both directions with a populated fixture.

| Path | Direction | Where | Overlay carriage |
| --- | --- | --- | --- |
| **Proto push** | kapi → server | `core/venue` (`BlockToProto`/`ProtoToBlock`), message `SyncBlock` in `core/proto/sync/v1` | typed `neokapi.content.v1.OverlayMessage` (reuses `protoconvert.OverlayToProto`) |
| **JSON pull** | server → kapi | `host/venue/client` (`StoredBlockToSyncBlock`/`SyncBlockToBlock`), JSON `SyncBlock` | discriminated JSON blob via `core/venue.MarshalOverlays` (matches the annotations blob idiom) |
| **Store** | venue persistence | a venue implementation's store (`StoreBlocks`/`GetBlock`) | an `overlays` column whose codec delegates to `core/venue` |

The overlay JSON codec lives once in `core/venue` (`overlays_json.go`)
and is shared by the JSON pull path and the store, so the wire shape is defined
in one place rather than mirrored per consumer.

### Why the proto push carries segmentation explicitly

The canonical `BlockMessage` reconstructs segmentation from its multi-segment
source/target boundaries and therefore **excludes** segmentation from its
`overlays` field. A `SyncBlock` instead carries source/target as a **single wire
segment**, so segmentation is not reconstructable from segment boundaries; it
rides in the `SyncBlock.overlays` list explicitly, alongside term/entity/qa/…
This is the one intentional divergence from the `BlockMessage` overlay rule.

## What must round-trip

The list below is the model's parity surface: what any projection's fixture
has to populate before its round-trip test means anything. The sync wire's
fixture is `core/venue/venuetest.KitchenSinkBlock`; a new projection writes
its own against the same list.

- **Scalars**: `ID`, `Name`, `Unit`, `Type`, `MimeType`, `Translatable`,
  `PreserveWhitespace`, `IsReferent`, `SourceLocale`, `SourceStatus`.
- **Source**: a `[]Run` with **every Run kind** (Text, Ph, PcOpen, PcClose,
  Sub, Plural, Select), including `Run.Attrs` (href/src/alt/title) on Ph/PcOpen.
- **Targets**: multiple `VariantKey`s (locale-only and locale+tone), each with
  `Status`, `Score`, and a full `Origin`, both halves of it: how the target was
  made (kind, engine, tool, reference, timestamp, **confidence**) and what
  governed it (**profile**, **profile version**, **context fingerprint**).
  Tone/channel ride the target map key's text form; status/origin/score ride the
  wire segment's properties.
- **Overlays**: **every OverlayType**: segmentation (incl. an ignorable span),
  term, entity, qa, alignment (variant-scoped) and term-candidate, each with anchors,
  props, variant, and typed span `Value`. Typed values (`*EntityAnnotation`,
  `*TermAnnotation`, …) must rehydrate to their concrete type via the payload
  registry, not a generic map.
- **Annotations**: block-scoped typed payloads (`*Notes`, …) keyed by type name.
- **Properties**, **Skeleton** (incl. polymorphic `SkeletonText`/`SkeletonRef`
  parts), **DisplayHint**, **ContentRef**.
- **Unknown/future overlay kinds** degrade gracefully: an unregistered payload
  round-trips by type name + JSON as a `GenericAnnotation`, never dropped or
  panicking.

`Block.Identity` is **derived** (recomputed by `model.ComputeIdentity`) and is
the only field a completeness guard allow-lists as intentionally not carried. A
projection that wants the hash available without recomputing it carries it in
its own envelope, as the sync wire does in `SyncBlock.content_hash`.

## The conformance gate

The parity contract is enforced by tests.

The canonical leg is gated in the framework, once for everyone:

- **Schema uniqueness** (`core/proto/content/guard_test.go`,
  `TestCanonicalContentSchemaIsUnique`): rejects a second Block, Run, or Segment
  message defined outside `neokapi.content.v1`, which is what stops a projection
  from quietly becoming a rival model definition.
- **Compat corpus** (`core/plugin/protoconvert/compat_test.go`): model → proto →
  model is identity across the Run kinds, overlays, multi-locale targets,
  segmentation, skeleton refs, display hints, and registered annotations.
- **Canonical JSON goldens** (`core/proto/content/v1/json_test.go`,
  `TestCanonicalJSONGolden`): the checked-in bytes must reproduce, and must keep
  decoding to the same messages.

Each projection then gates its own legs. For the sync wire:

- **Kitchen-sink round-trip** (`core/venue/conformance_test.go`,
  `TestKitchenSinkRoundTrip`): model → proto → model is deep-equal for the fully
  populated fixture. The JSON pull equivalent lives in
  `host/venue/client/sync_conformance_test.go`.
- **Full chain**: a venue implementation's store gates its own leg the same
  way, model → proto → `StoreBlocks` → `GetBlock` → proto → model against a
  real database, proving the whole wire→store→pull chain preserves overlays and
  core content.
- **Reflect completeness guard** (`TestBlockFixtureIsComplete`): walks
  `model.Block`'s exported fields and fails if any is left zero in the fixture,
  so a **new Block field** trips the test until it is populated (or allow-listed
  as derived).
- **Provenance completeness guard** (`TestOriginFixtureIsComplete`): the same
  walk one level down, over `model.Origin`. The Block-level guard only sees
  Block's own fields, so a new `Origin` field sits zero inside a non-zero
  `Targets` map and slips past it, and the round-trip then passes while
  silently dropping it. Provenance is the one record that cannot be
  reconstructed later, so it gets its own guard.
- **Kind tables** (`venuetest.AllRunKinds`, `venuetest.AllOverlayKinds`): iterated
  by the round-trip tests; a **new Run/Overlay kind** trips them until it is
  added to the table, the fixture, and the converter.

## Extend-without-breaking checklist

Adding a Block/Run/Overlay field, or a new Run/Overlay kind:

1. **Canonical schema**: add the field to the `content.proto` message and run
   `make proto` (never hand-edit the generated `*.pb.go`), then wire it into
   `core/plugin/protoconvert` in both directions and extend the compat corpus.
   Everything downstream reads the model through this schema, so it comes first.
2. **Each projection's converters, both directions**: a projection that must
   carry the field converts it on the way out and on the way back. On the sync
   wire that is `core/venue` (`BlockToProto`/`ProtoToBlock`) **and** the
   JSON pull path (`host/venue/client/sync_convert.go`); a projection with its
   own envelope message adds the field there too.
3. **Persistence**: if a projection stores the field, add the column and its
   (de)serialization. On the sync wire that is the venue implementation's store,
   and `core/blockstore/sqlitestore` locally.
4. **Fixtures**: populate the new field or kind in every projection's fixture,
   and add a new kind to the kind tables the round-trip tests iterate
   (`venuetest.AllRunKinds` / `venuetest.AllOverlayKinds` for the sync wire).
5. **Run the conformance tests**: they are the gate. Green means parity holds.

## Known limitations of the sync-wire projection

These are properties of that one projection, not of the contract. They are
recorded here because a projection is required to say what it does not carry.

- The **store persists a subset** of block fields (id/name/type/mime/
  translatable, source runs, properties, overlays, plus targets/annotations in
  side tables). Skeleton, DisplayHint, ContentRef, SourceLocale, SourceStatus,
  IsReferent, and PreserveWhitespace are **not** store columns today, so they do
  not survive the store leg; they *do* survive the model↔proto and model↔JSON
  legs. The full-chain test therefore asserts overlays + core content, not full
  deep-equal.
- **`Block.Skeleton` is not stored: it is a delivery-edge concern.** The skeleton (a format's non-translatable
  document frame: comments, attribute order, non-translatable entries, element
  ordering) is *typed scaffolding of one format* and would couple the
  format-agnostic content store to per-format structure. Instead, a venue's
  delivery connector reconstructs it **at the edge**: its write path re-reads the
  co-located **source** document, captures its skeleton with the format reader,
  and splices the reviewed targets back in, exactly the local `kapi merge`
  round trip (`host/merge.go` `writeMergedSourceWithSkeleton`). This is always
  available when the same checkout pushes and delivers, so the source sits on
  disk next to the delivery target. Pure-structure formats (json/yaml/arb/po/…) whose block set fully
  determines the file deliver byte-identically either way, so the re-parse path
  is transparent for them.
  - Content held by a venue with **no** co-located source at delivery time
    degrades to the from-blocks reconstruction rather than reintroducing
    skeleton storage into the venue.
- On the **proto push** path an *unregistered* overlay payload degrades to a
  `GenericAnnotation` whose `Fields` nests the payload's whole JSON (via
  `protoconvert`), whereas the JSON pull / store codecs reconstruct it exactly.
  No data is lost either way; registered (built-in) overlay kinds round-trip
  exactly on all paths.
