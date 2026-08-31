---
id: f-04-wire-schema
sidebar_position: 4
title: "F-04: The content-model wire schema"
description: "core/proto/content/v1 is the single canonical serialization of the content model; every other serialized shape is either an import of that schema or an explicitly-labeled projection, and a guard test enforces the boundary."
keywords: [wire schema, protobuf, protojson, content model, canonical serialization, protoconvert, projection, compatibility policy, architecture decision, neokapi]
---

# F-04: The content-model wire schema

## Summary

The content model has exactly one canonical wire representation: the protobuf
messages in `core/proto/content/v1/content.proto` (proto package
`neokapi.content.v1`), converted to and from `core/model` by
`core/plugin/protoconvert`. Every surface that serializes Parts, Blocks, or Runs
either imports this schema or is an explicitly-labeled **projection**: a lossy,
purpose-built shape that is never a model contract. A guard test rejects new
Block, Run, or Segment message definitions outside the canonical package.

## Context

A content model that crosses process boundaries (plugin subprocesses, generated
frontend types, JSON on disk) invites a second definition of itself at every
crossing. Two definitions drift, and the drift is silent: each side keeps
compiling while the fields they disagree about quietly vanish. The remedy is a
single definition plus a test that fails when a second one appears.

## Decision

### The canonical schema

`core/proto/content/v1/content.proto` defines the complete, symmetric message set
for the model: `PartMessage`, `BlockMessage`, the `RunMessage` discriminated union
(`TextRunMessage`, `PlaceholderRunMessage`, `PcOpenRunMessage`,
`PcCloseRunMessage`, `SubRunMessage`, `PluralRunMessage`, `SelectRunMessage`),
`OverlayMessage` with `SpanMessage` / `AnchorMessage` / `VariantMessage`,
`SegmentMessage` and `TargetEntry`, `SkeletonMessage` and `SkeletonPartMessage`,
`DisplayHintMessage`, `LayerMessage`, `DataMessage`, `GroupStartMessage` and
`GroupEndMessage`, `MediaMessage`, `AnnotationEntry`, the lightweight
`ContentBlock`, and `ContentRef`.

These messages are proven cross-language in production by the Java bridge plugin.
Keeping them in a dedicated package rather than inside a plugin proto makes the
model schema a first-class artifact rather than nominally a plugin detail. The
bridge service proto (`core/plugin/proto/v2/neokapi_bridge.proto`) imports the
canonical package and keeps only the `BridgeService` definition and its
request/response envelopes; the generated Go package re-exports the content types
as aliases, so plugin hosts compile unchanged.

`core/plugin/protoconvert` is the canonical converter: every message kind has a
symmetric `XToProto` / `ProtoToX` pair, and its test suite (in particular the
compat corpus in `core/plugin/protoconvert/compat_test.go`) is the compatibility
oracle. Model to proto to model is identity for every Run kind, for overlays, for
multi-locale targets, segmentation, skeleton refs, display hints, and registered
annotations.

Three things do not cross this schema: target variant tone and channel (targets
are keyed by locale in `TargetEntry`), per-target status, origin, and score, and
the block's durable `Unit` key ([F-03](f-03-identity.md)). A protocol that needs
them carries them in its own envelope rather than widening the canonical
messages.

### Canonical JSON

The canonical JSON for the content model is **protojson of the canonical proto**,
with the fixed options in `core/proto/content/v1/json.go` (`MarshalJSON`,
`MarshalJSONIndent`, `UnmarshalJSON`):

- lowerCamelCase protojson JSON names (`pc_open` becomes `"pcOpen"`); proto-name
  spellings are accepted on unmarshal but never emitted;
- unpopulated fields omitted; bytes as base64 strings; `int32` as JSON numbers;
- deterministic output: protojson randomizes its whitespace, so both
  marshal helpers re-serialize through `encoding/json`, which preserves
  protojson's stable field order;
- unknown fields ignored on unmarshal, so an older consumer reads a newer
  producer's output.

The encoding is locked by golden files under `core/proto/content/v1/testdata/`:
marshalling must reproduce the checked-in bytes, and the checked-in bytes must
keep decoding to the same messages. A failure there is a wire-compatibility break,
not a formatting nit.

`core/model`'s own JSON struct tags follow the same camelCase convention.
`model.AltTranslation.UnmarshalJSON` additionally accepts the snake_case spelling
that released bridge builds emit, with camelCase winning where a document carries
both.

The model's in-process Run JSON (`model.Run.MarshalJSON`, flat
`{"text":"literal"}` text runs) is a distinct, stable encoding used *inside*
projections such as the content tree, KBF, and flow traces. The wire form nests
the text payload as `{"text":{"text":"literal"}}`. The two are not
interchangeable, and neither is a bug in the other.

### Generated artifacts

`scripts/gen-contract-types`, drift-gated by `make check-contract-types` in the
reference-data-drift workflow, derives two artifacts from the canonical schema:

- **TypeScript types**: `packages/contract-types/src/content.gen.ts`: the wire
  shapes as they appear in canonical protojson, rendered from the proto
  descriptors, with `RunMessage` and `ContentRef` as discriminated unions; plus
  the `model.Run` JSON and the content-tree projection shapes, reflected from the
  Go structs. Frontend packages import these rather than hand-mirroring the model.
- **JSON Schema**: `core/proto/content/v1/content.schema.json` (draft 2020-12),
  generated from the proto descriptors for non-proto consumers. The protojson
  golden files are the binding contract; the schema is a generated convenience.

### Projections

A projection is a lossy, purpose-built serialization. It must be
labeled as such where it is defined, and it is never the model contract:

- **`core/editor.BlockIndex`**: runs flattened to plain and HTML strings for
  editor listings; versioned independently by its own version field.
- **`core/editor.ContentTree`**: the run-native anatomy tree behind the
  in-browser explorers.
- **`core/structrec.Record`**: the flat anchor record behind inspection,
  conversion, and retrieval export.

The framework defines one envelope of its own: the sync wire,
`core/proto/sync/v1/sync.proto`, whose `SyncBlock` and `SyncSegmentList` are
allow-listed in the guard by message name. It adds item scoping, hashes, the
block's durable `unit` key and `*_json` escapes around canonical payloads, and
`core/venue` (`BlockToProto` / `ProtoToBlock`) delegates every run and segment
conversion to `protoconvert`. An envelope adds context around canonical
payloads; it never redefines them.

### Compatibility policy

- Field numbers in `neokapi.content.v1` are frozen: never renumbered, never
  repurposed. Removed fields become `reserved`.
- Field names are stable, because protojson and cross-language peers depend on
  them.
- New fields append with fresh numbers.
- Proto *package* moves do not affect the wire: encoding depends on field numbers
  and types only. Fully-qualified message names matter solely for
  `google.protobuf.Any` packing and descriptor-based reflection, neither of which
  is used on these messages, and the guard test keeps it that way by keeping the
  definitions in one place.

### Enforcement

`core/proto/content/guard_test.go` walks every `.proto` in the repository and
fails on any message whose name defines Block, Run, or Segment content outside the
canonical file. The match is on message *names* (anything ending in `Run`,
`Block`, or `Segment`, bare or with an `s`, `List`, or `Message` suffix), so a
request/response envelope named `SegmentRequest` does not trip it while a
`BlockMessage` in the wrong file does. Agent worktrees under `.claude` are skipped,
since a whole checkout of this repository is the same file rather than a second
definition of it.

An explicit allowlist maps a proto file to the specific message names it may
define, each entry an envelope around canonical payloads. A second check fails when
an allowlisted message *disappears*, so the allowlist shrinks in step with the
code rather than accumulating stale exemptions.

## Consequences

- There is one place to change the content model's serialization, and one test
  that notices when someone adds a second.
- Frontend types and JSON Schema cannot drift from Go, because both are generated
  and drift-gated in CI.
- A wire-compatibility break announces itself as a golden-file failure at the
  moment of the change, rather than as a decode error in another process weeks
  later.
- Adding a field is cheap and safe (append a number, regenerate), while renaming
  or renumbering one is a deliberate, visible break.
- Consumers that need a shape the canonical schema does not carry write a labeled
  projection instead of widening the contract, which keeps the contract small.

## See also

- [F-02: The content model](f-02-content-model.md): the types this schema serializes
- [F-03: Identity](f-03-identity.md): the hashes the wire carries
- [E-05: The plugin system](../engine/e-05-plugin-system.md): the bridge service that imports this schema
- [M-06: Content packages](../multilingual/m-06-content-packages.md): the KBF bundle and its use of the in-process Run JSON
- [S-06: The visual editor data model](../surfaces/s-06-visual-editor.md): the editor projections
- [Plugin protocol v1](/contribute/implementation/engine/plugin-protocol-v1): the contract plugin repositories verify against
