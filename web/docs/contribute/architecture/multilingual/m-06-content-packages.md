---
id: m-06-content-packages
sidebar_position: 6
title: "M-06: Content packages"
description: "A family of deterministic, lossless JSON formats, one per content atom, and a .kpz container that bundles them with the project recipe into a portable parcel, in two profiles: a whole-project snapshot and a task-scoped bilingual interchange file."
keywords: [neokapi, architecture decision, kpz, package, content bundle, memory bundle, terms bundle, pack, unpack, interchange, determinism]
---

import { LanesDiagram } from "@neokapi/docs-shared";

# M-06: Content packages

## Summary

neokapi defines a family of native, deterministic, lossless serialization
formats, one per content atom, and a **`.kpz` package container** that bundles
them into a portable snapshot of a project's authoritative content:

| Atom | Native format (lossless) | Package member | Interchange format (lossy) |
| --- | --- | --- | --- |
| blocks and targets | content bundle (`.kbf.json`) | `blocks/*.kbf.json` | XLIFF / PO |
| stand-off overlays | overlay records (`.overlays.jsonl`) | `annotations/*.overlays.jsonl` | none |
| content memory | memory bundle (`.memory.json`) | `memory.json` | TMX |
| terms | terms bundle (`.terms.json`) | `terms.json` | TBX |
| media | opaque blobs | `media/*` | none |

Every member of the family is JSON, and its suffix says so. A marker segment
ahead of `.json` keeps the file self-describing while `jq`, a code host, and
editor syntax highlighting still see what they are looking at; a bare extension
would buy the marker at the cost of every tool that reads JSON by extension.
Only `.kpz` keeps a dedicated extension, because it is a binary zip nobody
hand-edits.

The memory and terms members carry the conventional bare names inside a package,
so unzipping one by hand yields the spelling the rest of the tooling teaches.
The names are **presentation only**: unmarshalling routes every member by its
manifest content type, never by filename, so a member's path can change without
changing how a package is read.

The package itself is a deterministic zip with a manifest carrying a per-member
SHA-256 and a Merkle root hash. It is the **at-rest twin** of the sync wire's
chunk set, over the same content types.

## Why native formats at all

The content bundle already gives blocks a deterministic, hashable, lossless
on-disk form. The other content a project owns had no equivalent, and the
interchange formats cannot supply one:

- **TMX** preserves the multilingual variants a translation tool understands but
  silently drops the enrichments a memory entry carries: entity mappings
  including the cross-link to a term concept, provenance origins and import
  sessions, per-entry properties, and notes.
- **TBX** maps the standard terminology fields but drops a concept's native
  ones: whether a term came from terminology or from a voice vocabulary, the
  competitor flag, and the extensible property map.

Without a lossless serialization there is no way to back a whole project up,
move it between machines, seed a fresh working copy, or build a deterministic
test fixture, and a lossy serialization cannot faithfully **regenerate the
derived stores** a runtime builds from this content.

A second observation shaped the design. The project layout already separates
**authoritative content** (the committed sources and the unit-state record)
from everything a run derives from them: the project's one database and the
free-to-delete cache ([C-03](../context/c-03-context-store-and-graph.md)). The
thing worth packaging is the authoritative content, never the derived stores or
the secrets.

## One tier per store

Each atom's native format round-trips its full model and shares one discipline:
a `kind` magic string, a `MAJOR.MINOR` schema version with the reject-unknown-major
and accept-unknown-minor contract, and a **deterministic** encoder (sorted keys
and records, no HTML escaping, a trailing newline), so output is stable for
content hashing and for a `git diff`.

- The **content-memory bundle** maps wire records to and from memory entries;
  variant content reuses the canonical run serialization, so inline codes,
  placeholders, and plural and select constructs survive identically to the
  blocks in a content bundle.
- The **terms bundle** reuses the already-JSON-tagged concept type directly,
  one source of truth with no parallel wire type to drift, and so preserves
  every native field by construction.

There is exactly **one tier per store**: the bundle. A project commits it,
because reviewed memory and terms have to diff line by line, and the compound
`.json` suffix is what makes that diff readable in a browser and greppable with
`jq`. A second, compressed spelling per store would buy nothing that `zip` and
`gzip` do not already give.

The **suffix**, not the location, identifies a bundle, because a project is not
limited to one per store. Terms is the exception: a project has exactly one set
of terms, so it gets a conventional location ([C-08](../context/c-08-terms.md)).
Content memory gets none, because a project accumulates a bundle per content
surface and a fallback would have nothing single to name.

## The container

The `.kpz` format is a deterministic zip (stored entries, fixed timestamps,
sorted order) containing a manifest plus one member per content type. The
manifest lists each member with its content type and SHA-256, and a Merkle root
hash over the sorted member hashes gives the package a stable content identity
independent of zip framing. Unmarshalling validates the envelope, every member
checksum, and the root hash.

Membership is decided **per table, not per file**. Because every subsystem
shares one database, "does this project have terms?" is a question about rows,
so a pack carries only the parts that hold something: an empty subsystem
contributes no member, exactly as an absent one would.

## Two tiers: native and interchange

- **Native (lossless)**: the format family and `.kpz`. Used for packing,
  caching, hashing, and any flow that must reconstruct project state exactly.
- **Interchange (lossy)**: XLIFF and PO for blocks, TMX for content memory, TBX
  for terms. Used to hand content across an organizational boundary into the
  wider translation industry. These stay the export path and are **never**
  package members, because they cannot represent the native fields
  ([M-01](m-01-bilingual-interop.md)).

## A parcel, and the workspace it opens into

Day-to-day work happens in an **ambient project**, discovered by a git-style
upward walk ([C-01](../context/c-01-project-model.md)) and never named on a
command. A `.kpz` is a **parcel**: a thing that crosses a boundary, named only at
that boundary. You do not work *inside* one; receiving one and working on it
means opening it into a project and packing it again to ship. That is the split
between a working tree and a bundle, and it is why the everyday loop never types
a `.kpz` path.

| Boundary | Parcel | Fidelity | Verbs |
| --- | --- | --- | --- |
| Time or space, in-ecosystem (backup, transfer, seeding) | **project `.kpz`** | lossless native | `pack` / `unpack` |
| To a translator or reviewer | **bilingual `.kpz`** | lossless native | `extract` / `merge` |
| To a third-party translation tool | XLIFF 2.x / PO | interoperable, lossy | `extract` / `merge` |
| To a hosted platform layer | the sync wire, the package's over-the-wire twin | lossless, streamed | `push` / `pull` |

One container therefore carries **two profiles**, distinguished by the manifest
kind:

- **Project profile** (`kapi-project`): the whole project, every locale, the
  full recipe, content memory, terms, overlays, source identity and skeletons.
  The snapshot, moved by `pack` and `unpack`.
- **Bilingual profile** (`kapi-interchange`): a task-scoped slice for one
  source→target pair, the blocks with faithful inline codes, the segmentation
  and alignment overlays, the per-source skeleton, and the relevant memory-match
  and term context. It excludes other locales, the full recipe, and raw source.
  This is neokapi's native interchange carrier, the parcel `extract` sends and
  `merge` ingests.

Both profiles are parcels rather than workspaces.

## Working state, hand-off, and resume

A `.kpz` is both an at-rest snapshot of finished content **and** a carrier of
in-progress working state, so work can stop, move between machines, and resume
where it left off. That is delivered through mechanisms already in the grain
rather than a step-by-step verb family.

<LanesDiagram
  lanes={[
    {
      title: "ad-hoc workspace",
      sub: "no project: the parcel is the unit",
      steps: [
        "extract <sources> -o work.kpz",
        "run / exec transforms the cache in place",
        "merge work.kpz emits the documents",
        "pack work.kpz ejects to the file",
      ],
      role: "io",
    },
    {
      title: "cached resume",
      sub: "inside a project",
      steps: [
        "a run executes against the persistent block store",
        "a tool appends an overlay keyed by (kind, blockHash)",
        "re-running skips work whose overlay is present",
      ],
      role: "tool",
    },
    {
      title: "project snapshot",
      sub: "the whole project as a parcel",
      steps: [
        "pack exports overlays, content, skeletons, recipe",
        "unpack rehydrates into another machine's .kapi/",
        "the recipe comes back as a runnable kapi.yaml",
      ],
      role: "annotate",
    },
  ]}
  handoff="same store, same overlays"
  caption="Three ways to carry work, all reading and writing the same content-addressed overlays."
/>

**The ad-hoc workspace.** A `.kpz` is the portable bundle; the runtime is a
persistent **shadow cache** keyed by the file's absolute path, so the working
directory stays a single file. Three pipeline-stage verbs work with no project:
`extract <sources> -o work.kpz` ingests the sources and records a recipe;
running any tool or flow *on* the `.kpz` transforms it in place against the
cache's per-source block stores, incrementally, without rewriting the file; and
`merge work.kpz` emits the finished documents from the cache, one file per
source and locale. The `.kpz` itself is rewritten only by an explicit eject,
`kapi pack work.kpz`, and `kapi info work.kpz` reports whether the cache is
**dirty**, its content root hash having diverged from the packed file. Block ids
are unique only within one document, so each source has its own store and each
overlay is tagged with its source. Transforming reuses overlays already present
rather than recomputing, so the output equals a one-shot run.

**Cached resume in a project.** A project run executes against the project's
persistent block store. Because that store is append-only and content-addressed
(a tool appends an *overlay* keyed by kind and block hash rather than rewriting
a block), a session tool caches its per-block result and hydrates from it on a
later run. Re-running a flow skips work already done; the store *is* the
workspace, and resume is just running again.

**The project snapshot.** `pack` exports the block-store overlays, the content
memory and terms content, the source identity and skeletons, and the **full
project recipe** into a portable `.kpz`; `unpack` rehydrates it into another
machine's state directory, reconstituting a complete recipe. A `.kpz` is to a
project directory what a bundle is to a repository, and, because it carries the
recipe, a *runnable* one.

### Progress is derived from content

Because the store is content-addressed, "has step X run?" is a pure function of
the content: *does X's overlay exist, anchored to the current block hashes?*
That is what makes cached resume correct and idempotent. Re-running is a no-op
where the overlay is present; a changed source re-hashes its block so only the
affected work recomputes; and crash safety is automatic, since a crash that did
not commit an overlay leaves it absent and the next run redoes it, with
nothing to reconcile.

An authoritative progress journal is deliberately **avoided**. It would be a
second source of truth that can drift from the content, and it cannot survive
the content changing underneath it (a re-hashed block silently invalidates a
"done" claim), so making it correct means re-deriving the content-addressing the
store already provides.

The one durable record beyond content is **advisory provenance**: `pack --log`
appends a hash-chained line to a history member, giving a hand-off a
tamper-evident custody trail that `unpack` verifies and warns about on a break.
It is strictly subordinate to content (excluded from the package root hash,
never read to decide anything, safe to delete with no loss of work), and a
default pack is byte-deterministic. A journal *is* the right tool where an action
has effects outside the content (sent mail, a charged card, a paid API call);
those belong to the audit subsystem, not to progress tracking, whose state is
wholly in the overlays.

## What a package carries

A `.kpz` is the portable twin of a project, so it carries the project's
**portable authoritative state**, both its content and its committed intent,
and nothing environment-specific. One principle decides membership:

> Pack authoritative state, not caches or secrets. **Content** defines the
> package identity (the Merkle root hash); the **recipe** is metadata, excluded
> from it. Secrets never travel; they live in the OS keychain, never in a
> recipe. Caches are regenerated on unpack. A `.kpz` has no runtime, so any
> side-effecting recipe travels **inert** and re-activates only when unpacked
> into a project, with explicit re-authorization and opt-in re-arming.

**Intent travels as the whole recipe.** A package embeds the project recipe
verbatim (flows, plugin declarations and requirements, defaults, content,
preset, and any platform extension blocks) in the same schema a `kapi.yaml`
uses, so there is one source of truth and no parallel intent model. Flows are
ordinary framework intent, so they travel like any other recipe field: a
standalone `.kpz` is runnable with its own named flows, and `unpack`
reconstitutes a complete recipe. This is what makes a package a **project in a
file** ([E-04](../engine/e-04-flows-and-io-binding.md): a flow is portable
composition, carrying no I/O of its own).

**Source travels as identity plus skeleton; raw bytes are opt-in.** A package
always records each source's identity (logical path, format, content hash) and
the per-source skeleton, the round-trip template `merge` reuses. That is enough
for the core loop: an in-place transform reads only blocks and overlays, `merge`
rebuilds the per-locale files from the skeleton, and `info` detects drift from
the source hash. The raw bytes are needed only to *re-extract* under different
settings, so they are embedded on request (`--with-source`), keeping a default
package from duplicating source that is already under version control. The
skeleton is the derived extract, not the original document.

`unpack` writes those raw bytes back into the working tree under their logical
paths, because being able to re-extract on the far side is the whole reason to
carry them. A file already present is left alone (the working tree is
authoritative on its own sources, the same rule the recipe follows), and a member
whose logical path would escape the project root is refused rather than written.

| Concern | In a project | In a `.kpz` | Disposition |
| --- | --- | --- | --- |
| flows | recipe + flow files | recipe `flows` | **travels** |
| plugin declarations and requirements | recipe | recipe | travels (binaries re-resolved via the registry) |
| defaults, content, preset | recipe | recipe | travels |
| side-effecting extension blocks at any scope | recipe extras | recipe extras | travels **inert** |
| path-valued recipe fields | recipe | recipe | travels **contained** |
| content memory / terms | committed bundle sources | `memory.json` / `terms.json` | travels (lossless) |
| blocks, targets, overlays | block tables (derived) | bundle + overlay members (authoritative) | travels |
| unit state | committed state records | the bilingual profile ([M-01](m-01-bilingual-interop.md)) | travels |
| source identity | working tree | the manifest | travels |
| source skeleton | extraction cache | `skeletons/<id>` | travels |
| raw source bytes | working tree | `source/<name>` | opt-in |
| secrets | OS keychain | none | **never travels** |
| derived stores and caches | the work directory | none | regenerated on unpack |
| plugin binaries | user or system install | none | re-resolved via the registry |
| provenance | none | the history member | opt-in, excluded from the root hash |

## A packaged recipe may only name places inside the project it lands in

The format asked the *packer* to strip what travels badly, which a hostile
packer will not do. So the sweep runs again on **ingest**, at the single
point every package this binary opens comes through. It removes exec-class steps
and formats, the per-tool config that would arm them, side-effecting extension
blocks at every scope they can be registered at, and any path-valued field that
climbs out of the project or starts at the root: the terms and memory source
bindings, the redaction rules file, the voice profile file, and each content
entry's base and target templates.

The asymmetry is deliberate. A project's **own** recipe may name an absolute
destination: publishing outside the tree is something its owner is entitled to
ask for. A recipe that arrived in a package is answering for a machine it has
never seen. Nothing is fatal; each removal is reported so the recipient learns
what the author meant, and every one of these fields has a defined meaning when
unset.

Plugin **requirements** deliberately survive. They declare which plugins the
package's flows need, and dropping them would remove a statement of fact without
removing any capability: a recipe installs nothing by itself, and an undeclared
plugin just fails later with a worse error. The install it can prompt for
resolves through the signed plugin registry, so a package can ask for a plugin
neokapi publishes, not for code of its own. That is the line
[E-06](../engine/e-06-execution-trust.md) draws: "install the plugin this recipe
asks for" and "run the commands this recipe names" are different classes of
decision, and only the second is withheld.

**Source of truth on round trip.** When a package is unpacked into, or sits
beside, a project, the on-disk recipe is authoritative and the package is a
snapshot; a standalone package is authoritative in itself. Intent never has two
live homes that can drift.

With **no** project in scope, `unpack` reconstitutes one: a directory beside the
snapshot, named from the recipe, holding that recipe as its `kapi.yaml`. That
write is the only moment unpack turns a package into intent, so it is the moment
it asks; adopting means a later run here executes flows someone else wrote.
Declining leaves nothing behind and says so, because without a recipe there is no
project for the content to land in. Unpacking the same snapshot again finds the
project it made and refreshes the state without asking.

## Consequences

- A project's full content serializes losslessly and rehydrates into fresh
  stores. The guarantee is enforced by a round-trip test: populate real stores,
  pack, unpack into fresh stores, re-pack, assert byte-identical.
- TMX and TBX keep their role as industry interchange, and their lossiness is a
  documented, intentional property.
- Every member is deterministic and hashable, so a package, and each member,
  has a stable content hash, and the same Merkle diff the sync engine runs over
  the wire applies to packages at rest.
- The content bundle format is unchanged: the family composes around it rather
  than growing it.
- Working state can be handed off and a flow resumed against a warm store, with
  progress derived from content rather than a journal. Side-effecting recipe
  travels inert, so receiving a package cannot trigger a network call, a hook, or
  an automation until it is adopted into a project.

## Related

- [F-02: The content model](../foundations/f-02-content-model.md): the runs and overlays every bundle serializes
- [F-03: Identity](../foundations/f-03-identity.md): content hashing and the Merkle root
- [F-04: The content-model wire schema](../foundations/f-04-wire-schema.md): the schema-version contract the family shares
- [E-04: Flows and I/O binding](../engine/e-04-flows-and-io-binding.md): why a flow is portable and can travel in a recipe
- [E-06: Execution trust](../engine/e-06-execution-trust.md): the line the ingest sweep enforces
- [M-01: Bilingual format interop](m-01-bilingual-interop.md): the bilingual profile in use
- [C-01: The project model](../context/c-01-project-model.md): the ambient project a parcel is opened into
- [C-03: The context store and graph](../context/c-03-context-store-and-graph.md): the derived store a package deliberately excludes
- [C-08: Terms](../context/c-08-terms.md) · [C-09: Content memory](../context/c-09-content-memory.md): the two bundles' models
- [Kapi format family and the .kpz package](/reference/serialization/project-archive): the member-by-member reference
