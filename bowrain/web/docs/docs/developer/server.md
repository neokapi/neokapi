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

A pull walks the stream's change log forward from the client's cursor and
serves each changed block under the item it belongs to. A block row that
belongs to no item is left out of the response, and the cursor the response
returns still moves past its change, so every client continues from the same
place. The server logs how many such rows a pull left out.

Server work that reads blocks and later writes them back, such as source
settlement, translation and extraction jobs, pseudo-translation and the
editor's bulk actions, writes each block only to the row it read, and only
while that row still holds the content hash it read. A block that a push
removed or rewrote in the meantime keeps what the push left: a removed item
stays removed, and a pushed source is never replaced by the wording the job
read. The properties such work records on a block, such as the source
settlement stamp, are stored without changing the block's stored context hash,
so the producer's next push compares against the hashes its own push stored and
uploads only what changed.

A push asserts the ref it last observed only for the governance it writes. It
asserts the decisions component when its records include a decision (a review
state, a reviewed or signed-off rung, a parked unit, an assignee or a note), and
the server refuses it with `409 governance_moved` when another decision has
landed since. Records that say only what was produced for a unit assert nothing
and merge by record time. The decisions component folds decisions alone, so the
records a server run writes while it drafts leave it where a client read it. The
worker makes the same assertion again inside the push's transaction, against the
ledger as it stood before the push wrote anything, so rows the push itself
removes, such as the decisions of an item its declared tree no longer holds,
never count against it.

A server run begins by waiting for the project's pushes that are still queued or
being applied, up to a limit, and settles source only after them. A client
confirms a push for a few seconds before it may start a run, and a large push
applies for longer, so settlement reads what the push wrote rather than the
blocks the push is about to change or remove.

A push applies under a write lock on its project stream, and so does removing a
file from the editor. The server's block writes on that stream, write-backs
among them, share the lock with one another. A block write waits for an
applying push to commit and a push waits for the block writes already in
progress, and both complete. A write-back that waited skips the blocks the push
removed or changed.

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
