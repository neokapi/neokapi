---
title: Server API
sidebar_position: 14
---

# Server API

Bowrain provides both REST and gRPC APIs for programmatic access to the platform.

## REST API

The REST API serves on the configured HTTP port under `/api/v1`. Every route
that touches a workspace's data is workspace-scoped: `:ws` is the workspace
slug, `:id` a project id, and `:ref` the stream (`main` when a project has only
one). API tokens (see [Members and roles](/server/members-and-roles#api-tokens))
authenticate as `Authorization: Bearer <token>`.

### Health

```
GET /api/v1/health
```

### Projects

```
GET    /api/v1/:ws/projects              # List projects in a workspace
POST   /api/v1/:ws/projects              # Create a project
GET    /api/v1/:ws/:id                   # Get a project
PUT    /api/v1/:ws/:id                   # Update a project
DELETE /api/v1/:ws/:id                   # Delete a project
GET    /api/v1/:ws/:id/blocks/:ref       # Blocks on a stream (?item=&locale=&status=&q=)
GET    /api/v1/:ws/:id/blocks/:ref/:bid  # One block
```

### Content changes

```
POST /api/v1/:ws/projects/:id/streams/:stream/changes  # Apply a change set to a stream
PUT  /api/v1/:ws/:id/blocks/:ref/:bid/access            # Move a block along the access ladder
POST /api/v1/:ws/:id/blocks/:ref/:bid/rollback          # Restore a translation to an entry of its history
POST /api/v1/:ws/:id/revert                             # Revert the changes recorded under one correlation id
POST /api/v1/:ws/:id/restore                            # Restore a stream to a version or a change-log cursor
GET  /api/v1/:ws/:id/blocks/:ref/:bid/notes             # A block's notes, oldest first
```

A person changes a stream's content, decides on a translation and annotates a
block through the changes route. Its body is a `kapi.change/v1` change set, the
form `kapi apply` and the `apply_edits` tool take (see [Change
sets](https://neokapi.github.io/reference/cli-contract#change-sets-kapi-apply)).
Each operation addresses one edition of one block as `{doc, block, edition}`:
`doc` is the item's path in the stream, `block` is the block's key, its name or
its id, and an empty `edition` is the source. Each operation carries `if_match`,
the revision the sender read. The answer is a `kapi.change-result/v1` document.

A change set that applied, previewed or partly landed is answered `200` with
its status. A refused change set writes nothing, and its status code is the one
its first refusal's code maps to:

| Code | Status |
| --- | --- |
| `invalid` | 400 |
| `not_permitted` | 403 |
| `not_found` | 404 |
| `stale`, `ambiguous`, `doc_changed` | 409 |
| `budget_exceeded` | 413 |
| `guard`, `gate_failed`, `unsupported` | 422 |
| `unreachable` | 503 |

A `stale` refusal carries the edition's current revision and text, so the
sender decides again on the wording that stands. A revision covers an
edition's content, inline codes included; a source edit or a review decision
leaves a translation's revision where it was. The editor's blocks payload
carries each served translation's revision in `target_revisions`, and the
`get_block` tool returns the same values as `revisions`.

The web editor, the review surfaces and the desktop app send every save and
decision with the revision they rendered from `target_revisions`
(`bowrain/packages/ui/src/api/contentChanges.ts`). On a `stale` refusal the
surface shows the person the translation as it stands and sends the operation
again on the refusal's revision only when they choose to. On a `gate_failed`
refusal of a save it lists the operation's failing findings and, when the
person chooses to save anyway, sends the same change set again with
`gate: "report"`, which the server admits from a person and records with the
overridden findings. A translation is always sent as `runs`: a plural as one
plural run with its forms, a content-memory match as the `target_runs` the
match lookup serves, and an inserted term appended to the translation's runs,
so inline codes and plural structure travel with the edit. Notes and entity
marks are `annotate` and `unannotate` operations on the source edition, anchored
at run positions in `source_runs`, which the blocks route serves for any source
that is more than one run. The
desktop sends its change sets through `editorclient.ApplyChanges`; with the
server out of reach it applies one to its local cache and queues it unchanged in
its offline outbox, so the server judges each `if_match` on replay and a queued
edit to a translation that moved meanwhile is refused and marked failed.

The server holds each operation to what the sender may do on the project:

- `set_content`, `replace_text` and `remove_edition` on a translation take the
  `translate` permission for its language. On the source they take
  `edit_source`, and the source of an item a connector syncs is edited at the
  connector. A block whose access is restricted takes an edit from its owner or
  a reviewer of the language, and a published block from a project manager.
  The access route moves a block along that ladder (`open`, `restricted`,
  `published`) and is recorded as `content.access_changed`: restricting or
  publishing takes `review`, un-publishing takes `manage_project`, and a
  publish is held to the workspace's separation-of-duties policy for every
  language it publishes.
- `decide` records a review decision. `establish` takes the `review`
  permission for the language and is held to the workspace's
  separation-of-duties policy; a change set that also writes the translation
  it establishes approves the sender's own wording. `reject` returns the translation to draft and
  `withdraw` to translated; moving an established translation takes `review`.
  Each decision is written to the decision ledger and the workspace's content
  memory, as approve-passing writes it. The server keeps no pre-review, so
  `advise` is refused as `unsupported`.
- A note is an `annotate` of type `note` on the source, which anyone who may
  read the content leaves; the server stamps it with its author and the time
  it landed. An `annotate` under an existing note's id rewrites it and an
  `unannotate` removes it, each sent by its author or a project manager. A
  note on a translation is refused. An entity is an annotation of type
  `entity` on the source, and takes `edit_source`.
- An agent may neither send `gate: report` nor write without the revision it
  read, and records no review decision.

Before a change set lands, the server checks each edition it changes against
the checks in force where the item sits, resolved as the editor's check routes
and the ship gate resolve them. A translation meets the rule checks, with
placeholder integrity against its source, the protected terms and the term
gate. The source meets the voice profile's rules and the workspace's retired,
forbidden and competitor terms. A failing finding refuses the change set with
`gate_failed` and the findings, unless a person sends `gate: report`. Only
deterministic checks run at commit.

The term gate holds an in-memory snapshot of the workspace's terms, which the
commit check, the ship-state pass and the review queue share. Every write to
the terms store gives the workspace's terms a new revision (`tb_revision`, in
the transaction of the write), and the server keeps one snapshot per workspace
under the revision it was read at. A call reads the revision first and reads
the whole terms again only when it moved, so a check costs one query while
nobody writes the terms, on every replica. A write made outside the store, such
as one-off SQL, moves no revision; the next write through the store, or a
restart, brings the snapshot up to date.

The writes of a change set, the block history and the change log land in one
transaction that holds the blocks' rows, and the revision each operation names
is compared inside it, so a change committed between the sender's read and the
write refuses the operation as `stale`. Each history entry carries the sender
and the change set's correlation id, which `revert` takes, and each change set
that lands is recorded as a `content.changed` event.

Rollback, revert and restore apply what they restore as a change set from the
person who asked, against the revision each translation holds when the route
reads it: rollback sets the content of the history entry, inline codes
included, and a translation that did not exist at the restored point is
removed. They read only the history entries that record content: an entry
that records a removal restores none, and a rollback to a decision entry is
refused with `422`. A rollback takes the revision the caller read as
`base_revision`. A translation that moved since the route read it keeps what it
holds. The restored wording lands over the findings it brings back.

### Sync

The sync routes are what the `kapi-bowrain` plugin speaks. A push declares its
tree, uploads only what the server lacks, and commits a manifest; a pull reads
the server's tree and the changes since the client's ref.

```
GET  /api/v1/:ws/:id/sync/:ref/tree             # The declared tree
GET  /api/v1/:ws/:id/sync/:ref/ref              # The server's current ref
GET  /api/v1/:ws/:id/sync/:ref/pull             # Changes since a ref
GET  /api/v1/:ws/:id/sync/:ref/blocks           # Blocks by id
GET  /api/v1/:ws/:id/sync/:ref/status           # Standing of an in-flight push
GET  /api/v1/:ws/:id/sync/:ref/blobs/:key       # Download a blob
POST /api/v1/:ws/:id/sync/:ref/push/init        # Declare the tree; receive what to upload
POST /api/v1/:ws/:id/sync/:ref/push/uploads     # Presigned upload URLs for the chunks
PUT  /api/v1/:ws/:id/sync/:ref/push/chunks/:uploadId/:chunkIndex   # Proxied chunk upload
POST /api/v1/:ws/:id/sync/:ref/push/commit      # Commit the manifest (202; a worker applies it)
```

The push init takes the recipe's `settings` (`converge_policy`,
`translate_after` and `term_rules`, at their effective values) and answers with `settings`, the
values the project holds, and `settings_refused`, the ones this caller's commit
would not apply. The client puts in the commit's `settings` map only a setting
that differs and is not refused, and reports the refusals. The commit requires
the push permission (`manage_files`), validates each value against the recipe
schema (`400` for a value outside it) and decides again against the project as
it then stands:

- A push to a stream other than the project's default applies nothing, and each
  differing setting is refused with `not_default_stream`.
- On the default stream a setting that tightens applies (`translate_after`
  toward `established`, `converge_policy` toward `manual`, `term_rules` that
  keep every rule the project holds). A setting that loosens applies only when
  the caller holds `manage_project`, and is otherwise refused with `loosens` and
  `requires: manage_project`.

`term_rules` carries the recipe's rules as `core/profile.RecipeTermRules`
encodes them: `all` for the rules on flow steps and in `defaults.tools`, and
`locales` for the ones under `defaults.locales.<lang>.tools`. The project keeps
them in its `term_rules` property. The ship-state pass, the review queue and
approve-passing read them through the term gate (`store.RecipeTermRulesOf`),
and the translation and recycle jobs add them to the rules the workspace terms
give, a recipe rule replacing the stored rule for the same term.

Applied settings are written before the push job is queued, so the run the push
starts reads them. Each one is recorded as a `project.setting_changed` audit
event with the actor and the value before and after. The `202` response carries
`settings_applied` and `settings_refused` when either is non-empty.

A pull walks the stream's change log forward from the client's cursor and
serves each changed block under the item it belongs to. A page carries each
block once, at its latest change since the cursor, so a pull from the start
serves every block once, and a block that changes after its page arrives again
on a later page. A block row that
belongs to no item is left out of the response, and the cursor the response
returns still moves past its change, so every client continues from the same
place. The server logs how many such rows a pull left out.

Server work that drafts from the blocks it read, such as the translation and
extraction jobs, pseudo-translation, content-memory translation, applying
content memory in bulk and the passes of a flow run, commits what it produced
as a change set from the tool, against the revision of each edition it read.
The spans a tool marks are guarded by the revision of the edition they lie on.
A block whose source moved, or whose translation a person changed, since the
read keeps what it holds, and the rest of the change set lands; the server logs
how many blocks a commit left out, by code. A removed item stays removed. The
block annotations and properties a tool writes (content-memory candidates,
extracted terminology) land on the rows that hold the content it produced, in
the same held write that source settlement and the review recheck use: those
two judge content without changing it, and their statuses, stamps and findings
land only on a row that still holds the content they read, so a translation a
person rewrote meanwhile keeps the rewrite. The properties such work records on
a block, such as the source settlement stamp, are stored without changing the
block's stored context hash, so the producer's next push compares against the
hashes its own push stored and uploads only what changed.

Approving a source proposal is the approver's edit of the source through the
changes route's rules, guarded by the source the proposal was made against.
Three writers move what a row holds beside its content without a change set:
a review decision's status (applied as the `decide` operation lands), the
term candidate an entity's promotion adds, and a push, which applies a
producer's state. The first two hold the block's row while they write, and a
push holds the stream's write lock alone, so a change set's write waits for it
to commit. The
`write_overlay` automation action writes annotations and plugin overlays and
refuses a translation.

A push asserts the ref it last observed only for the governance it writes. It
asserts the decisions component when its records include a decision (a review
state, an established rung, a parked unit, an assignee or a note), and
the server refuses it with `409 governance_moved` when another decision has
landed since. Records that say only what was produced for a unit assert nothing
and merge by record time. The decisions component folds decisions alone, so the
records a server run writes while it drafts leave it where a client read it. The
worker makes the same assertion again inside the push's transaction, against the
ledger as it stood before the push wrote anything, so rows the push itself
changes never count against it.

A removal takes a file's content and leaves the decisions it holds standing. The
item row stays as the anchor those rows are keyed on, because the producer's
committed record still holds the decisions and a producer sends that record again
only when its fold moves: rows dropped here would have nothing to bring them
back. Content pushed to that path again lands on the same item, and the ledger
still holds those decisions. An item holding no decision is removed outright, and
the tree the server serves leaves out an item holding no blocks, so a producer is
never told about a file it does not have.

A server run begins by waiting for the project's pushes that are still queued or
being applied, up to a limit, and settles source only after them. A client
confirms a push for a few seconds before it may start a run, and a large push
applies for longer, so settlement reads what the push wrote rather than the
blocks the push is about to change or remove.

A run's event stream sends a comment frame every 15 seconds while a stage works
in silence, which is well inside the 60 seconds any connection may sit idle
between a client and this server. The comment carries no id and no data, so it
moves no resume point and no client renders it. A client whose connection drops
resumes from the last event id it received, through the `Last-Event-ID` header
or `?after=<seq>`, and the server replays the persisted events past that point.
A run itself belongs to the server: it goes on, and its events are recorded,
whether or not anyone is watching the stream.

A push applies under a write lock on its project stream, and so does removing a
file from the editor. The server's block writes on that stream, write-backs
among them, share the lock with one another. A block write waits for an
applying push to commit and a push waits for the block writes already in
progress, and both complete. A write-back that waited skips the blocks the push
removed or changed.

Approving a source proposal is refused when the block's source no longer
matches the source the proposal was made against, and the proposal stays open.
A block removed in the meantime stays removed, as it does for a rollback, a
batch revert or a flow run over the project's blocks.

The same routes exist under `/api/v1/projects/:id/sync/:ref/...` for a project
that has not yet been claimed into a workspace, authenticated by its claim token.
See [`kapi push`](/cli/commands/push) for the protocol as a client sees it.

### Connectors

Connector instances are workspace-scoped and require the manage-connectors permission. See [Connectors](/server/connectors) for setup guides.

```
GET    /api/v1/:ws/connectors              # List active connectors
POST   /api/v1/:ws/connectors              # Add connector
PUT    /api/v1/:ws/connectors/:id          # Update connector
DELETE /api/v1/:ws/connectors/:id          # Remove connector
GET    /api/v1/:ws/connectors/:id/status   # Sync status
GET    /api/v1/:ws/connectors/:id/content  # Browse available content
POST   /api/v1/:ws/connectors/:id/fetch    # Fetch content from the external system
POST   /api/v1/:ws/connectors/:id/publish  # Publish results back
```

### Flows and automation

```
GET    /api/v1/:ws/:id/flows               # List flows (built-in catalog + project flows)
POST   /api/v1/:ws/:id/flows               # Create a project flow
GET    /api/v1/:ws/:id/flows/:flowId       # Get one flow
PUT    /api/v1/:ws/:id/flows/:flowId       # Replace a project flow
DELETE /api/v1/:ws/:id/flows/:flowId       # Delete a project flow
       /api/v1/:ws/:id/automations         # Automation rules (see Automation)
```

Runs are started and observed through the convergence routes under a project;
the Runs view and `kapi up` are their clients. See
[Server-side flows](/server/flows) and [Automation](/server/automation).

### Review and delivery

```
POST /api/v1/:ws/:id/review/approve-passing   # Bulk-approve every block passing checks, checked terminology and the voice bar
POST /api/v1/:ws/:id/blocks/:ref/bulk-review  # One review decision on the translations of chosen blocks
GET  /api/v1/projects/:id/ship.json           # Public per-locale ship manifest
GET  /api/v1/:ws/audit-log/verify             # Verify the workspace audit chain
```

### Webhooks (inbound)

```
POST /api/webhooks/forge/:configID      # Repository push webhook (token mode)
POST /api/webhooks/github-app           # GitHub App webhook
```

### Running the Server

```bash
bin/bowrain-server --port 8080 --host 0.0.0.0 \
    --database-url postgres://bowrain:password@localhost/bowrain
```

The server requires PostgreSQL. See [Configuration](/server/configuration) for
the complete environment-variable and flag reference.

## gRPC API

The gRPC API provides streaming access. It is **multiplexed onto the same HTTP
port** as the REST API using h2c (cleartext HTTP/2): requests carrying
`Content-Type: application/grpc` are routed to the gRPC handler, everything else
to the REST handler. There is no separate gRPC port or TLS flag; the server
runs behind a TLS-terminating reverse proxy in production (see
[Self-Hosting](/server/self-hosting#reverse-proxy)), which routes `/neokapi.*`
to the server.

### Service Definition

The `NeokapiService` provides these RPCs:

```protobuf
service NeokapiService {
  rpc CreateProject(CreateProjectRequest) returns (ProjectResponse);
  rpc GetProject(GetProjectRequest) returns (ProjectResponse);
  rpc ListProjects(ListProjectsRequest) returns (ListProjectsResponse);
  rpc CreateVersion(CreateVersionRequest) returns (VersionResponse);
  rpc ListVersions(ListVersionsRequest) returns (ListVersionsResponse);
  rpc PullContent(PullContentRequest) returns (PullContentResponse);
  rpc PushContent(PushContentRequest) returns (PushContentResponse);
  rpc ExecuteFlow(ExecuteFlowRequest) returns (stream FlowProgressResponse);
  rpc Subscribe(SubscribeRequest) returns (stream EventResponse);
}
```

### Streaming

Two RPCs use server-side streaming:

- **ExecuteFlow**: streams progress updates during flow execution
- **Subscribe**: streams events matching the subscription filter

Block content does not travel this service. It moves over the canonical
`neokapi.content.v1` sync wire (`core/proto/sync/v1/sync.proto`), which
carries runs, overlays, segmentation and source-locale losslessly.

### Client Example

```go
// Production: connect through the TLS-terminating proxy (port 443).
conn, err := grpc.NewClient("bowrain.example.com:443",
    grpc.WithTransportCredentials(credentials.NewClientTLSFromCert(nil, "")),
)

// Local dev: the server speaks cleartext h2c on its HTTP port.
// conn, err := grpc.NewClient("localhost:8080",
//     grpc.WithTransportCredentials(insecure.NewCredentials()),
// )

client := serverv1.NewNeokapiServiceClient(conn)

// Stream flow-execution progress
stream, _ := client.ExecuteFlow(ctx, &serverv1.ExecuteFlowRequest{
    ProjectId:  "proj-1",
    FlowConfig: "name: qa\ntools:\n  - case-transform",
})
for {
    resp, err := stream.Recv()
    if err == io.EOF {
        break
    }
    fmt.Println(resp.Stage, resp.Message)
}
```

### Proto File Location

The proto definitions are at `bowrain/proto/v1/neokapi_service.proto`. Generate Go code with:

```bash
make proto
```
