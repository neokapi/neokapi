---
id: 009-sync-protocol
sidebar_position: 9
title: "AD-009: Sync Protocol"
---

# AD-009: Sync Protocol

## Summary

The sync protocol is Bowrain's single transport for everything a project
holds — content blocks, terms, content memory entries, media, and **context**:
the collections a project declares, the coordinates that place them, and the
governance bound to them. It has three layers: a diff layer that negotiates
what must move by comparing hashes, a typed `SyncChunk` content layer that
carries the payload, and a chunked, compressed transport layer that moves bytes
to object storage without passing them through the API process.

Two properties define it. **Push describes a desired state, not a delta** — the
client sends what it holds and the server replies with what must change,
including what it holds that the client no longer does. And **ownership travels
in the payload**: every context entity says whether the recipe or the workspace
owns it, so the question "may this be edited here?" is answered from data rather
than from a rule bolted on at each surface.

## Context

The sync protocol is the data-exchange path between the kapi CLI, Kapi Desktop,
bowrain-server, and any other client of the platform. It carries the full Block
model, not just text, and must scale to 100K+ blocks per project, survive
unreliable networks, detect concurrent writes, and stay extensible as new kinds
of project state arrive.

Several constraints shape the design:

- **API server responsiveness.** The API process is a control plane: it
  authorizes, negotiates, and returns. Heavy database work happens in the
  worker, outside the request lifecycle.
- **Bandwidth.** A 100K-block project cannot transfer its full dataset on every
  sync. Hash comparison identifies the changed subtrees.
- **Concurrent writes.** Several clients may edit one project at once. Silent
  last-write-wins is not acceptable for translations.
- **Async ingestion.** Bulk insert outperforms per-row insert by one to two
  orders of magnitude. The worker controls its own batching and retry cadence.
- **Extensibility.** New kinds of project state must ship without breaking the
  transport.

The last constraint is the one that has been learned the hard way. A kind of
state that has no place in the content model does not simply wait — it arrives
by another route. Terminology grew a REST path of its own, media grew an asset
upload path, and brand voice grew a profile upsert. Each of those sits outside
the push transaction, outside the hash comparison, and outside reconciliation,
so a push carrying them can half-succeed: the content lands and the governance
does not, or the reverse, with nothing recording that the two disagree.

Hence the governing rule of this AD: **a kind of project state is a content
type of this protocol, or it is not synced.** A side call is not a smaller
version of syncing something. It is the absence of syncing it.

## Decision

### Three layers

| Layer | Answers | Mechanism |
| --- | --- | --- |
| Diff | What must move? | Hash comparison at project, item, and content-type level |
| Content | What is it? | Typed `SyncChunk` envelope, one content type per chunk |
| Transport | How does it move? | zstd-compressed chunks, content-addressed, direct to object storage |

The layers are independent. Adding a content type changes the content layer
only: the diff layer gains a hash, and the transport layer is untouched.

### Diff layer: reconciliation, not delta

Push negotiation runs in three round-trips before any payload moves.

1. **Init.** The client sends the state it holds: an item hash per item (the
   hash of that item's block hashes), a project root hash over those, and a
   collection hash per non-block content type — `terms_hash`, `memory_hash`,
   `context_hash`. All hashes use one construction: SHA-256 over the entries
   sorted by key, writing each key followed by its hash.
2. **Reply.** The server compares and answers `unchanged` when the root hash
   matches and no collection hash moved, `full_upload` when it holds nothing
   for this stream, and `diff_required` otherwise. A `diff_required` reply
   names the items that changed, the items the client has that the server does
   not, the items **the server holds that the client did not send**, and which
   collection hashes moved.
3. **Item diff.** For each changed item the client sends its block hashes and
   receives back which blocks are needed, which the server holds that the
   client no longer does, and which have been changed by another client since
   this one last synced.

The third category in each reply — `deleted_items`, and `deleted` within an
item — is what makes this reconciliation rather than a delta. The client does
not send "delete this"; it sends what it has, and absence is the signal.

#### Authoritative hashes are the precondition

A hash map is a statement about a desired state only if it is complete. A
client that hashes just the subset it intends to upload has not described a
state: the server cannot tell "absent because unchanged" from "absent because
removed", and every reported removal is a false positive. Such a client must be
treated as **additive** — the server upserts what arrives and infers no
deletions, and the client ignores the removals reported back to it.
Non-destructiveness is then a property of the client's incompleteness, not a
guarantee the protocol offers.

This is why the two halves of the protocol are calibrated differently, and
deliberately so:

- **Blocks** are the large set. Sending a complete hash map for a 100K-block
  project on every push is affordable at item granularity (one hash per item)
  but the client's own change detection works at block granularity, so a client
  that has scanned only the changed files holds a partial picture. A block push
  that has not established a complete map is additive; establishing one is what
  destructive block sync requires, and is the single precondition for it.
- **Context** is the small set — a project's axes, collections, profile
  bindings, and the voice they bind. Its complete key-to-hash map is a few
  kilobytes at any project size, so the client sends it in full on every push
  regardless of what changed. Context is therefore authoritative from the first
  push: a collection the recipe no longer declares is a removal, not an
  omission.

The manifest carries that complete map (`context_hashes`) alongside the chunks
that carry the changed entities, so the reconciliation input and the payload
commit together.

### Conflict detection

Every block being updated carries `expected_hash` — the hash the client last
saw. If the server's current hash differs, another client changed the block
since, and the server reports a conflict rather than overwriting. The client
resolves the conflicts or re-pushes with `--force`.

Context entities carry the same guard through their per-entity `content_hash`,
with one addition described below: a conflict on a context entity may also be
an **ownership** conflict, where the change is refused not because the content
moved but because it is not this side's to change.

### Content layer: the `SyncChunk` envelope

Each chunk is a typed envelope carrying a batch of one content type. Protobuf
gives forward and backward compatibility: fields are additive and an older peer
ignores what it does not know.

```protobuf
message SyncChunk {
  string content_type = 1;  // "blocks", "terms", "memory", "media", "context"
  int32 record_count = 2;

  // Exactly one of these is populated (determined by content_type).
  repeated SyncBlock blocks = 10;
  repeated SyncTerm terms = 11;
  repeated SyncMemoryEntry memory_entries = 12;
  repeated SyncMedia media = 13;
  // 14, 15 reserved: qa_results, activities.
  repeated SyncContextEntry context = 16;
}
```

Adding a content type means adding a field here, a collection hash in the init
request, a symmetric field on the pull response, and a handler in the worker.
The diff and transport layers do not change.

`SyncBlock` carries the full Block model across the boundary — structured
source and target segments, properties, annotations, skeleton, display hints,
connector data, run-anchored overlays, source locale, and referent flag — so a
round-trip through Bowrain loses nothing. Its run and segment content embeds
the canonical content-model schema rather than redefining it; see
[AD-framework-034: Content-Model Wire Schema](https://neokapi.github.io/contribute/architecture/034-content-model-wire-schema).

### Context is a content type

A project declares where its content sits and what governs it. Under
`coordinates:` a recipe names its own axes — product, channel, market, tenant,
whatever the taxonomy is. Each named content collection names the point its
content sits at, and `profiles:` binds governance to a region of that space,
the most specific match winning. This is the framework's model; see
[the kapi project file](https://neokapi.github.io/contribute/implementation/kapi-project-file).

All of it is project state, so all of it travels the protocol as the `context`
content type:

- **Axes** — the declared dimensions and the values each may take.
- **Collections** — each with its name, its item patterns, and its coordinates.
- **Profile bindings** — a coordinate match plus the voice, the terms, or both
  that govern content matching it.
- **Voice profiles** — the profile content itself, so the governance a binding
  names is present rather than dangling.

One envelope carries all four, which is what lets a single collection hash
cover the whole of a project's context and a single reconciliation pass settle
it:

```protobuf
message SyncContextEntry {
  // Stable identity within the project's context. Collections and axes are
  // keyed by declared name, profile bindings by their rendered coordinate
  // match, voice profiles by profile name.
  string key = 1;
  string kind = 2;   // "axis", "collection", "profile", "voice"
  Owner owner = 3;
  string content_hash = 4;

  // Exactly one of these is populated (determined by kind).
  SyncAxis axis = 10;
  SyncCollection collection = 11;
  SyncProfileBinding profile = 12;
  SyncVoiceProfile voice = 13;
}
```

The consequences follow from being a content type rather than being special:

- **One hash.** `context_hash` sits beside `terms_hash` and `memory_hash` in
  the init request, so an unchanged context costs one comparison.
- **One transaction.** Context entities ride the same `upload_id`, the same
  commit manifest, and the same worker job as the blocks pushed with them. One
  push is one consistent state, or it is a failed push.
- **Removal is an outcome, not a case.** A collection the recipe renamed
  appears as one key absent and another present. The server reports the absent
  one exactly the way it reports `deleted_items` for content, and applies it
  under the ownership rules below. Nothing in the client or the server needs to
  recognize a rename as a rename.
- **Pull is symmetric.** `SyncPullResponse` carries context in a typed field
  beside blocks, terms, memory entries, and media, filtered by content type and
  advanced by the same cursor.

Coordinate values are slugs, never concept references. A concept is designed to
be renamed and deprecated as vocabulary is revised, and governance that moved
when someone edited a term would be governance nobody could rely on. A value
may carry a concept for display; matching never reads it. That property has to
survive the wire, which is why `SyncAxis` carries the slug and the concept
reference as separate fields rather than resolving one into the other.

### Ownership travels in the payload

Every context entity carries its owner:

```protobuf
enum Owner {
  OWNER_UNSPECIFIED = 0;
  OWNER_RECIPE = 1;     // declared in kapi.yaml; git is the source
  OWNER_WORKSPACE = 2;  // created in the workspace; the server is the source
}
```

`kapi.yaml` is a file in a repository. What it declares is authored, reviewed,
and versioned there, and a workspace that edits it is editing a copy that the
next push overwrites. What the workspace itself creates has no counterpart in
any repository, and a push that deleted it because no recipe mentions it would
be destroying state its owner never handed over.

Ownership is a fact about an entity, so it lives on the entity. One field
settles three questions that would otherwise each grow their own rule:

1. **Pull does not overwrite what the recipe owns.** A recipe-owned entity
   arriving on a pull updates the local view of what the server believes;
   it never rewrites the recipe. The file on disk stays the source.
2. **The workspace API refuses to mutate what the recipe owns.** The refusal is
   a property of the row, not a check written into each handler, so a route
   added later inherits it.
3. **The web surfaces render read-only from data.** A recipe-owned collection
   shows its provenance and its edit affordances are absent because the entity
   says so — not because a component hardcoded a list of what is editable.

Provenance also has a default. An entity that arrives through a push and does
not name an owner is recipe-owned; one created through the workspace API is
workspace-owned. `OWNER_UNSPECIFIED` therefore resolves by the channel the
entity arrived on, and an older client that omits the field still produces
correct ownership rather than a workspace-editable copy of a governed entity.

### Brand voice is context, not a side call

A brand voice profile bound by a recipe is governance attached to a region of
the project's context space. It travels as a `voice` entry in the `context`
content type, in the same push as the collections and profile bindings that
name it, under the same hash and the same ownership rule.

That places it under this protocol's guarantees rather than beside them. The
profile cannot land while the content that it governs fails, or the reverse.
The workspace cannot silently edit a profile the recipe owns, and the version
history the brand hub keeps records which side authored each version instead of
inferring it. Rules the server promoted from corrections belong to the
workspace and are preserved across a push for exactly the reason the ownership
field exists: they were never the recipe's.

See [AD-framework-022: Brand Voice](https://neokapi.github.io/contribute/architecture/022-brand-voice)
for the profile model, and [AD-021: The context graph](021-brand-knowledge-graph.md)
for the graph the workspace's own context lives in.

### Streams are one concept

A git branch, a release, and a governance experiment are all **streams**. They
differ in intent and metadata, never in kind: each is a named branch of content
with its own change log, branched copy-on-write from a parent at a cursor, as
described in [AD-005: Streams](005-streams.md). There is no separate experiment
object, no separate release object, and no third thing a client has to learn to
address.

Two things follow.

**Change-sets are proposals applied to a stream.** A change-set is an ordered
set of operations with a review status — the reviewed path that governed edits
travel instead of applying directly. Applying one is applying its operations to
a stream. That makes "propose a governance change", "propose a terminology
change", and "propose a release" the same shape of object at the same target,
and it means the review, audit, and rollback machinery in
[AD-020](020-governance-audit-rollback.md) covers all of them.

**Context may differ per stream.** An experiment that tries a different voice
for one market is a stream whose context entries differ from its parent's, and
nothing else about it is unusual. This needs no new resolution machinery: the
framework's profile resolution chain already ranks an explicit selection over a
collection binding, over a **stream** binding, over a project binding, over a
workspace default. The stream tier sits between collection and project and has
from the start. A stream-scoped context entry populates that tier; comparing
two streams' governance is comparing two sets of context entries.

Every sync operation is already stream-addressed — `SyncPushInit.stream`,
`SyncPullRequest.stream`, `SyncManifest.stream`, and the stream segment of each
route — so carrying context per stream adds a scope to the entities, not a
dimension to the protocol.

### Transport layer

The API server never handles content bytes. Chunks move between the client and
object storage, and the API process authorizes, negotiates, and records.

A push is: init, then a block diff per changed item, then chunk uploads, then a
commit that returns `202 Accepted` with a push identifier. A pull is a single
cursor-paginated request returning changed records since the cursor. The client
polls the push status endpoint for ingestion progress.

The init exchange settles a transport mode. In `direct` mode the server issues
pre-signed URLs and the client uploads straight to object storage. In `proxy`
mode — local development, or a self-hosted deployment without pre-signed URL
support — the client uploads through the API, which stores each chunk
content-addressed so the worker resolves it identically either way. The client
library handles both behind one call.

Chunks are content-addressed by the SHA-256 of the exact bytes uploaded, after
compression. The commit manifest names those hashes, and the server verifies
each named chunk exists before enqueuing the job. The worker re-derives the
hash of what it downloaded and refuses bytes that do not match: the manifest's
hash is a promise about content, not merely the name of a place to find it.

### Compression and chunking

zstd, applied per chunk, from a pooled encoder and decoder
(`core/storage/compression`) so no allocation is paid per request. Repetitive
translation payloads compress well. Dictionary support is an optional hook: the
pool accepts a trained dictionary, but none is shipped, so the default path is
standard zstd. A dictionary would most help small payloads, which lack the
context to build shared state on the fly; training and embedding one is future
work.

Chunks are sealed by serialized byte size rather than record count, sized from
each record's full marshaled size so that records with rich annotations cannot
produce an oversized chunk. A record larger than the threshold rides in a chunk
of its own. A secondary record-count cap keeps chunks bounded when records are
tiny. Each chunk carries one content type, which keeps worker routing a switch,
and each is independently retryable.

### Budget and rate limits

The commit manifest declares each chunk's byte size, and the server rejects a
manifest whose total exceeds the project's upload budget before it pays for a
storage probe per chunk. Per-project rate limits bound push frequency and block
count. Pull is cursor-paginated with a default and a maximum page size.

### Hash cache

Item and block hashes are cached so diff negotiation does not read the full
hash set from the database on every push init: block hashes keyed by project
and item, item hashes keyed by project, invalidated on write and falling back
to a database query on a miss. Without it the server would load 100K hashes per
init on a large project.

### Async ingestion

The API process validates, negotiates, receives the manifest, enqueues a job,
and returns. The worker reads chunks from storage, verifies each against its
declared hash, decompresses, routes by content type, and stores — grouping
records by item and writing in batches under one transaction per item, with
multi-row upsert rather than per-row insert. It publishes a push-completed
event, invalidates the hash cache, and deletes the manifest blob.

A content type the worker does not implement fails the job explicitly rather
than being dropped: a push that reports success while discarding a payload is
worse than one that fails, because only the second is recoverable.

### Endpoints

```
POST /api/v1/workspaces/:ws/projects/:id/sync/push/init
POST /api/v1/workspaces/:ws/projects/:id/sync/push/diff
PUT  /api/v1/workspaces/:ws/projects/:id/sync/push/chunks/:uploadId/:index
POST /api/v1/workspaces/:ws/projects/:id/sync/push/commit
GET  /api/v1/workspaces/:ws/projects/:id/sync/pull
GET  /api/v1/workspaces/:ws/projects/:id/sync/status?push_id=X
```

Stream-scoped variants carry the stream in the path. Workspace-scoped routes
enforce tenancy at the transport layer; the server authorizes against the
path-scoped project and ignores any project, actor, or workspace the client
supplies in a body, so a request cannot address a tenant it was not routed to.

### Operational alignment

The API process and the worker must use the same blob store. Misaligned stores
produce push jobs that fail with a chunk download error and no other symptom,
because every earlier step succeeds. Local development pins both to the same
directory; deployed environments pass both the same bucket configuration.

## Consequences

- One transport carries every kind of project state. A new kind is a content
  type, and a content type is inside the transaction, the hash comparison, and
  the reconciliation by construction.
- Push describes a desired state. Removals are reported by the side that
  notices them, and a rename needs no special handling on either side.
- The completeness of a client's hash map determines whether its push can be
  destructive. Context is complete on every push and reconciles from the first
  one; blocks are additive until a client establishes a complete map.
- Ownership is data. Whether an entity may be edited is answered the same way
  in the pull path, the API, and the UI, and a new surface inherits the answer
  instead of reimplementing it.
- Brand voice, terminology, and content memory land or fail with the content
  they govern. A push cannot half-succeed across the boundary between content
  and the governance over it.
- A stream is the only branching concept. Experiments, releases, and branches
  reuse the machinery streams already have, and per-stream governance falls out
  of a resolution tier that exists.
- The init request stays small at any project size: item-level hashes, not
  block-level. Block comparison runs only for items that actually changed.
- The API process never touches content bytes, so push load is bounded by
  negotiation cost rather than by payload size.
- Conflict detection prevents silent overwrites for content and for governance,
  and distinguishes "changed underneath you" from "not yours to change".

## Related

- [AD-004: Content Store and Versioning](004-content-store.md)
- [AD-005: Streams](005-streams.md)
- [AD-007: Media and Blob Storage](007-media-and-blob-storage.md)
- [AD-008: Connector System](008-connector-system.md)
- [AD-020: Governance, audit, and rollback](020-governance-audit-rollback.md)
- [AD-021: The context graph](021-brand-knowledge-graph.md)
- [AD-022: Convergence as a service](022-convergence-as-a-service.md)
- [Note: Bowrain Sync Protocol](/notes/sync-protocol) — message shapes, hash
  construction, and the reconciliation table
- [AD-framework-002: Content Model](https://neokapi.github.io/contribute/architecture/002-content-model)
- [AD-framework-022: Brand Voice](https://neokapi.github.io/contribute/architecture/022-brand-voice)
- [AD-framework-034: Content-Model Wire Schema](https://neokapi.github.io/contribute/architecture/034-content-model-wire-schema)
