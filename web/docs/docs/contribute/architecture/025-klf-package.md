---
id: 025-klf-package
sidebar_position: 25
title: "AD-025: KLF Family and the .klz Package"
description: "Architecture decision: a family of deterministic, lossless KLF formats (blocks, translation memory, termbase) and a .klz package container that bundles a project's authoritative content for portable, lossless pack/unpack — distinct from the lossy industry interchange formats (XLIFF/PO, TMX, TBX)."
keywords: [KLF, klftm, klftb, klz, package, translation memory, termbase, TMX, TBX, lossless, interchange, content store, cache]
---

# AD-025: KLF Family and the .klz Package

## Summary

neokapi defines a **KLF family** of native, deterministic, lossless
serialization formats — one per content atom — and a **`.klz` package
container** that bundles them into a portable, lossless snapshot of a project's
authoritative content:

| Atom | Native format (lossless) | Package member | Interchange format (lossy) |
| --- | --- | --- | --- |
| blocks + targets | KLF (`core/klf`, `.klf`) | `blocks/*.klf` | XLIFF / PO |
| stand-off annotations | KLF annotations (`.klfl`) | `annotations/*.klfl` | — |
| translation memory | KLF-TM (`sievepen/klftm`) | `tm.klftm` | TMX |
| termbase | KLF-TB (`termbase/klftb`) | `termbase.klftb` | TBX |
| media | opaque blobs | `media/*` | — |

The package is a deterministic zip with a `manifest.json` carrying a per-member
SHA-256 and a Merkle `rootHash`. It is the **at-rest twin** of the over-the-wire
sync chunk set (`bowrain/core/proto/sync/v1`, content types
`blocks / annotations / tm / termbase / media`).

## Context

KLF (AD: the content model serialization) already gives blocks a deterministic,
hashable, lossless on-disk form. But the other content a project owns had no
equivalent:

- **Translation memory** had only `sievepen` TMX export — an *interchange*
  format. TMX preserves the multilingual variants a CAT tool understands but
  **silently drops** the AI-native enrichments `sievepen.TMEntry` carries:
  entity mappings (including the `ConceptID` cross-link to the termbase),
  provenance origins and import sessions, per-entry properties, and notes.
- **Termbase** had TBX export and an ad-hoc JSON export. TBX maps the standard
  terminology fields but **drops** `termbase.Concept`'s native fields: the term
  `Source` (terminology vs `brand_vocabulary`), the `CompetitorTerm` flag, and
  the extensible `Properties` map.

So there was no lossless way to serialize a whole project — to back it up, move
it between machines, seed a fresh server or an offline desktop working copy, or
build a deterministic test fixture. Critically, a lossy serialization cannot
faithfully **regenerate the caches** (`blocks.db`, sync hashes, the redis hash
cache) the platform builds from this content.

A second observation shaped the design: the project layout already separates
**authoritative state** (TM, termbase, manifest at the top of `.kapi/`) from
**regenerable cache** (`cache/blocks.db`, extractions, `sync-cache.json` — "safe
to delete and rebuild"). The thing worth packaging is the *authoritative
content*, never the cache or secrets.

## Decision

### 1. A KLF family of native, lossless formats

Each content atom gets a native format that round-trips its full model and
shares the KLF discipline — a `kind` magic string, a `MAJOR.MINOR`
`schemaVersion` with the reject-unknown-major / accept-unknown-minor contract,
and a **deterministic** encoder (sorted keys/records, no HTML escaping, trailing
newline) so output is stable for content hashing and git diffing.

- **KLF-TM** (`sievepen/klftm`, `kind: kapi-tm-format`). Wire DTOs mapped
  to/from `sievepen.TMEntry`; variant content reuses the canonical `model.Run`
  serialization, so inline codes, placeholders, and plural/select survive
  identically to KLF blocks. Carries entities (with `ConceptID`), origins,
  import sessions, properties, and notes.
- **KLF-TB** (`termbase/klftb`, `kind: kapi-termbase-format`). Reuses the
  already-JSON-tagged `termbase.Concept` directly — one source of truth, no
  parallel wire type to drift — and so preserves `Source`, `CompetitorTerm`, and
  `Properties`.

### 2. The `.klz` package container

`klz` (`github.com/neokapi/neokapi/klz`) defines the `.klz` format: a
deterministic zip (stored, fixed timestamps, sorted entries) containing a
`manifest.json` plus one member per content type. The manifest lists each
member with its content type and SHA-256, and a Merkle `rootHash` over the
sorted member hashes gives the package a stable content identity independent of
zip framing. `Unmarshal` validates the envelope, every member checksum, and the
root hash.

### 3. Two tiers: native vs interchange

- **Native (lossless)** — the KLF family and `.klz`. Used for packing,
  caching, hashing, and any flow that must reconstruct project state exactly.
- **Interchange (lossy)** — XLIFF/PO for blocks, TMX for TM, TBX for termbase.
  Used to hand content across an organizational boundary into the wider
  localization industry. These remain the export/handoff path and are **never**
  package members, because they cannot represent neokapi's native fields.

### 4. Pack authoritative content, not caches

A `.klz` bundles the authoritative content; unpacking re-seeds the stores and
lets the regenerable caches rebuild. It **excludes** regenerable caches
(`blocks.db`, the sync hash cache) and secrets (the `sync-cache.json` claim
token). This makes the package the at-rest equivalent of the sync wire protocol:
packing is the sync converters writing files instead of protobuf chunks.

## Consequences

- A project's full content can be serialized losslessly and rehydrated into
  fresh stores. The guarantee is enforced by a cache-internal round-trip test:
  populate real `sievepen` / `termbase` stores → pack to `.klz` → unpack into
  fresh stores → re-pack → assert byte-identical.
- TMX/TBX keep their role unchanged (industry interchange), and their lossiness
  is now a documented, intentional property rather than an accident.
- The KLF family stays cohesive: every member is deterministic and hashable, so
  a `.klz` — and each member — has a stable content hash, and the same Merkle
  diff the sync engine runs over the wire applies to packages at rest.
- KLF itself is unchanged; the family composes around it rather than growing it.

## Implementation

- Formats: `sievepen/klftm`, `termbase/klftb`, container `klz/`.
- Tests: per-format lossless round-trip + determinism + envelope rejection, and
  `klz` package round-trip + a cache-internal store round-trip.
- Reference: [KLF family & the .klz package](/reference/klf/package).
