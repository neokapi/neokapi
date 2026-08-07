---
sidebar_position: 8
title: "Bowrain Sync Protocol"
---

# Bowrain Sync Protocol

This note provides implementation details for [AD-009](/architecture-decisions/009-sync-protocol) and [AD-010](/architecture-decisions/010-bowrain-cli-and-project-model).

## The recipe keys sync reads

The recipe schema is specified once, in
[the kapi project file](https://neokapi.github.io/contribute/implementation/kapi-project-file).
Three parts of it are what the sync protocol reads, and they are the ones worth
naming here:

- **`collections[]`** — the content collections. A named collection (`name:` plus
  `content:`) is what becomes a `SyncContextEntry` and what an item's
  `collection` field points at; a bare entry (a promoted `path:` with no name)
  declares no collection and syncs ungrouped.
- **`profiles:`** — the project's context space and the governance bound to it.
  A key under `profiles:` is a product and the channels it declares are the
  channels that product ships on; a collection's `channel:` names the point its
  content sits at. The declarations do not travel the wire: the client resolves
  them and sends the resolved point per collection. See [Context
  sync](#context-sync).
- **`bowrain:`** — the connection. `url` is a compound URL encoding
  server, workspace, and project id, parsed on demand by `ParseProjectURL()`;
  `stream` names the content stream, defaulting to `$auto` (detected
  from the git branch or the CI environment). Claim tokens for anonymous
  projects live in the sync cache, never in the recipe.

Bowrain adds `bowrain:`, `hooks:`, `automations:`, `assets:` and `brand_voice:`
as top-level keys. The framework loader captures them as unknowns in
`KapiProject.Extras`; the bowrain loader decodes them into typed fields on
`Recipe`. Both loaders round-trip the same file, which is what keeps the
framework free of bowrain-specific knowledge.

### Dynamic target languages

When `defaults.target_languages` is empty, the CLI fetches target locales from
the server during pull. The resolution order is:

1. CLI flags (`--locales fr,de`)
2. `defaults.target_languages` in the recipe
3. Server-side target locales, cached in the sync cache as `server_meta`

Server metadata is cached locally so subsequent operations don't require a
network round-trip.

## Sync Cache (`.kapi/work/cache/sync-cache.json`) Format

```json
{
  "server_url": "https://bowrain.example.com",
  "project_id": "abc123",
  "last_sync": "2026-02-15T10:30:00Z",
  "active_stream": "main",
  "stream_cursors": { "main": 4821 },
  "claim_token": "clm_abc123",
  "files": {
    "src/locales/en.json": {
      "mtime": "2026-02-15T10:25:00Z",
      "size": 4096,
      "blocks": {
        "greeting": "a1b2c3d4...",
        "farewell": "e5f6a7b8..."
      }
    }
  },
  "context_hash": "9f2c...",
  "server_context": {
    "docs": {
      "coordinates": { "channel": "docs", "product": "kapi" },
      "channel": "docs",
      "voice_profile": "Kapi Docs",
      "owner": "recipe"
    }
  },
  "server_meta": {
    "target_locales": ["fr", "de", "ja"],
    "fetched_at": "2026-02-15T10:30:00Z"
  }
}
```

**Key fields:**

- **`stream_cursors`** -- Monotonic sequence number from the server's change log, per stream. Used by `pull` to request only changes since the last sync (`WHERE seq > cursor`). This follows the Contentful sync token / CouchDB sequence ID pattern. `active_stream` records the stream last synced.
- **`last_sync`** -- Timestamp of the last successful push or pull.
- **`claim_token`** -- Claim token for anonymous projects. Stored here (gitignored) rather than in the recipe to avoid accidentally committing credentials to version control. Cleared after `kapi auth claim` transfers ownership.
- **`files`** -- Per-file entries keyed by relative path. Each entry tracks the file's mtime, size, a map of block ID → content hash (SHA-256), and a map of asset source ID → blob key. Used by `push` to diff local blocks against the last known server state and send only changed blocks.
- **`context_hash`** -- The context the last push carried: the fold over every declared collection's entry (`bowrain/core/sync.ContextHashOf`). The local half of the context fast path — an unedited recipe is not worth a round trip, and a recipe that gained a collection or rebound a voice must reach the server even when no content moved.
- **`server_context`** -- The collections the last pull observed on the server, keyed by name, each with its coordinates, channel, bound voice profile name, and owner. An **observation, never an instruction**: nothing derived from a recipe-owned entry is applied to local governance. What it buys is the ability to say so — `kapi status` reports a recipe-owned collection the server governs differently instead of letting the two diverge unremarked.
- **`concept_baseline`** -- The governed concepts and relations a concept pull last wrote into the project's bound terms, so a later concept push can diff local edits against what was pulled.
- **`server_meta`** -- Cached project metadata from the server, including target locales. Updated on each push/pull. Used to resolve dynamic target languages when `defaults.target_languages` is empty.

**Design principles:**

- **Cache, not state**: The sync cache can be deleted and regenerated. Deleting it forces a full re-scan on the next push (expensive but correct), one redundant context reconcile, and one concept re-baseline. The server is the source of truth.
- **Block-level granularity**: Tracks individual block hashes, not file-level hashes. When one string changes in a 100-string file, only that block is pushed. This is also why a push's hashes are not authoritative Merkle roots — see [AD-009](/architecture-decisions/009-sync-protocol).
- **Gitignored**: Contains local-only data. Each developer's cache tracks their own sync position.

## Push and pull, end to end

**Push:**

1. Resolve the stream: `--stream` flag, then `BOWRAIN_STREAM`, then
   `server.stream`, then `$auto`, then `main`.
2. Scan local files via the format registry, extract blocks, compute identities.
3. Diff block hashes against the sync cache — changed blocks only.
4. Resolve the declared context: one entry per named collection, carrying its
   coordinates, its resolved governance, and the authored voice profile.
5. `push/init` sends item hashes, root hash, context hash, and declared
   collection names; the reply names changed, new and deleted items,
   `context_changed`, and undeclared collections.
6. `push/diff` sends block hashes per changed item; the reply names the needed
   blocks and the transport mode.
7. `push/chunks/:uploadId/:index` uploads each batch as a zstd-compressed
   protobuf `SyncChunk`.
8. `push/commit` sends the manifest — chunk refs, item metadata, context
   entries — and returns `202 Accepted` with a push id.
9. Poll `status?push_id=…` for ingestion progress.
10. Update the sync cache: block hashes, stream cursor, context hash, server
    metadata.

**Pull:**

1. Resolve the stream as above.
2. Resolve target locales: CLI flags, then the recipe, then the server cache.
3. Read the stream cursor from the sync cache.
4. `pull?cursor=X&limit=1000&locales=fr,de` returns blocks and media since the
   cursor, plus the current context on every page. Follow `has_more` until
   drained and keep the last page's context.
5. Record the pulled context in the sync cache and report divergences.
6. Write target files per locale, aborting without advancing the cursor if any
   write fails.
7. Update the sync cache with the new cursor and server metadata.

The append-only change log with sync cursors follows the industry-standard
pattern used by Contentful (sync tokens), CouchDB (sequence IDs), and Firebase
(timestamp-based feeds). The server maintains it in a `change_log` table with a
monotonic `seq` per entry, so sync queries are O(changes) via indexed cursor
lookup — the server never diffs entire version snapshots.

A pull's cursor advance is gated on every target file being written. The
server's change query is forward-only (`seq > cursor`), so advancing past an
unwritten change would lose it permanently; a write failure therefore aborts
without saving the cursor and the next pull re-delivers everything.

## Server API endpoints

When `server.url` is configured, kapi uses the Bowrain Server REST API
([AD-011](/architecture-decisions/011-rest-api)). The stream is a path segment,
not a header:

```
POST /api/v1/:ws/:id/sync/:stream/push/init
POST /api/v1/:ws/:id/sync/:stream/push/diff
PUT  /api/v1/:ws/:id/sync/:stream/push/chunks/:uploadId/:index
POST /api/v1/:ws/:id/sync/:stream/push/commit
GET  /api/v1/:ws/:id/sync/:stream/pull
GET  /api/v1/:ws/:id/sync/:stream/blocks
GET  /api/v1/:ws/:id/sync/:stream/status?push_id=X
POST /api/v1/:ws/:id/sync/:stream/translate
```

A project not yet claimed into a workspace uses the flat equivalents under
`/api/v1/projects/:id/sync/:stream/…`, authorized by claim token or by JWT. The
client picks between the two forms by whether it holds a workspace slug. An
absent stream segment resolves to `main`, and the server auto-creates a stream
on first push.

**Stream endpoints** ([AD-005](/architecture-decisions/005-streams)):

```
GET    /api/v1/:ws/:id/streams
POST   /api/v1/:ws/:id/streams
GET    /api/v1/:ws/:id/streams/:stream
PATCH  /api/v1/:ws/:id/streams/:stream
DELETE /api/v1/:ws/:id/streams/:stream            # archive
POST   /api/v1/:ws/:id/streams/:stream/restore
POST   /api/v1/:ws/:id/streams/:stream/merge
GET    /api/v1/:ws/:id/streams/:stream/diff
POST   /api/v1/:ws/:id/streams/:stream/lock
POST   /api/v1/:ws/:id/streams/:stream/unlock
```

Authentication uses the token from `kapi auth login`, held in the OS keychain ([AD-002](/architecture-decisions/002-authentication-and-workspaces)).

## Where the wire actually lives

Adding a field to `bowrain/core/proto/sync/v1/sync.proto` is necessary and not
sufficient. The protocol is protobuf in one place and JSON everywhere else:

| Exchange | Encoding | Authoritative shape |
| --- | --- | --- |
| `push/init`, `push/diff`, `push/commit` | JSON | `PushInitRequest` / `PushInitResponse` / `PushDiffRequest` / `PushDiffResponse` / `PushCommitRequest` in `bowrain/core/client/push.go`, and the request structs in `bowrain/server/handlers_sync.go` |
| chunk body | protobuf, zstd-compressed | `SyncChunk` from `sync.proto` |
| commit manifest | JSON, stored as a blob | `PushCommitRequest`, decoded by the worker into `syncPushManifest` (`bowrain/jobs/worker_sync.go`) |
| `pull` | JSON, optionally zstd | `apiclient.RichPullResponse` |

Two consequences for anyone extending the protocol:

- A new negotiation field needs the proto message, the client struct, **and**
  the server's bind struct. A field present in only two of the three is
  silently dropped.
- The manifest's `items` and `contexts` arrays are decoded into
  `[]pb.SyncItemMeta` and `[]*pb.SyncContextEntry` with `encoding/json`, so they
  are matched against the **generated Go struct's JSON tags** —
  `block_index_json`, `preview_html`, `voice_profile_json` — not against the
  protobuf field names. A producer struct whose tag differs decodes to the zero
  value with no error.

That JSON hop is also why `SyncContextEntry.owner` is a `string` rather than a
proto enum. The manifest crosses client → commit handler → worker as JSON; an
enum crosses it as an opaque integer, unreadable in a stored manifest blob
without the generated file to hand.

The JSON status vocabulary is narrower than the proto's: `SyncPushInitResponse`
declares `unchanged | diff_required | full_upload`, and the server currently
returns `unchanged` or `diff_computed`. Clients branch on `unchanged` and treat
everything else as "negotiate".

Several proto fields have no producer and no reader at all:
`SyncPushInit.content_types`, `terms_hash` and `memory_hash`, and the reply's
`terms_changed` / `memory_changed`. They are the shape a terms or
content-memory content type would take, not a path anything travels. Only
`blocks` and `context` cross the protocol on push; media crosses it on pull
only, rendered from the asset store.

## Context sync

The `context` content type carries a project's declared structure and the
governance in force over it: one entry per named content collection, carrying
the point that collection occupies in the project's context space, the voice
resolved for that point, and which side owns it. See
[AD-009](/architecture-decisions/009-sync-protocol) for why it is a content
type rather than a side call.

**Profile declarations do not travel.** The recipe's `profiles:` and each
collection's `channel:` are read by `core/project.ResolveGovernance` on the
client, and what goes on the wire is the resolved answer. One resolver, one precedence model —
the alternative puts a second implementation on the server, and two
implementations of a precedence rule diverge.

### Message shapes

One flat entry per collection, defined in `bowrain/core/proto/sync/v1/sync.proto`:

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

| Field | Built from | Notes |
| --- | --- | --- |
| `name` | `content[].name` | Non-empty. A bare entry declares no collection and is not carried |
| `coordinates` | `content[].context` | Axis → value. Slugs, never concept references |
| `channel` | `ResolvedGovernance.Channel` | The value on `core/project.ChannelAxis`, the one axis the framework reads for itself |
| `voice_profile` | resolved profile name | The linkage key the brand hub upserts by. Empty binds no voice |
| `voice_profile_json` | the authored `VoiceProfile` | Sent once per distinct name per push; empty under `--no-brand` |
| `owner` | `"recipe"` on push | See [Ownership](#ownership) |
| `content_hash` | `ComputeContextEntryHash` | Stamped by the client, stored and compared by the server |

The entries ride on the negotiation and the manifest, not in a chunk:

```protobuf
  string context_hash = 8;
  repeated string collections = 9;
```

```protobuf
  bool context_changed = 9;
  repeated string undeclared_collections = 10;
```

```protobuf
  repeated SyncContextEntry contexts = 10;
```

```protobuf
  repeated SyncContextEntry contexts = 14;
```

(In declaration order: `SyncPushInit`, `SyncPushInitResponse`, `SyncManifest`,
`SyncPullResponse`.) `SyncChunk` has no context field. A chunk is an
independently uploaded blob the commit merely names; the manifest is the one
thing applied atomically with the items, which is the whole reason context rides
there.

Each item names the collection that claims it:

```protobuf
  string collection = 4;
```

on `SyncItemMeta`, resolved client-side by `KapiProject.CollectionForPath`.
Empty is not an error — an ad-hoc file syncs ungrouped and the project's default
point governs it.

### Hash construction

Every fold in the protocol uses one construction, implemented as
`ComputeItemHash` and `ComputeRootHash` in `bowrain/core/sync/convert.go`:
SHA-256 over the entries sorted by key, writing each key followed by its hash.
No delimiter is needed — every value is a 64-character hex digest, so the
boundary between value and next key is unambiguous.

`ComputeContextHash` is that same fold over collection name → entry hash.
`ContextHashOf` is the whole client-side computation in one call: hash each
entry, stamp its `ContentHash`, then fold. Stamping and folding in one place is
what stops the hash the client negotiates on from drifting away from the hashes
it sends in the manifest.

A per-entry `ComputeContextEntryHash` covers, in declared order: the collection
name, the coordinates in sorted key order, the channel, the voice profile name,
the authored profile content, and the normalized owner. Field order is declared
rather than serialized because protobuf's wire output is not canonical — two
encoders may order fields differently and produce different bytes for identical
content, which would make an unchanged entry look changed on every push from a
different client.

Server-assigned state is deliberately excluded: the collection id, the
timestamps, and the resolved profile id. The hash answers "has the recipe
changed?", and folding in anything the server minted would make the answer
always yes.

The server folds its side from the collections it holds **under recipe
ownership only** and compares. Workspace-owned collections are not in the fold,
so creating one in the web hub does not make every client's next push look like
a context change.

### Ownership

`owner` is `"recipe"` or `"workspace"`, compared through
`bowrain/core/sync.NormalizeContextOwner` and `IsRecipeOwned` rather than
against literals.

**Anything else — including empty — normalizes to `"workspace"`.** The default
is the conservative one, and it does not vary by arrival channel: a collection
created before this content type existed carries no owner, and reading it as
recipe-owned would hand authority over it to a `kapi.yaml` that never mentioned
it. Normalization runs on the way out of the store as well as into it
(`bowrain/store/sqlitestore`, `bowrain/store/postgres`), so no caller sees a raw
empty value.

A recipe claims a collection by declaring it. `buildPushContext` stamps every
entry `"recipe"`, and the worker's reconcile sets `Owner` to recipe on create
and on update — including for a row that was workspace-owned, which it reports
as `claimed`.

### Reconciliation on push

`reconcileContext` (`bowrain/jobs/worker_context.go`), per declared entry:

| Client declares | Server holds | Server-side owner | Action |
| --- | --- | --- | --- |
| yes | no | — | create, owner `recipe`, governance written into the collection's config |
| yes | yes, same hash and same governance keys | `recipe` | no-op — `unchanged` |
| yes | yes, different hash or different governance keys | `recipe` | update from the pushed entry — `updated` |
| yes | yes | `workspace` | update and take ownership — `claimed` |
| no | yes | `recipe` | **report, never delete** — `undeclared` |
| no | yes | `workspace` | ignore: never the recipe's to declare |

The governance keys are compared even when the hash matches, because the profile
id is minted server-side and can change without the recipe changing — a hub
rename, or a first bind after the project was claimed into a workspace.

Governance lands in the collection's `connector_config` under the two keys
`core/profile` already reads: `PropertyProfileID` and `PropertyChannel`. Every
other key is carried through untouched — a collection may also hold connector
settings, and governance has no business dropping them. An empty profile id or
channel **removes** the key rather than storing an empty value, so "no voice
bound at this point" reads the same as "never bound one".

The voice itself is upserted by `profileBinder`, matched by name within the
workspace, cached per name so several collections governed by one voice cost one
upsert. Identical authored content is a no-op; changed content goes through
`UpdateProfile`, which archives the current state before bumping the version, so
a server-side edit is superseded by something revertible. An entry naming a
profile the hub does not hold and carrying no content binds nothing — the
collection is better ungoverned than governed by a same-named profile nobody
meant. An unclaimed project has no hub at all, so the reconcile settles
structure alone.

The undeclared list reaches three places, and is acted on by none of them: the
init reply, the worker's log line, and the `undeclared_collections` field of the
push-completed event.

### Reconciliation on pull

`applyPulledContext` (`bowrain/plugin/connector/context.go`) writes only into
the sync cache. It does not touch `kapi.yaml`, does not touch the local brand
store, and does not feed `core/project.ResolveGovernance` — the profile a run
resolves is exactly the one the recipe binds, before the pull and after it.

| Owner of the arriving entry | Locally declared | Action |
| --- | --- | --- |
| `workspace` | — | record in `server_context`; counted as workspace-owned |
| `recipe` | yes, and it agrees | record; nothing reported |
| `recipe` | yes, and it disagrees | record; reported as a **divergence** |
| `recipe` | no | record; nothing reported here — the push path already reports it as undeclared |

Divergence compares the coordinates and the channel. The voice profile is
deliberately not compared: the server holds the name of a profile in its brand
hub while the recipe binds a file, a starter pack, or a local store entry, so
comparing those would report a divergence on every pull of a project whose voice
is a file — which is most of them.

The pull's context is not cursor-driven. Every page carries the current set, and
the client keeps the last page's.

### Ordering within a push

The worker reconciles context **before** it processes any chunk. An item naming
a collection that has not been created yet would be stored ungrouped and stay
that way until the next push, and governance that changed must be in force for
the content arriving under it. Both alternatives are wrong: blocks first leaves
content briefly ungoverned, and splitting the two into separate pushes
reintroduces the half-succeeding side call the content type exists to remove.

An unparseable `contexts` payload fails the job rather than storing the items
into a structure that was only half described — the same reasoning as the item
metadata beside it.
