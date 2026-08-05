---
id: 025-kbf-package
sidebar_position: 25
title: "AD-025: KBF Family and the .kpz Package"
description: "Architecture decision: a family of deterministic, lossless KBF formats (blocks, content memory, terms) and a .kpz package container that bundles a project's authoritative content for portable, lossless pack/unpack — distinct from the lossy industry interchange formats (XLIFF/PO, TMX, TBX). A .kpz also carries a project's in-progress working state for hand-off and cached resume, with progress derived from content rather than an authoritative journal, plus the full project recipe (flows, plugins, defaults, content) so it is a runnable project in a file — near-full parity with a .kapi project, excluding secrets, caches, plugin binaries, and (by default) raw source. The same container serves two profiles — a whole-project snapshot (pack/unpack) and a task-scoped bilingual interchange file (extract/merge), neokapi's lossless interchange format for a translator or reviewer. A .kpz is always a parcel, never a workspace: day-to-day work is the ambient .kapi project."
keywords: [KBF, memory bundle, terms bundle, kpz, package, content memory, terms store, TMX, TBX, lossless, interchange, content store, cache, pack, unpack, working state, hand-off]
---

# AD-025: KBF Family and the .kpz Package

## Summary

neokapi defines a **Kapi format family** of native, deterministic, lossless
serialization formats — one per content atom — and a **`.kpz` package
container** that bundles them into a portable, lossless snapshot of a project's
authoritative content:

| Atom | Native format (lossless) | Package member | Interchange format (lossy) |
| --- | --- | --- | --- |
| blocks + targets | KBF (`core/kbf`, `.kbf.json`) | `blocks/*.kbf.json` | XLIFF / PO |
| stand-off annotations | KBF annotations (`.overlays.jsonl`) | `annotations/*.overlays.jsonl` | — |
| content memory | memory bundle (`memory/kmb`, `.memory.json`) | `memory.json` | TMX |
| terms | terms bundle (`terms/ktb`, `.terms.json`) | `terms.json` | TBX |
| media | opaque blobs | `media/*` | — |

Every member of the family is JSON, and its suffix says so. A marker segment
ahead of `.json` (`.kbf.json`, `.memory.json`, `.terms.json`) keeps the file
self-describing while `jq`, GitHub, and editor syntax highlighting still see
what they are looking at; a bare `.kbf` or `.kmb` would buy the marker at the
cost of every tool that reads JSON by extension. Only `.kpz` keeps a dedicated
extension, because it is a binary zip nobody hand-edits.

The memory and terms members carry the conventional bare names rather than the
compound suffix, so unzipping a package by hand yields the spelling the rest of
the tooling teaches. The names are **presentation only**: `Unmarshal` routes
every member by its manifest `contentType`, never by filename, so a member's
path can change without changing how a package is read.

The package is a deterministic zip with a `manifest.json` carrying a per-member
SHA-256 and a Merkle `rootHash`. It is the **at-rest twin** of the over-the-wire
sync chunk set (`bowrain/core/proto/sync/v1`, content types
`blocks / terms / memory / media`).

## Context

KBF (AD: the content model serialization) already gives blocks a deterministic,
hashable, lossless on-disk form. But the other content a project owns had no
equivalent:

- **Content memory** had only `memory` TMX export — an *interchange*
  format. TMX preserves the multilingual variants a CAT tool understands but
  **silently drops** the AI-native enrichments `memory.Entry` carries:
  entity mappings (including the `ConceptID` cross-link to a term concept),
  provenance origins and import sessions, per-entry properties, and notes.
- **Terms** had TBX export and an ad-hoc JSON export. TBX maps the standard
  terminology fields but **drops** `terms.Concept`'s native fields: the term
  `Source` (terminology vs `brand_vocabulary`), the `CompetitorTerm` flag, and
  the extensible `Properties` map.

So there was no lossless way to serialize a whole project — to back it up, move
it between machines, seed a fresh server or an offline desktop working copy, or
build a deterministic test fixture. Critically, a lossy serialization cannot
faithfully **regenerate the derived stores** (the project database, sync hashes,
the redis hash cache) the platform builds from this content.

A second observation shaped the design: the project layout already separates
**authoritative content** — the committed sources and the unit-decision record —
from everything a run derives from them, which is the project's one database
(`.kapi/work/store.db`, [AD-039](039-local-context-graph-store.md)) plus the
free-to-delete `.kapi/work/cache/`. The thing worth packaging is the *authoritative
content*, never the derived stores or secrets.

## Decision

### 1. A Kapi format family of native, lossless formats

Each content atom gets a native format that round-trips its full model and
shares the KBF discipline — a `kind` magic string, a `MAJOR.MINOR`
`schemaVersion` with the reject-unknown-major / accept-unknown-minor contract,
and a **deterministic** encoder (sorted keys/records, no HTML escaping, trailing
newline) so output is stable for content hashing and git diffing.

- **Content-memory bundle** (`memory/kmb`, `.memory.json`,
  `kind: kapi-memory`). Wire DTOs mapped
  to/from `memory.Entry`; variant content reuses the canonical `model.Run`
  serialization, so inline codes, placeholders, and plural/select survive
  identically to KBF blocks. Carries entities (with `ConceptID`), origins,
  import sessions, properties, and notes.
- **Terms bundle** (`terms/ktb`, `.terms.json`, `kind: kapi-terms`). Reuses the
  already-JSON-tagged `terms.Concept` directly — one source of truth, no
  parallel wire type to drift — and so preserves `Source`, `CompetitorTerm`, and
  `Properties`.

There is **one tier per store**: the bundle. A project **commits** it, because
reviewed memory and terms have to diff line by line in git, and the compound
`.json` suffix is what makes that diff readable in a browser and greppable with
`jq`. An earlier design added a second, compressed spelling per store — a zip
holding exactly one deflated member. It is withdrawn: `.kpz` already packages a
whole project's state for shipping, and a single bundle that wants to travel
small is what `zip` and `gzip` are for. A dedicated extension bought nothing the
general-purpose tools do not already give.

The **suffix**, not the location, is what identifies a bundle, because a project
is not limited to one per store. Terms is the exception that proves it: a project
has exactly one set of terms, so it gets a conventional location
([AD-010](010-terminology.md)) — `<root>/.kapi/context/terms.json`, then
`<root>/terms.json`. Content memory gets none, because a project accumulates a
bundle per content surface (this repository's own dogfood commits one per surface
under `.kapi/context/memory/`) and a fallback would have nothing single to name.

### 2. The `.kpz` package container

`kpz` (`github.com/neokapi/neokapi/kpz`) defines the `.kpz` format: a
deterministic zip (stored, fixed timestamps, sorted entries) containing a
`manifest.json` plus one member per content type. The manifest lists each
member with its content type and SHA-256, and a Merkle `rootHash` over the
sorted member hashes gives the package a stable content identity independent of
zip framing. `Unmarshal` validates the envelope, every member checksum, and the
root hash.

### 3. Two tiers: native vs interchange

- **Native (lossless)** — the Kapi format family and `.kpz`. Used for packing,
  caching, hashing, and any flow that must reconstruct project state exactly.
- **Interchange (lossy)** — XLIFF/PO for blocks, TMX for content memory, TBX for
  terms. Used to hand content across an organizational boundary into the wider
  translation industry. These remain the export/handoff path and are **never**
  package members, because they cannot represent neokapi's native fields.

### 4. Pack authoritative content, not derived stores

A `.kpz` bundles the authoritative content; unpacking re-seeds it and lets the
project database rebuild from there. It **excludes** everything derived (the
database's tables, the sync hash cache) and secrets (the `sync-cache.json` claim
token). This makes the package the at-rest equivalent of the sync wire protocol:
packing is the sync converters writing files instead of protobuf chunks.

Membership is decided **per table, not per file**. Because every subsystem shares
one database, "does this project have a terms store?" is a question about rows,
so `pack` carries only the parts that hold something: an empty subsystem
contributes no member, exactly as an absent one would.

### 5. A `.kpz` carries working state for hand-off and resume

A `.kpz` is both an at-rest snapshot of finished content **and** a carrier of
**in-progress working state**, so work can stop, move between machines, and resume
where it left off. The design delivers this through existing-grain mechanisms
rather than a step-by-step CLI verb family:

- **`.kpz` as an ad-hoc workspace, the git-bundle model.** A `.kpz` is the
  portable *bundle*; the runtime is a persistent **shadow cache** under
  `$XDG_CACHE_HOME/kapi/kpz/<key>`, keyed by the `.kpz`'s absolute path, so the
  working directory stays a single file. Three pipeline-stage verbs (no project):
  `extract <sources> -o work.kpz` ingests the sources and records a recipe (§6 —
  the same schema as a `kapi.yaml` recipe; an ad-hoc extract fills only target locales +
  output layout, but the slot holds a full recipe); running any tool or `run` flow *on* the `.kpz`
  **transforms it in place** against the cache's persistent per-source block stores
  — incrementally, *without rewriting the `.kpz`*; and `merge work.kpz` emits the
  finished documents from the cache (hydrating stored target overlays, one file per
  source × locale). The `.kpz` is rewritten only by `kapi pack work.kpz` (or a
  transform's `--pack`) — the explicit eject; `kapi info work.kpz` reports whether
  the cache is **dirty** (its content `RootHash` differs from the packed `.kpz`).
  Block ids are only unique within one document, so each source has its own store
  and each overlay is tagged with its source (`OverlayDoc.Source`). Transforming
  reuses overlays already present rather than recomputing (the cache is the cache),
  so output equals a one-shot run. (§7 frames the mental model: a `.kpz` is a
  parcel *opened into* a working project, not a place you author in — the in-place
  transform is just the shadow cache making open → work → pack cheap. Day-to-day
  work is the ambient `.kapi` project.)
- **Cached resume (project).** A project run executes against the project's
  persistent block store (`core/blockstore`, the block tables of `.kapi/work/store.db`,
  wired via `flow.WithBlockStore`). Because the store is append-only and
  content-addressed —
  a tool appends an *overlay* keyed by `(kind, blockHash)` rather than rewriting a
  block — a `SessionTool` caches its per-block result and hydrates from it on a
  later run. Re-running a flow therefore **skips work already done**; the store
  *is* the workspace, resume is just running again.
- **Project snapshot (`pack` / `unpack`).** For the whole project, `pack` exports
  the block-store overlays, the content memory and terms content, the
  source identity + skeletons, and the **full project recipe** (flows, plugins,
  defaults, content — §6) into a portable `.kpz`; `unpack` rehydrates it into
  another machine's `.kapi/` directory, reconstituting a complete, runnable
  `kapi.yaml`. A `.kpz` is to `.kapi/` what a git *bundle* is to `.git` — and,
  because it carries the recipe, a *runnable* one.

**Progress is derived from content, not recorded in an authoritative journal.**
Because the store is content-addressed, "has step X run?" is a pure function of
the content: *does X's overlay exist, anchored to the current block hashes?*
That is what makes cached resume correct and idempotent — re-running is a no-op
where the overlay is present; a changed source re-hashes its block so only the
affected work recomputes; and crash safety is automatic, since a crash that did
not commit an overlay simply leaves it absent and the next run redoes it, with
nothing to reconcile. An authoritative progress journal is deliberately
**avoided**: it would be a second source of truth that can drift from the content
(the dual-state footgun this codebase avoids — `sync-cache.json` and the block
tables are both explicitly regenerable). A journal cannot survive the content changing
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

### 6. What a `.kpz` carries: parity with a `.kapi` project

A `.kpz` is the portable twin of a `.kapi` project, so it carries the project's
**portable authoritative state** — both its content and its committed intent —
and nothing environment-specific. One principle decides membership:

> Pack authoritative state, not caches or secrets. **Content** defines the
> package identity (the Merkle `rootHash`); the **recipe** is metadata (excluded
> from `rootHash`, as the workspace recipe already is). Secrets never travel —
> they live in the OS keychain, never in a recipe. Caches are regenerated on
> unpack. A `.kpz` has no runtime, so any side-effecting recipe (`hooks`,
> `automations`, a `server:` binding) travels **inert** and re-activates only when
> unpacked into a project, with explicit re-auth and opt-in re-arming.

**Intent travels as the whole recipe.** A `.kpz` embeds the project recipe
verbatim — `flows`, `plugins` / `requires`, `defaults`, `content`, `preset`, and
the platform `Extras` — using the same schema as a `kapi.yaml` recipe, so there is one
source of truth and no parallel intent model. Flows are ordinary framework intent
(`flow.StepsSpec`), so they travel like any other recipe field: a standalone
`.kpz` is runnable with its own named flows (`kapi run <flow> work.kpz`), and
`unpack` reconstitutes a complete `kapi.yaml`. This is what makes a `.kpz` a
**project in a file** ([AD-026](026-flow-io-binding.md) — a flow is portable
composition, carrying no I/O of its own).

**Source travels as identity + skeleton; raw bytes are opt-in.** A `.kpz` always
records each source's **identity** (logical path, format, content hash) and the
per-source **skeleton** — the round-trip template `merge` reuses. That is enough
for the core loop: `transform`-in-place reads only blocks and overlays, `merge`
rebuilds the per-locale files from the skeleton, and `info` / `status` detects
drift from the source hash. The **raw source bytes** are needed only to
*re-extract* (re-derive blocks under different settings), so they are embedded
only on request (`pack --with-source` / `extract --with-source`), keeping a
default `.kpz` from duplicating git-tracked source. The skeleton is the *derived
extract*, not the original document.

`unpack` writes those raw bytes back into the working tree under their logical
paths, because being able to re-extract on the far side is the whole reason to
carry them — a flag that packed bytes nobody could get out again was a one-way
trip. A file already present is left alone (the working tree is authoritative on
its own sources, the same rule the recipe follows), and a member whose logical
path would escape the project root is refused rather than written.

| Concern | In a `.kapi` project | In a `.kpz` | Disposition |
| --- | --- | --- | --- |
| flows | `flows:` + `.kapi/flows/` | recipe `flows` | **travels** |
| plugins (declaration) + `requires` | recipe | recipe | travels (binaries re-resolved via registry) |
| defaults, content, preset | recipe | recipe | travels |
| `server:` / `hooks:` / `automations:` (Extras, any scope) | recipe Extras | recipe Extras | travels **inert** |
| path-valued fields (`terms_source`, `memory_source`, `redaction.rules`, `brand_voice.profile_file`, content `base` / `target`) | recipe | recipe | travels **contained** |
| content memory / terms | committed `.memory.json` / `.terms.json` sources | `memory.json` / `terms.json` | travels (lossless) |
| blocks + targets, annotations, in-progress overlays | block tables of `store.db` (derived) | `blocks/*.kbf.json`, `annotations/*.overlays.jsonl`, `overlays.json` (authoritative) | travels |
| review decisions | `.kapi/context/decisions/*.jsonl` (committed) | bilingual profile ([AD-017](017-bilingual-format-interop.md)) | travels |
| source identity (path, format, hash) | working tree | `manifest.json` | travels |
| source skeleton (round-trip template) | `work/cache/extractions/.../skel-*.bin` | `skeletons/<id>` | travels |
| raw source bytes | working tree `src/` | `source/<name>` | opt-in (`--with-source`) |
| secrets (auth tokens, API keys) | OS keychain | — | **never travels** |
| derived stores and caches (`store.db`, `sync-cache.json`, extractions, collections) | `.kapi/work/` | — | regenerated on unpack |
| plugin binaries | user / system install | — | re-resolved via `requires` / registry |
| provenance | — | `history.jsonl` (opt-in) | travels (excluded from `rootHash`) |

**A packaged recipe may only name places inside the project it lands in.** The
format asked the *packer* to strip what travels badly, which a hostile packer
simply will not do, so the sweep (`kpz.SanitizeRecipe`) runs again on **ingest**,
at the single point every `.kpz` this binary opens comes through. It removes
exec-class steps and formats, the per-tool config that would arm them, the
side-effecting `Extras` at every scope they can be registered at, and any
path-valued field that climbs out of the project or starts at the root — the
terms, terms-source, memory-source and state bindings, the redaction rules file,
the brand-voice profile file, and each content entry's `base` and `target`.

The asymmetry is the point. A project's **own** recipe may name an absolute
destination — publishing outside the tree is a thing its owner is entitled to
ask for — but a recipe that arrived in a package is answering for a machine it
has never seen. Nothing is fatal: each removal is reported so the recipient
learns what the author meant, and every one of these fields has a defined
meaning when unset.

`requires:` deliberately survives. It declares which plugins the package's flows
need, and dropping it would remove a statement of fact without removing any
capability — a recipe installs nothing by itself, and an undeclared plugin just
fails later with a worse error. The install it can prompt for resolves through
the signed plugin registry, so a package can ask for a plugin neokapi publishes,
not for code of its own. That is the line [AD-038](038-execution-trust.md)
draws for `--yes`: "install the plugin this recipe asks for" and "run the
commands this recipe names" are different classes of decision.

**Source of truth on round-trip.** When a `.kpz` is unpacked into or sits beside a
`.kapi` project, the on-disk recipe is authoritative and the package is a
snapshot; a standalone `.kpz` (the ad-hoc workspace) is authoritative in itself.
Intent therefore never has two live homes that can drift.

With **no** project in scope, `unpack` reconstitutes one: a directory beside the
snapshot, named from the recipe (a name that is not a single path segment falls
back to the snapshot's own base name), holding the recipe as its `kapi.yaml`.
That write is the only moment unpack turns a package into intent, so it is the
moment it asks — adopting means later `kapi run` here executes flows someone
else wrote. Declining leaves nothing behind and says so, because without a
recipe there is no project for the content to land in. Unpacking the same
snapshot again finds the project it made and refreshes the state without asking.

### 7. Boundaries: workspace vs payload, and the two `.kpz` profiles

Day-to-day work happens in an **ambient `.kapi` project**, discovered by a
git-style upward walk ([AD-008](008-project-model.md)) — never named on a command.
A `.kpz` is a **parcel**: a thing that crosses a boundary, named only at that
boundary. You do not *work inside* a `.kpz`; receiving one and working on it means
opening it into a project (`unpack`, or an in-place open backed by the shadow
cache of §5), then `pack` to ship again. This is git's split between a *working
tree* (ambient) and a *bundle* (named only at create / clone), and it is why the
everyday loop never types a `.kpz` path.

Which parcel crosses which boundary:

| Boundary | Parcel | Fidelity | Verbs |
| --- | --- | --- | --- |
| Time / space, in-ecosystem (backup, transfer, seed a server) | **project `.kpz`** (whole project, §6) | lossless native | `pack` / `unpack` |
| To a translator or reviewer | **bilingual `.kpz`** (one locale pair, below) | lossless native | `extract` / `merge` |
| To a third-party CAT tool | XLIFF 2.x / PO ([AD-017](017-bilingual-format-interop.md)) | interoperable, lossy | `extract` / `merge` |
| To the live server | sync wire protocol (the `.kpz`'s over-the-wire twin) | lossless, streamed | `push` / `pull` |

So one `.kpz` container carries **two profiles**, distinguished by the manifest
`kind`:

- **Project profile** (`kind: kapi-project`) — the whole project: all locales,
  full recipe, content memory, terms, overlays, source identity + skeletons (§6).
  The *snapshot / ecosystem payload*, moved by `pack` / `unpack`.
- **Bilingual profile** (`kind: kapi-interchange`) — a task-scoped slice for one
  source→target pair: the blocks with faithful inline codes, the
  segmentation/alignment overlays, the per-source skeleton for round-trip, and the
  relevant memory-match + term context. It excludes other locales, the full
  recipe, and raw source. This is **neokapi's interchange format** — the parcel
  `extract` sends to a translator or reviewer and `merge` ingests back
  ([AD-017](017-bilingual-format-interop.md)) — lossless where XLIFF is lossy, with
  inline memory/term context and integrity-verified, diffable review. It is *ecosystem*
  interchange (read by a neokapi tool); XLIFF / PO remain the industry-interop
  tier, and turning this profile into a cross-vendor standard is an open-spec
  effort, not a property of the format.

Both profiles are parcels — neither is a workspace.

## Consequences

- A project's full content can be serialized losslessly and rehydrated into
  fresh stores. The guarantee is enforced by a cache-internal round-trip test:
  populate real `memory` / `terms` stores → pack to `.kpz` → unpack into
  fresh stores → re-pack → assert byte-identical.
- TMX/TBX keep their role as industry interchange, and their lossiness is a
  documented, intentional property.
- The Kapi format family stays cohesive: every member is deterministic and hashable, so
  a `.kpz` — and each member — has a stable content hash, and the same Merkle
  diff the sync engine runs over the wire applies to packages at rest.
- KBF itself is unchanged; the family composes around it rather than growing it.
- A `.kpz` can carry in-progress work (overlays), so a project's working state
  can be handed off (`pack`/`unpack`) and a flow resumed against the warm
  block-store cache — with progress derived from content rather than a journal,
  the server-less twin of the platform's stateful project.
- Because a `.kpz` carries the full recipe (§6), it is a **project in a file**: a
  standalone package runs its own flows and `unpack` rebuilds a complete `kapi.yaml`.
  Near-full parity with a `.kapi` project follows, the deliberate gaps being
  secrets (never), caches (regenerated), plugin binaries (re-resolved), and raw
  source (opt-in). Side-effecting recipe travels inert, so receiving a `.kpz`
  cannot trigger a server call, hook, or automation until it is adopted into a
  project.
- The same `.kpz` container serves two profiles (§7): a whole-project snapshot
  (`pack`/`unpack`) and a task-scoped **bilingual interchange** file
  (`extract`/`merge`) — neokapi's lossless interchange format for a translator or
  reviewer. A `.kpz` is always a *parcel*, never a *workspace*: day-to-day work is
  the ambient `.kapi` project ([AD-008](008-project-model.md)), so the everyday
  loop never names a `.kpz`.

## Implementation

- Formats: `memory/kmb`, `terms/ktb`, container `kpz/`.
- Tests: per-format lossless round-trip + determinism + envelope rejection, and
  `kpz` package round-trip + a cache-internal store round-trip.
- The working-state / hand-off capability (§5) is implemented
  ([GitHub issue #787](https://github.com/neokapi/neokapi/issues/787)): the
  `.kpz` carries `overlays.json` (in-progress overlays) + `source/<name>` + a
  manifest `recipe`; the block-store exporter/loader (`core/blockstore/exporter`)
  is the inverse of the importer; `flow.FileRunner` runs against a persistent
  store. The ad-hoc workspace verbs live in `cli/kpzworkspace.go` —
  `extract`/transform-in-place/`merge`, dispatched from the `extract`, `merge`,
  `run`, and tool commands — and `pack` / `unpack` snapshot and rehydrate a whole
  project's state. Progress is derived from the overlays present (no journal); the
  optional advisory `history.jsonl` (hash-chained, opt-in `pack --log`, verified
  on `unpack`) is
  excluded from the content `rootHash`. The package embeds the full project recipe
  (§6, side-effecting `Extras` inert) and retains source as identity + skeleton,
  with raw bytes behind `--with-source` (§7). Covered by unit tests (kpz overlays +
  history-chain round-trip and tamper detection; the exporter store round-trip),
  the `kapi/e2e` suite (pack/unpack round-trip, cached-resume byte-equality,
  pack determinism, provenance log), and the `make kpz-smoke` headless gate.
- Reference: [Kapi format family & the .kpz package](/reference/serialization/project-archive).
