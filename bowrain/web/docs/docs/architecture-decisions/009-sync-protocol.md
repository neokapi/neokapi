---
id: 009-sync-protocol
sidebar_position: 9
title: "AD-009: Sync Protocol"
---

# AD-009: Sync Protocol

## Summary

The sync protocol is Bowrain's transport between a kapi project and the
platform. It has three layers: a diff layer that negotiates what must move by
comparing hashes, a content layer that carries the payload, and a chunked,
compressed transport layer that moves bytes to object storage without passing
them through the API process.

Two content types travel it: **blocks**, the content model itself, and
**context** — the collections a project declares, the point each occupies in the
project's context space, and the governance resolved for that point. Blocks
ride in chunks; context rides in the commit manifest, so the structure the
items are stored into lands in the same transaction as the items.

Two properties define it. **The client sends what it holds and the server
answers what differs**, including what it holds that the client did not send —
and a difference that amounts to a removal is reported by the side that notices
it and acted on by neither. And **ownership travels in the payload**: every
context entry states whether the recipe or the workspace is authoritative for
it, so "may this be changed here?" is a lookup on a stored field rather than a
rule reconstructed at each surface.

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

The rule is met for governance and not for the rest. Brand voice is a field on
a context entry inside the push transaction; terminology and media are side
calls. Which brings the rule's corollary: **a content type is not
first-class until it travels the protocol.** Declaring a message in
`sync.proto` is not travelling it — see [What travels
today](#what-travels-today), which says for each declared type whether anything
actually sends or reads it.

## Decision

### Three layers

| Layer | Answers | Mechanism |
| --- | --- | --- |
| Diff | What must move? | Hash comparison at project, item, and context level |
| Content | What is it? | Typed `SyncChunk` envelope for blocks; typed entries on the commit manifest for context |
| Transport | How does it move? | zstd-compressed chunks, content-addressed, direct to object storage |

The layers are independent. Adding a content type changes the content layer
only: the diff layer gains a hash, and the transport layer is untouched.

### What travels today

`sync.proto` declares more than the protocol carries. The distinction matters,
because a message with no producer offers none of the guarantees the rule above
is about — no hash comparison, no place in the transaction, no reconciliation.

| Declared | Carried by | Status |
| --- | --- | --- |
| `SyncBlock` | `SyncChunk.blocks`, chunk bodies | Live in both directions |
| `SyncContextEntry` | `SyncManifest.contexts`, `SyncPullResponse.contexts` | Live in both directions |
| `SyncMedia` | `SyncPullResponse.media` | Pull only, rendered from the asset store |
| `SyncTerm` | — | No producer. Terminology travels the workspace knowledge REST surface |
| `SyncMemoryEntry` | — | No producer |

A chunk whose content type is `terms`, `memory` or `media` fails the worker job
explicitly rather than being dropped, so a client that starts emitting one gets
an error instead of a silent discard.

Four negotiation fields are declared and inert: `SyncPushInit.content_types`,
`terms_hash` and `memory_hash`, and the `terms_changed` / `memory_changed`
fields of the reply. No client sets them and no server reads them. They are the
shape a terms or content-memory content type would take, not a path anything
travels.

Terminology's real path is the workspace knowledge graph over REST: `kapi push`
reconciles local terms edits against the baseline the last pull recorded,
sending ordinary edits directly to the concept endpoints and bundling governed
edits — a term banned or promoted, a `REPLACED_BY` relation, a concept deleted —
into one submitted change-set. Media's real path is the project asset
endpoints; a pull renders the resulting assets as `SyncMedia` on the way back
down. Both are outside the push transaction, which is exactly what the rule
above says is the cost.

### Diff layer: negotiation before payload

Push negotiation runs in up to three round-trips before any payload moves.

1. **Init.** The client sends the state it holds: an item hash per item (the
   hash of that item's block hashes), a project root hash over those, a context
   hash over the context entries it declares, and the declared collection names.
   All hashes use one construction: SHA-256 over the entries sorted by key,
   writing each key followed by its hash.
2. **Reply.** The context comparison runs first, because its answer qualifies
   the content fast path: a push whose blocks are all unchanged but whose recipe
   declares a new collection still has work to do. The server answers
   `unchanged` only when the root hash matches *and* the context is unchanged;
   otherwise it names the items that changed, the items the client has that the
   server does not, the items the server holds that the client did not send,
   whether the context changed, and the collections it holds under recipe
   ownership that this push no longer declares.
3. **Item diff.** For each changed or new item the client sends its block hashes
   and receives back which blocks are needed and which the server holds that the
   client did not send.

An empty context hash means the push makes no claim about the declared context —
a content-only caller with no recipe to read. That is reported as unchanged with
nothing undeclared, so such a push can never look like a recipe that just
dropped every collection.

The context fields on the init exchange:

```protobuf
  string context_hash = 8;
  repeated string collections = 9;
```

```protobuf
  bool context_changed = 9;
  repeated string undeclared_collections = 10;
```

When `context_changed` is false the client drops the entries from the manifest
rather than sending them to be ignored, so an unedited recipe costs one hash
comparison and nothing else.

#### Removal is reported, never acted on

Both halves of the protocol report removals, and neither acts on one. The
reasons differ, and keeping them distinct is what makes the block half's
restriction removable and the context half's deliberate.

- **Blocks are additive because the client's picture is partial.** A hash map is
  a statement about a desired state only if it is complete, and the client
  diffs against its own content-hash cache before calling push: an unchanged
  item is absent from the request entirely, and a changed item carries only its
  changed blocks. The item hashes and root hash are therefore change indicators,
  not authoritative Merkle roots. The server treats such a push as additive, and
  the client ignores every removal reported back to it — `deleted_items` from
  the init reply and `deleted` from each item diff. Non-destructiveness here is
  a property of the client's incompleteness rather than a guarantee the
  protocol offers.

  **Authoritative hashes are the precondition for changing that.** If
  server-side deletion of blocks or items is ever wanted, the caller must pass
  the full block set so the hashes become authoritative Merkle roots, and the
  sites that ignore reported removals must be revisited in the same change. Not
  one without the other: a client that acts on removals while hashing a subset
  deletes content the user still has.

- **Context is complete, and non-destructive anyway.** A project's declared
  collections are a few kilobytes at any project size, so the client sends every
  one of them on every push along with a hash over all of them. The map *is*
  authoritative — the server can tell "absent because unchanged" from "absent
  because no longer declared", and it says which collections fall in the second
  category. It still does not remove them, for a reason that is about content
  rather than about hashes: a collection is where content lives, and editing a
  recipe is not consent to drop the server-side content grouped under the
  collection the edit dropped. The report reaches the client on the init reply,
  the worker's log, and the push-completed event; nothing downstream of those
  deletes anything.

### Conflict detection

A block may carry `expected_hash` — the hash the client last saw. The worker
compares it against the stored block before writing and fails the push rather
than overwriting, so a block another client changed underneath this one is not
silently clobbered. The block diff exchange also declares a `conflicts` list
beside `needed` and `deleted`; the hash comparison does not populate it yet, so
today the guard is enforced at ingestion rather than during negotiation. `kapi
push --force` re-uploads unchanged blocks; it is not a conflict override.

Context entries carry a `content_hash` too, but it serves idempotence rather
than refusal: an entry whose stored hash is unchanged is left alone rather than
rewritten with identical values, which would churn the row's timestamp and make
"when did this collection last change?" unanswerable. The hash covers the
collection name, its coordinates, its resolved governance and its owner, and
folds in the authored profile content — so editing the voice a collection is
governed by changes the entry's hash and the push carries the new governance
instead of skipping it. Server-assigned state (the collection id, timestamps,
the resolved profile id) is deliberately excluded: the hash answers "has the
recipe changed?", and including anything the server minted would make the answer
always yes.

Where the two sides genuinely disagree about a collection, the recipe wins by
claiming it. A workspace-owned collection that a recipe now declares becomes
recipe-owned on the next push — declaring a collection in `kapi.yaml` is the act
that claims it, and after that the ownership field has something to say on git's
behalf.

### Content layer: the `SyncChunk` envelope

Each chunk is a typed envelope carrying a batch of one content type. Protobuf
gives forward and backward compatibility: fields are additive and an older peer
ignores what it does not know.

```protobuf
message SyncChunk {
  string content_type = 1;
  int32 record_count = 2;

  // Exactly one of these is populated (determined by content_type).
  repeated SyncBlock blocks = 10;
  repeated SyncTerm terms = 11;
  repeated SyncMemoryEntry memory_entries = 12;
  repeated SyncMedia media = 13;
  // Future: repeated SyncQAResult qa_results = 14;
  // Future: repeated SyncActivity activities = 15;
}
```

`blocks` is the only route the worker implements. The rest fail the job by
name, which is the honest failure: a push that reports success while discarding
a payload is worse than one that fails, because only the second is recoverable.

`SyncBlock` carries the full Block model across the boundary — structured
source and target segments, properties, annotations, skeleton, display hints,
connector data, run-anchored overlays, source locale, and referent flag — so a
round-trip through Bowrain loses nothing. Its run and segment content embeds
the canonical content-model schema rather than redefining it; see
[AD-framework-034: Content-Model Wire Schema](https://neokapi.github.io/contribute/architecture/034-content-model-wire-schema).

### Context is a content type

A project declares where its content sits and what governs it. The space has two
axes — the product the content belongs to and the channel it ships on — and it
is structural: a key under `profiles:` is a product, the channels that profile
declares are the channels that product ships on, and each named content
collection names its point with one `channel:` reference. This is the
framework's model; see
[the kapi project file](https://neokapi.github.io/contribute/implementation/kapi-project-file).

What travels is not that model but its outcome. **The client resolves it and
sends the result**: one flat entry per declared content collection, carrying the
collection's coordinates, the governance those coordinates resolved to, and the
voice profile itself. The profile declarations themselves are never sent.

That is the decisive property. `core/project.ResolveGovernance` is the one
resolver — the same code path a local `kapi` run uses to decide which voice
governs a collection — and its answer is what the server stores. Sending the
declarations instead would put a second resolver on the server, and two
implementations of a resolution rule diverge: the same content would be governed
by one voice locally and another in the workspace, with neither obviously wrong.
The server stores a resolved point; it does not re-derive one.

```protobuf
message SyncContextEntry {
  string name = 1;
  map<string, string> coordinates = 2;
  string channel = 3;
  string voice_profile = 4;
  bytes voice_profile_json = 5;
  string owner = 6;
  string content_hash = 7;
}
```

- `name` is the collection name from `collections[].name`. A bare entry that
  declares no collection is not carried: its files sync ungrouped, governed by
  the project's default point.
- `coordinates` is the collection's `channel:` resolved into the two axes —
  `product` carrying the profile name and `channel` carrying the channel within
  it (`core/project.ProductAxis` / `ChannelAxis`). Values are slugs, never
  concept references. A concept is designed to be renamed and deprecated as
  vocabulary is revised, and governance that moved when someone edited a term
  would be governance nobody could rely on. A profile or a channel may carry a
  concept for display; resolution never reads it, and nothing on this wire
  resolves one into the other.
- `channel` is the collection's channel, resolved by the client. It selects the
  matching override inside the voice profile.
- `voice_profile` is the *name* of the governing profile — the linkage key the
  workspace brand hub upserts by. Empty when the collection's point binds no
  voice.
- `voice_profile_json` is that profile's authored content, so the push carries
  the governance rather than a reference to one the server may not hold. Sent
  once per distinct profile name in a push; later entries naming the same
  profile carry only the name.
- `owner` and `content_hash` are covered above and below.

`name` and every coordinate value become identifiers on the server: items are
linked to a collection by name, and the coordinates are stored as the point the
governance was resolved at. Renaming a collection in the recipe therefore does
not rename it on the server — it declares a new one and leaves the old one
reported but undeclared.

The consequences of being a content type rather than being special:

- **One hash.** `context_hash` sits in the init request beside the block root
  hash, so an unchanged context costs one comparison.
- **One transaction.** Context entries ride the same `upload_id`, the same
  commit manifest, and the same worker job as the blocks pushed with them, and
  the worker reconciles them *before* it stores the chunks — an item naming a
  collection that has not been created yet would be stored ungrouped and stay
  that way until the next push. One push is one consistent state, or it is a
  failed push.
- **The manifest is the carrier, not a chunk.** Context has no `SyncChunk`
  field. A chunk is an independently uploaded, independently retryable blob that
  the commit merely names; the manifest is the one thing already applied
  atomically with the items. Context is small enough that splitting it out would
  buy nothing and cost the atomicity that is its entire point.

  ```protobuf
  repeated SyncContextEntry contexts = 10;
  ```

- **Items name their collection.** `SyncItemMeta` carries the collection whose
  glob claims the item's path, which is the linkage that gives an item a place
  in the declared structure:

  ```protobuf
  string collection = 4;
  ```

  Empty is not an error — an ad-hoc file syncs ungrouped and the project's
  default point governs it.
- **Pull carries the same entries.** `SyncPullResponse` has a symmetric field:

  ```protobuf
  repeated SyncContextEntry contexts = 14;
  ```

  Unlike blocks it is not cursor-driven. The declared context is small and
  always current, so every page carries the whole of it and the last page's is
  the one to keep. The client records it in the sync cache and reports what
  diverges; see [What a pull does with context](#what-a-pull-does-with-context).

### Ownership travels in the payload

Every context entry carries its owner, as a string:

```protobuf
  string owner = 6;
```

The two values are `"recipe"` — `kapi.yaml` declares it, so git is authoritative
— and `"workspace"` — created and governed on the server, by the web hub, the
editor or a connector, so the recipe has no say.

**A string rather than a proto enum, deliberately.** The manifest is protobuf in
name only: it crosses the client, the commit handler and the worker as JSON,
serialized from the client's Go struct and decoded by the worker with
`encoding/json`. A proto enum crosses that boundary as an opaque integer — `1`
and `2` in a manifest blob that a human debugging a failed push has to decode
against a generated file. A string survives every hop legible, and the values
are compared through one normalizing helper rather than by scattered literals.

`kapi.yaml` is a file in a repository. What it declares is authored, reviewed,
and versioned there, and a workspace that edits it is editing a copy that the
next push overwrites. What the workspace itself creates has no counterpart in
any repository, and a push that deleted it because no recipe mentions it would
be destroying state its owner never handed over.

Ownership is a fact about an entity, so it lives on the entity, and every
decision that turns on it is one lookup of one field.

**An unrecognized or absent owner resolves to `"workspace"`, unconditionally.**
The default is the conservative one and it does not depend on which channel the
entry arrived by. A collection created before this content type existed carries
no owner, and reading it as recipe-owned would hand authority over it to a
`kapi.yaml` that never mentioned it. The server owns what the recipe has not
claimed — and the recipe claims a collection by declaring it, which the next
push does explicitly rather than by default. The normalization runs on the way
out of the store as well as on the way in, so no caller sees a raw empty value.

Where the field is read today:

1. **The push diff skips what the recipe does not own.** The server folds its
   context hash from recipe-owned collections only, and reports only
   recipe-owned ones as undeclared. A workspace-owned collection was never the
   recipe's to declare, so its absence from a push says nothing about it.
2. **The worker reconciles against it.** A declared collection is created or
   updated as recipe-owned; a workspace-owned one that the recipe now declares
   is claimed; an undeclared recipe-owned one is reported and left standing.
3. **A pull leaves recipe-owned governance alone.** See below.

The API-level refusal is the next place the field applies and is not wired yet:
the collection endpoints gate edits on the collection's kind and connector
config rather than on `owner`, which reaches the right answer for a
recipe-pushed collection — it is created connected, so the API renders it
non-editable — by a route that does not generalize. Keying that gate on `owner`
is what makes the answer identical in the API, in the pull path, and in every
surface added later.

#### What a pull does with context

A pull carries every collection the server holds, workspace-owned and
recipe-owned alike. Withholding the ones the client may not act on would leave
it unable to tell "not mine to touch" from "not there at all".

A workspace-owned entry is the server's: the pull records it, and that is the
whole of it, because there is no local governance for it to conflict with. A
recipe-owned entry is git's: the pull records what the server holds, compares it
with what the recipe resolves, and **reports** a difference rather than resolving
one. Nothing on this path writes `kapi.yaml` or the local brand store — the
entries land in the sync cache, which is observation, and what the client
produces for its caller is a list of divergences to print.

Governance that a pull could rewrite is governance nobody can rely on: the same
content would resolve to a different voice depending on where the loop last ran.

The comparison covers the coordinates and the channel, not the voice profile.
The server holds the name of a profile in its brand hub while the recipe binds a
file, a starter pack, or a local store entry — comparing those would report a
divergence on every pull of a project whose voice is a file, which is most of
them. A collection the recipe does not declare at all is not reported here
either: the push path already reports it as an undeclared collection, and saying
it twice in two vocabularies helps nobody.

### Brand voice is context, not a side call

A brand voice profile bound by a recipe is governance attached to a region of
the project's context space. It travels on the context entries for the
collections it governs — the name in `voice_profile`, the authored content in
`voice_profile_json` — in the same push as the collections themselves, under the
same hash and the same ownership rule.

That places it under this protocol's guarantees rather than beside them. A
push cannot leave content stored against governance that never landed, which is
what a REST upload issued after the content transport had finished could not
promise. The upsert is the worker's: matched by name within the workspace, a
no-op when the authored content is identical, and otherwise a new version
through the store's profile versioning, so a server-side edit is superseded by
something revertible rather than clobbered. An entry naming a
profile the hub does not hold, carrying no content, binds nothing — the
collection is better ungoverned than governed by a same-named profile nobody
meant.

`kapi push --no-brand` drops the authored content and the name binding from
every entry. The collections and their coordinates still travel: a project's
structure is not a brand decision. The client reports which voice it declined to
carry rather than saying nothing at all.

An unclaimed project — one not yet in a workspace — has no brand hub to bind in,
so the reconcile settles structure alone. That is the honest outcome rather than
a failure.

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
route — so carrying context per stream adds a scope to the entries, not a
dimension to the protocol.

### Transport layer

The API server never handles content bytes. Chunks move between the client and
object storage, and the API process authorizes, negotiates, and records.

A push is: init, then a block diff per changed item, then chunk uploads, then a
commit that returns `202 Accepted` with a push identifier. A pull is a single
cursor-paginated request returning changed records since the cursor, plus the
current context on every page. The client polls the push status endpoint for
ingestion progress.

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
and item, item hashes keyed by project, held in Redis under a time-to-live and
falling back to a database query on a miss. Without it the server would load
100K hashes per init on a large project. The cache is optional — a deployment
without Redis reads through to the store every time — and it is bounded by the
time-to-live rather than invalidated on write, which is why the interval is
short.

### Async ingestion

The API process validates, negotiates, receives the manifest, enqueues a job,
and returns. The worker reads the manifest, reconciles the context it declares,
then reads chunks from storage, verifies each against its declared hash,
decompresses, routes by content type, and stores — grouping records by item and
writing in batches under one transaction per item, with multi-row upsert rather
than per-row insert. It publishes a push-completed event carrying the counts and
the undeclared collections, and deletes the manifest blob.

An unparseable context payload fails the push rather than storing the items into
a structure that was only half described, for the same reason unparseable item
metadata does.

### Endpoints

```
POST /api/v1/:ws/:id/sync/:stream/push/init
POST /api/v1/:ws/:id/sync/:stream/push/diff
PUT  /api/v1/:ws/:id/sync/:stream/push/chunks/:uploadId/:index
POST /api/v1/:ws/:id/sync/:stream/push/commit
GET  /api/v1/:ws/:id/sync/:stream/pull
GET  /api/v1/:ws/:id/sync/:stream/status?push_id=X
```

The stream is a path segment on every route, defaulting to `main`. A project not
yet claimed into a workspace uses the flat equivalents under
`/api/v1/projects/:id/sync/:stream/…`, authorized by claim token or by JWT.

Workspace-scoped routes enforce tenancy at the transport layer; the server
authorizes against the path-scoped project and overwrites any project, actor, or
workspace the client supplies in a body, so a request cannot address a tenant it
was not routed to.

### Operational alignment

The API process and the worker must use the same blob store. Misaligned stores
produce push jobs that fail with a chunk download error and no other symptom,
because every earlier step succeeds. Local development pins both to the same
directory; deployed environments pass both the same bucket configuration.

## Consequences

- One transport carries content and the structure it is stored into. Brand voice
  lands or fails with the content it governs; terminology and media do not yet,
  and until they travel the protocol they carry none of its guarantees.
- Removals are reported by the side that notices them and applied by neither.
  For blocks that is a consequence of the client hashing only what changed, and
  authoritative hashes are the stated precondition for changing it. For context
  it is a decision: the map is complete, and a recipe edit is still not consent
  to drop the content grouped under a collection.
- Ownership is data. Whether an entry may be changed is one lookup of one field,
  the same lookup in the diff, the worker, and the pull path — and the same one
  the API gate will read when it moves off collection kind.
- Context resolution has one implementation. The client resolves axes and
  profile bindings and sends the result, so a collection's governance cannot
  differ between a local run and the workspace.
- A stream is the only branching concept. Experiments, releases, and branches
  reuse the machinery streams already have, and per-stream governance falls out
  of a resolution tier that exists.
- The init request stays small at any project size: item-level hashes, not
  block-level, and a context whose whole state folds into one hash.
- The API process never touches content bytes, so push load is bounded by
  negotiation cost rather than by payload size.
- A conflicting block is refused at ingestion rather than overwritten, and the
  push fails loudly enough to be retried.

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
