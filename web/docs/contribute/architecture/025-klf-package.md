---
id: 025-klf-package
sidebar_position: 25
title: "AD-025: KLF Family and the .klz Package"
description: "Architecture decision: a family of deterministic, lossless KLF formats (blocks, translation memory, termbase) and a .klz package container that bundles a project's authoritative content for portable, lossless pack/unpack — distinct from the lossy industry interchange formats (XLIFF/PO, TMX, TBX). A .klz also carries a project's in-progress working state for hand-off and cached resume, with progress derived from content rather than an authoritative journal, plus the full project recipe (flows, plugins, defaults, content) so it is a runnable project in a file — near-full parity with a .kapi project, excluding secrets, caches, plugin binaries, and (by default) raw source. The same container serves two profiles — a whole-project snapshot (pack/unpack) and a task-scoped bilingual interchange file (extract/merge), neokapi's lossless interchange format for a translator or reviewer. A .klz is always a parcel, never a workspace: day-to-day work is the ambient .kapi project."
keywords: [KLF, klftm, klftb, klz, package, translation memory, termbase, TMX, TBX, lossless, interchange, content store, cache, pack, unpack, working state, hand-off]
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

### 5. A `.klz` carries working state for hand-off and resume

A `.klz` is both an at-rest snapshot of finished content **and** a carrier of
**in-progress working state**, so work can stop, move between machines, and resume
where it left off. The design delivers this through existing-grain mechanisms
rather than a step-by-step CLI verb family:

- **`.klz` as an ad-hoc workspace, the git-bundle model.** A `.klz` is the
  portable *bundle*; the runtime is a persistent **shadow cache** under
  `$XDG_CACHE_HOME/kapi/klz/<key>`, keyed by the `.klz`'s absolute path, so the
  working directory stays a single file. Three pipeline-stage verbs (no project):
  `extract <sources> -o work.klz` ingests the sources and records a recipe (§6 —
  the same schema as a `.kapi` file; an ad-hoc extract fills only target locales +
  output layout, but the slot holds a full recipe); running any tool or `run` flow *on* the `.klz`
  **transforms it in place** against the cache's persistent per-source block stores
  — incrementally, *without rewriting the `.klz`*; and `merge work.klz` emits the
  finished documents from the cache (hydrating stored target overlays, one file per
  source × locale). The `.klz` is rewritten only by `kapi pack work.klz` (or a
  transform's `--pack`) — the explicit eject; `kapi info work.klz` reports whether
  the cache is **dirty** (its content `RootHash` differs from the packed `.klz`).
  Block ids are only unique within one document, so each source has its own store
  and each overlay is tagged with its source (`OverlayDoc.Source`). Transforming
  reuses overlays already present rather than recomputing (the cache is the cache),
  so output equals a one-shot run. (§7 frames the mental model: a `.klz` is a
  parcel *opened into* a working project, not a place you author in — the in-place
  transform is just the shadow cache making open → work → pack cheap. Day-to-day
  work is the ambient `.kapi` project.)
- **Cached resume (project).** A project run executes against the project's
  persistent block store (`core/blockstore` at `.kapi/cache/blocks.db`, wired via
  `flow.WithBlockStore`). Because the store is append-only and content-addressed —
  a tool appends an *overlay* keyed by `(kind, blockHash)` rather than rewriting a
  block — a `SessionTool` caches its per-block result and hydrates from it on a
  later run. Re-running a flow therefore **skips work already done**; the store
  *is* the workspace, resume is just running again.
- **Project snapshot (`pack` / `unpack`).** For the whole project, `pack` exports
  the block-store overlays, the authoritative TM and termbase, the source
  identity + skeletons, and the **full project recipe** (flows, plugins,
  defaults, content — §6) into a portable `.klz`; `unpack` rehydrates it into
  another machine's `.kapi/` state dir, reconstituting a complete, runnable
  `<name>.kapi`. A `.klz` is to the state directory what a git *bundle* is to
  `.git` — and, because it carries the recipe, a *runnable* one.

**Progress is derived from content, not recorded in an authoritative journal.**
Because the store is content-addressed, "has step X run?" is a pure function of
the content: *does X's overlay exist, anchored to the current block hashes?*
That is what makes cached resume correct and idempotent — re-running is a no-op
where the overlay is present; a changed source re-hashes its block so only the
affected work recomputes; and crash safety is automatic, since a crash that did
not commit an overlay simply leaves it absent and the next run redoes it, with
nothing to reconcile. An authoritative progress journal is deliberately
**avoided**: it would be a second source of truth that can drift from the content
(the dual-state footgun this codebase avoids — `sync-cache.json` and `blocks.db`
are both explicitly regenerable). A journal cannot survive the content changing
underneath it (a re-hashed block silently invalidates a "done" claim), so making
it correct means re-deriving the content-addressing the store already provides.

The one durable record beyond content is **advisory provenance**: `pack --log`
appends a hash-chained line to `history.jsonl` recording the pack, giving a
hand-off a tamper-evident custody trail (`unpack` verifies the chain and warns on
a break). It is strictly subordinate to content — excluded from the package
`rootHash`, never read to decide anything, and safe to delete with no loss of
work; a default `pack` is byte-deterministic. (A journal *is* the right tool
where an action has effects outside the content — sent mail, a charged card, a
paid API call; those belong to the authz/audit subsystem, not to progress
tracking, whose state is wholly in the overlays.)

### 6. What a `.klz` carries: parity with a `.kapi` project

A `.klz` is the portable twin of a `.kapi` project, so it carries the project's
**portable authoritative state** — both its content and its committed intent —
and nothing environment-specific. One principle decides membership:

> Pack authoritative state, not caches or secrets. **Content** defines the
> package identity (the Merkle `rootHash`); the **recipe** is metadata (excluded
> from `rootHash`, as the workspace recipe already is). Secrets never travel —
> they live in the OS keychain, never in a recipe. Caches are regenerated on
> unpack. A `.klz` has no runtime, so any side-effecting recipe (`hooks`,
> `automations`, a `server:` binding) travels **inert** and re-activates only when
> unpacked into a project, with explicit re-auth and opt-in re-arming.

**Intent travels as the whole recipe.** A `.klz` embeds the project recipe
verbatim — `flows`, `plugins` / `requires`, `defaults`, `content`, `preset`, and
the platform `Extras` — using the same schema as a `.kapi` file, so there is one
source of truth and no parallel intent model. Flows are ordinary framework intent
(`flow.StepsSpec`), so they travel like any other recipe field: a standalone
`.klz` is runnable with its own named flows (`kapi run <flow> work.klz`), and
`unpack` reconstitutes a complete `<name>.kapi`. This is what makes a `.klz` a
**project in a file** ([AD-026](026-flow-io-binding.md) — a flow is portable
composition, carrying no I/O of its own).

**Source travels as identity + skeleton; raw bytes are opt-in.** A `.klz` always
records each source's **identity** (logical path, format, content hash) and the
per-source **skeleton** — the round-trip template `merge` reuses. That is enough
for the core loop: `transform`-in-place reads only blocks and overlays, `merge`
rebuilds the localized files from the skeleton, and `info` / `status` detects
drift from the source hash. The **raw source bytes** are needed only to
*re-extract* (re-derive blocks under different settings), so they are embedded
only on request (`pack --with-source` / `extract --with-source`), keeping a
default `.klz` from duplicating git-tracked source. The skeleton is the *derived
extract*, not the original document.

| Concern | In a `.kapi` project | In a `.klz` | Disposition |
| --- | --- | --- | --- |
| flows | `flows:` + `.kapi/flows/` | recipe `flows` | **travels** |
| plugins (declaration) + `requires` | recipe | recipe | travels (binaries re-resolved via registry) |
| defaults, content, preset | recipe | recipe | travels |
| `server:` / `hooks:` / `automations:` (Extras) | recipe Extras | recipe Extras | travels **inert** |
| TM / termbase | `tm.db` / `termbase.db` (authoritative) | `tm.klftm` / `termbase.klftb` | travels (lossless) |
| blocks + targets, annotations, in-progress overlays | `cache/blocks.db` (regenerable) | `blocks/*.klf`, `annotations/*.klfl`, `overlays.klfo` (authoritative) | travels |
| source identity (path, format, hash) | working tree | `manifest.json` | travels |
| source skeleton (round-trip template) | `cache/extractions/.../skel-*.bin` | `skeletons/<id>` | travels |
| raw source bytes | working tree `src/` | `source/<name>` | opt-in (`--with-source`) |
| secrets (auth tokens, API keys) | OS keychain | — | **never travels** |
| caches (`blocks.db`, `sync-cache.json`, extractions, collections) | `cache/` | — | regenerated on unpack |
| plugin binaries | user / system install | — | re-resolved via `requires` / registry |
| provenance | — | `history.jsonl` (opt-in) | travels (excluded from `rootHash`) |

**Source of truth on round-trip.** When a `.klz` is unpacked into or sits beside a
`.kapi` project, the on-disk recipe is authoritative and the package is a
snapshot; a standalone `.klz` (the ad-hoc workspace) is authoritative in itself.
Intent therefore never has two live homes that can drift.

### 7. Boundaries: workspace vs payload, and the two `.klz` profiles

Day-to-day work happens in an **ambient `.kapi` project**, discovered by a
git-style upward walk ([AD-008](008-project-model.md)) — never named on a command.
A `.klz` is a **parcel**: a thing that crosses a boundary, named only at that
boundary. You do not *work inside* a `.klz`; receiving one and working on it means
opening it into a project (`unpack`, or an in-place open backed by the shadow
cache of §5), then `pack` to ship again. This is git's split between a *working
tree* (ambient) and a *bundle* (named only at create / clone), and it is why the
everyday loop never types a `.klz` path.

Which parcel crosses which boundary:

| Boundary | Parcel | Fidelity | Verbs |
| --- | --- | --- | --- |
| Time / space, in-ecosystem (backup, transfer, seed a server) | **project `.klz`** (whole project, §6) | lossless native | `pack` / `unpack` |
| To a translator or reviewer | **bilingual `.klz`** (one locale pair, below) | lossless native | `extract` / `merge` |
| To a third-party CAT tool | XLIFF 2.x / PO ([AD-017](017-bilingual-format-interop.md)) | interoperable, lossy | `extract` / `merge` |
| To the live server | sync wire protocol (the `.klz`'s over-the-wire twin) | lossless, streamed | `push` / `pull` |

So one `.klz` container carries **two profiles**, distinguished by the manifest
`kind`:

- **Project profile** (`kind: kapi-project`) — the whole project: all locales,
  full recipe, TM, termbase, overlays, source identity + skeletons (§6). The
  *snapshot / ecosystem payload*, moved by `pack` / `unpack`.
- **Bilingual profile** (`kind: kapi-interchange`) — a task-scoped slice for one
  source→target pair: the blocks with faithful inline codes, the
  segmentation/alignment overlays, the per-source skeleton for round-trip, and the
  relevant TM-match + termbase context. It excludes other locales, the full
  recipe, and raw source. This is **neokapi's interchange format** — the parcel
  `extract` sends to a translator or reviewer and `merge` ingests back
  ([AD-017](017-bilingual-format-interop.md)) — lossless where XLIFF is lossy, with
  inline TM/term context and integrity-verified, diffable review. It is *ecosystem*
  interchange (read by a neokapi tool); XLIFF / PO remain the industry-interop
  tier, and turning this profile into a cross-vendor standard is an open-spec
  effort, not a property of the format.

Both profiles are parcels — neither is a workspace.

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
- A `.klz` can carry in-progress work (overlays), so a project's working state
  can be handed off (`pack`/`unpack`) and a flow resumed against the warm
  block-store cache — with progress derived from content rather than a journal,
  the server-less twin of the platform's stateful project.
- Because a `.klz` carries the full recipe (§6), it is a **project in a file**: a
  standalone package runs its own flows and `unpack` rebuilds a complete `.kapi`.
  Near-full parity with a `.kapi` project follows, the deliberate gaps being
  secrets (never), caches (regenerated), plugin binaries (re-resolved), and raw
  source (opt-in). Side-effecting recipe travels inert, so receiving a `.klz`
  cannot trigger a server call, hook, or automation until it is adopted into a
  project.
- The same `.klz` container serves two profiles (§7): a whole-project snapshot
  (`pack`/`unpack`) and a task-scoped **bilingual interchange** file
  (`extract`/`merge`) — neokapi's lossless interchange format for a translator or
  reviewer. A `.klz` is always a *parcel*, never a *workspace*: day-to-day work is
  the ambient `.kapi` project ([AD-008](008-project-model.md)), so the everyday
  loop never names a `.klz`.

## Implementation

- Formats: `sievepen/klftm`, `termbase/klftb`, container `klz/`.
- Tests: per-format lossless round-trip + determinism + envelope rejection, and
  `klz` package round-trip + a cache-internal store round-trip.
- The working-state / hand-off capability (§5) is implemented
  ([GitHub issue #787](https://github.com/neokapi/neokapi/issues/787)): the
  `.klz` carries `overlays.klfo` (in-progress overlays) + `source/<name>` + a
  manifest `recipe`; the block-store exporter/loader (`core/blockstore/exporter`)
  is the inverse of the importer; `flow.FileRunner` runs against a persistent
  store. The ad-hoc workspace verbs live in `cli/klzworkspace.go` —
  `extract`/transform-in-place/`merge`, dispatched from the `extract`, `merge`,
  `run`, and tool commands — and `pack` / `unpack` snapshot and rehydrate a whole
  project's state. Progress is derived from the overlays present (no journal); the
  optional advisory `history.jsonl` (hash-chained, opt-in `pack --log`, verified
  on `unpack`) is
  excluded from the content `rootHash`. The package embeds the full project recipe
  (§6, side-effecting `Extras` inert) and retains source as identity + skeleton,
  with raw bytes behind `--with-source` (§7). Covered by unit tests (klz overlays +
  history-chain round-trip and tamper detection; the exporter store round-trip),
  the `kapi/e2e` suite (pack/unpack round-trip, cached-resume byte-equality,
  pack determinism, provenance log), and the `make klz-smoke` headless gate.
- Reference: [KLF family & the .klz package](/reference/klf/package).
