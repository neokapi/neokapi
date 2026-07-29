---
sidebar_position: 8
title: "Bowrain Sync Protocol"
---

# Bowrain Sync Protocol

This note provides implementation details for [AD-009](/architecture-decisions/009-sync-protocol) and [AD-010](/architecture-decisions/010-bowrain-cli-and-project-model).

## Recipe (`kapi.yaml`) Full Schema

```yaml
version: v1
name: my-app

# Project-wide defaults for language and organization.
defaults:
  source_language: en
  target_languages: [fr, de, ja]
  collection: ui/strings

# Content collections: which files to track.
content:
  - path: src/locales/**/*.json
    format: json

  - path: content/docs/**/*.md
    format: markdown
    target: i18n/{lang}/docs/{path}/{filename}

  - path: src/es/**/*.json
    format: json
    source_language: es     # Override source language for this entry
    collection: spanish-ui  # Override collection for this entry

# Plugins: map of name -> version constraint.
plugins:
  okapi-bridge: "^1.47.0"

# Bowrain server connection. Optional — its presence enables push/pull/sync.
server:
  # Compound URL encoding server, workspace, and project ID.
  # Formats:
  #   https://bowrain.example.com/my-team/abc123     (workspace project)
  #   https://bowrain.example.com/projects/abc123     (direct project, no workspace)
  url: https://bowrain.example.com/my-team/abc123
  # Stream determines which content stream to sync with.
  # Default: $auto (detect from git branch / CI environment)
  # Explicit: "main", "v2.0", "feature/new-ui"
  stream: $auto

# Hooks: flows that run automatically at lifecycle points.
hooks:
  pre-push: [qa, term-enforce]
  post-pull: [update-stats]

# Flow definitions (inline; or in .kapi/flows/<name>.yaml).
flows:
  pseudo:
    steps:
      - tool: pseudo-translate
        config: { method: extended }
```

**Field descriptions:**

- **`server.url`** -- Compound URL encoding server, workspace, and project ID. Parsed on demand via `ParseProjectURL()`. Accessor methods: `ServerURL()`, `ProjectID()`, `Workspace()`, `HasServer()`. Claim tokens for anonymous projects are stored in `.kapi/cache/sync-cache.json` (gitignored), not in the URL.
- **`server.stream`** -- Content stream name. Defaults to `$auto` (auto-detect from git branch or CI environment variables). Set to a specific name like `v2.0` to pin the stream. See [AD-005](/architecture-decisions/005-streams) for full stream design.
- **`defaults.source_language`** -- BCP-47 tag for the project's default source language (e.g., `en`)
- **`defaults.target_languages`** -- Array of BCP-47 tags for target languages. When empty, the CLI falls back to server-side target locales (cached in the sync cache).
- **`defaults.collection`** -- Default collection for organizing content on the server
- **`content`** -- Array of content collections defining which files to track (see below)
- **`plugins`** -- Map of plugin name to semver constraint (e.g., `okapi-bridge: "^1.47.0"`)
- **`hooks`** -- Flow names to run on events (`pre-push`, `post-pull`)
- **`flows`** -- Inline flow definitions (file-per-flow definitions also supported under `.kapi/flows/`)

## Content Collections

Content collections define which files to track and how they map to the server:

```yaml
content:
  - path: src/locales/{lang}/*.json
    format: json
    target: src/locales/{lang}/*.json
    base: src/locales/
    collection: ui
    source_language: en
    target_languages: [fr, de]
```

**Fields:**

- **`path`** -- Glob pattern for source files (relative to project root). May contain `\{lang\}` placeholder expanded with the source language.
- **`target`** -- (Optional) Output path pattern for translated files. May contain `\{lang\}` for target locale, or `\{locale\}`, `\{path\}`, `\{filename\}` for legacy-style templates.
- **`format`** -- Format ID (from FormatRegistry: `json`, `html`, `markdown`, etc.) — string or object form (`{ name, config, preset }`). Use `$auto` or omit for auto-detection by file extension.
- **`base`** -- (Optional) Path prefix to strip when reporting files to the server (e.g., `src/locales/` so the server sees `en/messages.json` instead of `src/locales/en/messages.json`).
- **`collection`** -- (Optional) Override the default collection for this content entry. Sent with each block during push.
- **`source_language`** -- (Optional) Override the project's default source language for this entry. Enables projects with multiple source languages.
- **`target_languages`** -- (Optional) Override the default target languages for this entry.

### Per-Entry Language Override

Projects with content in multiple source languages use the `language` field:

```yaml
defaults:
  source_language: en

content:
  - path: src/en/**/*.json
    format: json
    # Uses defaults.source_language (en)

  - path: src/es/**/*.json
    format: json
    source_language: es # This content is in Spanish, not English
```

The `EffectiveLanguage()` method on `ContentEntry` resolves the per-entry language, falling back to the project default. All code paths that expand `\{lang\}` placeholders use this method.

### Dynamic Target Languages

When `defaults.target_languages` is empty, the CLI automatically fetches target locales from the server during pull. The resolution order is:

1. CLI flags (`--locales fr,de`)
2. `defaults.target_languages` in config
3. Server-side target locales (cached in `.kapi/cache/sync-cache.json` as `server_meta`)

Server metadata is fetched via `GET /api/v1/projects/:id` and cached locally so subsequent operations don't require a network round-trip.

### Collections

Collections organize content on the server. They are resolved per content entry:

1. `content[].collection` (per-entry override)
2. `defaults.collection` (project-wide default)

Collections are sent with each block during push via the `collection` field in `BlockInput`.

## Sync Cache (`.kapi/cache/sync-cache.json`) Format

```json
{
  "server_url": "https://bowrain.example.com",
  "project_id": "abc123",
  "sync_cursor": 4821,
  "last_sync": "2026-02-15T10:30:00Z",
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
  "server_meta": {
    "target_locales": ["fr", "de", "ja"],
    "fetched_at": "2026-02-15T10:30:00Z"
  }
}
```

**Key fields:**

- **`sync_cursor`** -- Monotonic sequence number from the server's change log. Used by `pull` to request only changes since the last sync (`WHERE seq > cursor`). This follows the Contentful sync token / CouchDB sequence ID pattern.
- **`last_sync`** -- Timestamp of the last successful push or pull.
- **`claim_token`** -- Claim token for anonymous projects. Stored here (gitignored) rather than in the recipe to avoid accidentally committing credentials to version control. Cleared after `kapi auth claim` transfers ownership.
- **`files`** -- Per-file entries keyed by relative path. Each entry tracks the file's mtime, size, and a map of block ID → content hash (SHA-256). Used by `push` to diff local blocks against the last known server state and send only changed blocks.
- **`server_meta`** -- Cached project metadata from the server, including target locales. Updated on each push/pull. Used to resolve dynamic target languages when `defaults.target_languages` is empty.

**Design principles:**

- **Cache, not state**: The sync cache can be deleted and regenerated. Deleting it forces a full re-scan on the next push (expensive but correct). The server is the source of truth.
- **Block-level granularity**: Tracks individual block hashes, not file-level hashes. When one string changes in a 100-string file, only that block is pushed.
- **Gitignored**: Contains local-only data. Each developer's cache tracks their own sync position.
- **Server metadata caching**: Target locales and other project metadata are cached locally to avoid redundant API calls. The cache is refreshed on each sync operation.

## Push Algorithm (Cursor-Based)

```
0. Resolve stream: --stream flag > BOWRAIN_STREAM env > server.stream > $auto > "main"
1. Scan local files -> extract blocks -> compute hashes
   - Each content entry uses its effective language for {lang} expansion
   - Collections resolved per-entry (entry override > default)
2. Diff block hashes against .kapi/cache/sync-cache.json -> identify changed blocks
3. Send changed blocks to server: POST /projects/:id/sync/push (batched)
   - Each block includes: id, text, name, type, item_name, collection
   - X-Bowrain-Stream header sent for non-main streams
4. Server appends to change log (scoped to stream), returns new cursor
5. Fetch project metadata from server (best-effort) -> cache in .kapi/cache/sync-cache.json
6. Update .kapi/cache/sync-cache.json with new hashes + cursor + server metadata
```

## Pull Algorithm (Cursor-Based)

```
0. Resolve stream: --stream flag > BOWRAIN_STREAM env > server.stream > $auto > "main"
1. Fetch project metadata from server -> cache target locales
2. Resolve target locales: CLI flags > config > server cache
3. Read sync_cursor from .kapi/cache/sync-cache.json
4. Query server: GET /projects/:id/sync/pull?cursor=X&locales=fr
   - X-Bowrain-Stream header sent for non-main streams
5. Server returns only changes since cursor (O(changes), not O(total))
6. For each changed item, fetch blocks and write translated files
7. Update .kapi/cache/sync-cache.json with new cursor + server metadata
```

The append-only change log with sync cursors follows the industry-standard pattern used by Contentful (sync tokens), CouchDB (sequence IDs), and Firebase (timestamp-based feeds).

## Server API Endpoints

When `url` is configured, kapi uses the Bowrain Server REST API ([AD-011](/architecture-decisions/011-rest-api)):

**Sync API endpoints:**

```
POST /api/v1/projects/:id/sync/push       # Push source blocks to server
GET  /api/v1/projects/:id/sync/pull        # Pull changes since cursor
GET  /api/v1/projects/:id/sync/blocks      # Get blocks for an item
GET  /api/v1/projects/:id/sync/status      # Push status (translation job tracking)
GET  /api/v1/projects/:id                  # Project metadata (languages, name)
POST /api/v1/projects/:id/sync/translate   # Create translation job for pushed content
GET  /api/v1/projects/:id/changes          # Raw change log query
```

**Stream API endpoints** ([AD-005](/architecture-decisions/005-streams)):

```
GET    /api/v1/projects/:id/streams                    # List streams
POST   /api/v1/projects/:id/streams                    # Create stream
GET    /api/v1/projects/:id/streams/:name              # Get stream info
DELETE /api/v1/projects/:id/streams/:name              # Archive stream
POST   /api/v1/projects/:id/streams/:name/merge        # Merge into parent
GET    /api/v1/projects/:id/streams/:name/diff          # Diff against parent
```

Push and pull endpoints accept the `X-Bowrain-Stream` header to target a specific stream. When absent, operations target the `main` stream. The server auto-creates a stream on first push if it doesn't exist.

Workspace-scoped equivalents are also available at `/api/v1/workspaces/:ws/projects/:id/sync/...`.

**Push workflow:**

```
1. Read local files via FormatRegistry -> extract blocks
2. Compute block hashes (BlockIdentity SHA-256)
3. Compare with .kapi/cache/sync-cache.json -> identify changed blocks
4. Resolve collections per content entry
5. Run pre-push automations (if configured; recipe `hooks:` are validated but not yet executed — see /cli/flows/hooks)
6. POST /api/v1/projects/:id/sync/push
   -> Request body: { blocks: [{id, text, name, type, item_name, collection}] }
   -> Response: { stored: N, new_cursor: X, push_id: "..." }
   -> Batched at 1000 blocks per request (MaxBlocksPerRequest)
7. GET /api/v1/projects/:id -> cache server metadata (target_locales)
8. Update .kapi/cache/sync-cache.json with new hashes + cursor + server metadata
```

**Pull workflow:**

```
1. GET /api/v1/projects/:id -> cache server metadata (target_locales)
2. Resolve target locales: CLI flags > config > server cache
3. Read sync_cursor from .kapi/cache/sync-cache.json
4. GET /api/v1/projects/:id/sync/pull?cursor=X&locales=fr,de
   -> Response: { changes: [...], new_cursor: Y, has_more: bool }
   -> Paginated: follow has_more until all changes consumed
5. For each item with changes:
   -> GET /api/v1/projects/:id/sync/blocks?item_name=...
   -> Write translated file for each target locale
6. Update .kapi/cache/sync-cache.json with new cursor + server metadata
```

**Server-side change log:**

The server maintains an append-only change log (`change_log` table) that records every mutation to a project's blocks. Each entry has a monotonic sequence number (`seq`). Sync queries are O(changes) via indexed cursor lookup -- the server never needs to diff entire version snapshots.

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
- The manifest's `items` array is decoded into `[]pb.SyncItemMeta` with
  `encoding/json`, so it is matched against the **generated Go struct's JSON
  tags** — `block_index_json`, `preview_html` — not against the protobuf field
  names. A producer struct whose tag differs decodes to the zero value with no
  error.

The JSON status vocabulary is narrower than the proto's: `SyncPushInitResponse`
declares `unchanged | diff_required | full_upload`, and the server currently
returns `unchanged` or `diff_computed`. Clients branch on `unchanged` and treat
everything else as "negotiate".

## Context sync

The `context` content type carries a project's declared context space — its
axes, its content collections and their coordinates, the profile bindings that
govern regions of it, and the voice profiles those bindings name. See
[AD-009](/architecture-decisions/009-sync-protocol) for why it is a content
type rather than a side call.

### Message shapes

```protobuf
// Owner records which side authored an entity. A recipe-owned entity is
// declared in kapi.yaml and versioned in git; a workspace-owned one exists
// only on the server. OWNER_UNSPECIFIED resolves by arrival channel: an entity
// pushed without an owner is recipe-owned, one created through the workspace
// API is workspace-owned.
enum Owner {
  OWNER_UNSPECIFIED = 0;
  OWNER_RECIPE = 1;
  OWNER_WORKSPACE = 2;
}

// SyncContextEntry is the envelope for one context entity. Exactly one of the
// entity fields is populated, determined by kind — the same idiom SyncChunk
// uses for content types.
message SyncContextEntry {
  string key = 1;            // stable identity within the project's context
  string kind = 2;           // "axis" | "collection" | "profile" | "voice"
  Owner owner = 3;
  string content_hash = 4;   // SHA-256 over the entity, see below

  SyncAxis axis = 10;
  SyncCollection collection = 11;
  SyncProfileBinding profile = 12;
  SyncVoiceProfile voice = 13;
}

// SyncAxis is one declared dimension: coordinates.<name> in the recipe. An
// axis with no values is open — any well-formed slug is accepted on it.
message SyncAxis {
  string name = 1;
  repeated SyncAxisValue values = 2;
}

// SyncAxisValue keeps the slug and the concept reference separate. The slug is
// the identity; the concept is display metadata and takes no part in matching,
// so resolving one into the other on the wire would make governance move when
// vocabulary is revised.
message SyncAxisValue {
  string id = 1;
  string concept = 2;  // optional concept reference, e.g. "term:9a1c0f42b7"
}

// SyncCollection is one named content collection and the point it sits at.
message SyncCollection {
  string name = 1;
  repeated string item_patterns = 2;   // glob patterns, in declared order
  map<string, string> coordinates = 3; // axis → value; empty = project default point
  string source_language = 4;
  repeated string target_languages = 5;
  string base = 6;
}

// SyncProfileBinding binds governance to a region of the space. An empty match
// is the project's base. Among matching bindings the one matching on the most
// coordinates governs; a tie is a load error, never a coin flip.
message SyncProfileBinding {
  map<string, string> when = 1;
  string voice_profile = 2;  // name of a SyncVoiceProfile in the same push
  string terms = 3;          // terms store path, as written in the recipe
}

// SyncVoiceProfile carries the profile content itself, so a binding never
// names governance that is not present.
message SyncVoiceProfile {
  string name = 1;
  int32 version = 2;
  bytes profile_json = 3;  // the serialized VoiceProfile
}
```

Added fields on the existing messages:

```protobuf
message SyncPushInit {
  // … 1–7 unchanged …
  string context_hash = 8;
}

message SyncPushInitResponse {
  // … 1–8 unchanged …
  bool context_changed = 9;
  // Context keys the server holds that the client's context_hashes map does
  // not name — the context counterpart of deleted_items.
  repeated string deleted_context = 10;
}

message SyncChunk {
  // … 10–13 unchanged; 14 and 15 stay reserved for qa_results and activities …
  repeated SyncContextEntry context = 16;
}

message SyncManifest {
  // … 1–9 unchanged …
  // The COMPLETE set of context entities the client declares, key → content
  // hash, whether or not this push uploaded them. This is the reconciliation
  // input: an entity the server holds and this map does not name is one the
  // recipe no longer declares.
  map<string, string> context_hashes = 10;
}

message SyncPullResponse {
  // … 10–13 unchanged; 14 and 15 reserved, matching SyncChunk …
  repeated SyncContextEntry context = 16;
}
```

`SyncChunk.context` and `SyncPullResponse.context` deliberately share field
number 16, keeping the push and pull envelopes aligned field-for-field the way
blocks (10), terms (11), memory entries (12), and media (13) already are.

### Hash construction

Every hash in the protocol uses one construction, already implemented for the
block levels as `ComputeItemHash` and `ComputeRootHash` in
`bowrain/core/sync/convert.go`:

```go
// hashMap returns SHA-256 over the entries sorted by key, writing each key
// followed by its value. No delimiter is needed: every value is a 64-character
// hex digest, so the boundary between value and next key is unambiguous.
func hashMap(entries map[string]string) string {
	keys := slices.Sorted(maps.Keys(entries))
	h := sha256.New()
	for _, k := range keys {
		h.Write([]byte(k))
		h.Write([]byte(entries[k]))
	}
	return hex.EncodeToString(h.Sum(nil))
}
```

`context_hash` is that function over `key → content_hash` across every context
entity the client declares — all four kinds in one map, which is what lets a
single comparison settle whether any part of a project's context moved.

A per-entity `content_hash` is SHA-256 over the entity's fields written in
declared order, with every map written in sorted key order and empty optional
fields omitted. Field order is declared rather than serialized because
protobuf's wire output is not canonical: two encoders may order fields
differently and produce different bytes for identical content, which would make
an unchanged entity look changed on every push from a different client.

Keys are stable and kind-scoped, so two kinds cannot collide:

| Kind | Key |
| --- | --- |
| `axis` | `axis:<name>` |
| `collection` | `collection:<name>` |
| `profile` | `profile:<rendered match>`, e.g. `profile:{channel: docs, product: kapi}` |
| `voice` | `voice:<profile name>` |

The rendered match is the same stable rendering the framework uses to reject
two profiles claiming one point, so a recipe that loads has no key collisions
by construction.

### Reconciliation

On **push**, for each key in the union of the client's `context_hashes` and the
context the server holds for the stream:

| Client declares | Server holds | Server-side owner | Action |
| --- | --- | --- | --- |
| yes | no | — | create, owner `recipe` |
| yes | yes, same hash | `recipe` | no-op; not uploaded |
| yes | yes, different hash | `recipe` | update from the pushed entity |
| yes | yes | `workspace` | refuse: reported as an ownership conflict, not overwritten |
| no | yes | `recipe` | remove; reported in `deleted_context` |
| no | yes | `workspace` | keep — not the recipe's to delete |

The two refusal rows are the ones that earn the ownership field. Without it,
row four silently converts a workspace's own entity into a recipe-managed one,
and row six deletes state whose owner never handed it over.

On **pull**:

| Owner of the arriving entity | Locally declared | Action |
| --- | --- | --- |
| `recipe` | yes | update the local view of server state; never rewrite `kapi.yaml` |
| `recipe` | no | the server holds a recipe-owned entity this recipe dropped; surfaced as pending removal, applied by the next push |
| `workspace` | — | store in the project's local view, marked read-only |

The read-only marking is data, not a UI rule: `kapi` surfaces and the web
surfaces both render editability from `owner`, so a surface added later
inherits the behaviour rather than reimplementing the check.

### Ordering within a push

Context entities are reconciled **before** blocks in the same push. A block
whose item belongs to a collection declared in the same push must find that
collection present, and governance that changed must be in force for the
content arriving under it. Both directions of the alternative are wrong: blocks
first leaves content briefly ungoverned, and splitting the two into separate
pushes reintroduces the half-succeeding side call the content type exists to
remove.
