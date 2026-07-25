---
id: 034-content-model-wire-schema
sidebar_position: 34
title: "AD-034: Content-Model Wire Schema"
description: "Architecture decision: core/proto/content/v1 is the single canonical protobuf serialization of the content model (Part, Block, Run, Overlay, Target, Skeleton); every other serialized shape is either an import of that schema or an explicitly-labeled projection, and a guard test enforces the boundary."
keywords: [wire schema, protobuf, content model, canonical serialization, protoconvert, projection, sync protocol, BlockIndex, ContentTree, compatibility policy, architecture decision, neokapi]
---

# AD-034: Content-Model Wire Schema

## Summary

The content model (AD-002) has exactly one canonical wire representation:
the protobuf messages in `core/proto/content/v1/content.proto` (proto package
`neokapi.content.v1`), converted to and from `core/model` by
`core/plugin/protoconvert`. Every surface that serializes Parts, Blocks, or
Runs either imports this schema or is an explicitly-labeled *projection* — a
lossy, purpose-built shape that is never a model contract. A guard test
rejects new Block/Run message definitions outside the canonical package.

## The canonical schema

`core/proto/content/v1/content.proto` defines the complete, symmetric message
set for the model: `PartMessage`, `BlockMessage`, the `RunMessage`
discriminated union (`TextRunMessage`, `PlaceholderRunMessage`,
`PcOpenRunMessage`, `PcCloseRunMessage`, `SubRunMessage`, `PluralRunMessage`,
`SelectRunMessage`), `OverlayMessage`/`SpanMessage`/`RunRangeMessage`/
`VariantMessage`, `SegmentMessage`/`TargetEntry`, `SkeletonMessage`,
`DisplayHintMessage`, `LayerMessage`, `DataMessage`, `GroupStartMessage`/
`GroupEndMessage`, `MediaMessage`, the lightweight `ContentBlock`, and
`ContentRef`.

These messages originated as the plugin-bridge proto and are proven
cross-language in production by the Java okapi-bridge; promoting them to a
dedicated package makes the model schema a first-class artifact rather than
nominally a plugin detail. The bridge service proto
(`core/plugin/proto/v2/neokapi_bridge.proto`) imports the canonical package
and keeps only the `BridgeService` definition and its request/response
envelopes; the generated Go package `bridgev2` re-exports the content types
as aliases so existing plugin hosts compile unchanged.

`core/plugin/protoconvert` is the canonical converter: every message kind has
a symmetric `XToProto`/`ProtoToX` pair, and its test suite (in particular the
compat corpus in `core/plugin/protoconvert/compat_test.go`) is the
compatibility oracle — model → proto → model is identity for every Run kind,
overlays, multi-locale targets, segmentation, skeleton refs, display hints,
and registered annotations.

Two model dimensions deliberately do not cross this schema: target variant
tone/channel (targets are keyed by locale in `TargetEntry`) and per-target
status/origin/score. Protocols that need them carry them in envelope
properties (the sync protocol stashes them in segment properties).

## Canonical JSON

The canonical JSON for the content model is **protojson of the canonical
proto**, with the fixed options in `core/proto/content/v1/json.go`
(`MarshalJSON` / `MarshalJSONIndent` / `UnmarshalJSON`):

- lowerCamelCase protojson JSON names (`pc_open` → `"pcOpen"`); proto-name
  spellings are accepted on unmarshal but never emitted;
- unpopulated fields omitted; bytes as base64 strings; deterministic output
  (protojson's deliberate whitespace instability is normalized away);
- unknown fields ignored on unmarshal, so older consumers read newer
  producers (forward compatibility across appended fields).

The encoding is locked by golden files under `core/proto/content/v1/testdata/`
(`json_test.go`): marshal must reproduce the checked-in bytes, and the
checked-in bytes must keep decoding to the same messages. A failure there is a
wire-compatibility break. `core/model`'s own JSON struct tags follow the same
camelCase convention (`model.AltTranslation` was converted from snake_case;
its `UnmarshalJSON` accepts the legacy keys for payloads persisted before the
switch and for released okapi-bridge JARs).

Note that the model's in-process Run JSON (`model.Run.MarshalJSON`, RFC 0001 —
flat `{"text":"literal"}` text runs) is a distinct, stable encoding used
*inside* projections (ContentTree, KBF, flow traces); the wire form nests the
text payload (`{"text":{"text":"literal"}}`).

## Generated artifacts

`scripts/gen-contract-types` (drift-gated by `make check-contract-types` in
`.github/workflows/reference-data-drift.yml`) derives from the canonical
schema:

- **TypeScript types** — `packages/contract-types/src/content.gen.ts`: the
  wire shapes as they appear in canonical protojson (rendered from the proto
  descriptors; `RunMessage` and `ContentRef` as discriminated unions), plus
  the model.Run JSON and the core/editor ContentTree projection shapes
  (reflected from the Go structs). Frontend packages import these instead of
  hand-mirroring the model; `@neokapi/ui-primitives/preview` layers only
  deliberate, documented refinements on top.
- **JSON Schema** — `core/proto/content/v1/content.schema.json` (draft
  2020-12), generated from the proto descriptors for non-proto consumers. The
  protojson golden files are the binding contract; the schema is a generated
  convenience.

## Consumers

- **Plugin bridge** (`core/plugin/proto/v2`) — imports the canonical package;
  the Java okapi-bridge compiles both files. `content.proto` keeps
  `java_package = "neokapi.bridge.proto"` so the generated Java classes are
  unchanged.
- **Sync protocol** (`bowrain/core/proto/sync/v1`) — `SyncBlock` is a
  sync-specific envelope (item scoping, content/expected hashes, JSON escapes
  for annotations/skeleton/display-hint) around canonical `SegmentMessage`
  payloads. `bowrain/core/sync` delegates all run/segment conversion to
  `protoconvert` and owns only the envelope and the Merkle hash helpers
  (`ComputeItemHash`, `ComputeRootHash`).

## Projections

A projection is a deliberately lossy, purpose-built serialization. It must be
labeled as such where it is defined, and it is never the model contract:

- **`core/editor.BlockIndex`** — runs flattened to `source`/`source_html`
  strings for editor listings; versioned by `kat_version`.
- **`core/editor.ContentTree`** — the run-native anatomy tree used by the
  WASM lab explorers.
- **`core/structrec.Record`** — the flat anchor record behind `kapi inspect`,
  conversion, and RAG export.
- **`bowrain/proto/v1` editor messages** (`EditorRun` family, the store
  service's flat `BlockMessage`) — the bowrain desktop's gRPC surface;
  frozen (no new fields) and slated to be retired when the desktop moves to
  the REST/sync client.

## Compatibility policy

- Field numbers in `neokapi.content.v1` are frozen: never renumbered,
  never repurposed; removed fields become `reserved`.
- Field names are stable (protojson and cross-language peers depend on them).
- New fields append with fresh numbers.
- Proto *package* moves do not affect the wire: encoding depends on field
  numbers and types only. Fully-qualified message names matter solely for
  `google.protobuf.Any` packing and descriptor-based reflection, neither of
  which is used on these messages — and the guard test keeps it that way by
  keeping definitions in one place.

## Enforcement

`core/proto/content/guard_test.go` scans every `.proto` in the repo and fails
on any message whose name defines Block/Run content outside the canonical
file, modulo an explicit allowlist of the labeled projections and the sync
envelope. A second check fails when an allowlisted message disappears, so the
allowlist shrinks in step with the code.

The **round-trip** side of the contract — that a `model.Block` survives the
kapi↔bowrain sync wire (push → store → pull) losslessly — is enforced by the
kitchen-sink conformance and drift-guard tests documented in
[Content-Model Parity Over the Sync Wire](/contribute/implementation/content-parity).
