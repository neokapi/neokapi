---
title: Server API
sidebar_position: 14
---

# Server API

Bowrain provides both REST and gRPC APIs for programmatic access to the platform.

## REST API

The REST API is built on Echo v4 and serves on the configured HTTP port.

### Endpoints

#### Health

```
GET /api/v1/health
```

#### Formats and Tools

```
GET /api/v1/formats
GET /api/v1/tools
```

#### Projects

```
POST   /api/v1/projects              # Create project
GET    /api/v1/projects              # List projects
GET    /api/v1/projects/:id          # Get project
PUT    /api/v1/projects/:id          # Update project
DELETE /api/v1/projects/:id          # Delete project
POST   /api/v1/projects/:id/blocks   # Store blocks
GET    /api/v1/projects/:id/blocks   # Get blocks
POST   /api/v1/projects/:id/versions # Create version
GET    /api/v1/projects/:id/versions # List versions
```

#### Connectors

Connector instances are workspace-scoped (`:ws` is the workspace slug) and require the manage-connectors permission. See [Connectors](/server/connectors) for setup guides.

```
GET    /api/v1/:ws/connectors              # List active connectors
POST   /api/v1/:ws/connectors              # Add connector
PUT    /api/v1/:ws/connectors/:id          # Update connector
DELETE /api/v1/:ws/connectors/:id          # Remove connector
GET    /api/v1/:ws/connectors/:id/status   # Sync status
GET    /api/v1/:ws/connectors/:id/content  # Browse available content
POST   /api/v1/:ws/connectors/:id/fetch    # Fetch content from the external system
POST   /api/v1/:ws/connectors/:id/publish  # Publish translations back
```

#### Processing

```
POST /api/v1/convert           # Convert between formats
POST /api/v1/translate         # Translate content
POST /api/v1/flow/execute      # Execute a flow
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
to the REST handler. There is no separate gRPC port or TLS flag — the server
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

- **ExecuteFlow**: Streams progress updates during flow execution
- **Subscribe**: Streams events matching the subscription filter

Block content does not travel this service. It moves over the canonical
`neokapi.content.v1` sync wire (`bowrain/core/proto/sync/v1/sync.proto`), which
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

The proto definitions are at `proto/v1/neokapi_service.proto`. Generate Go code with:

```bash
make proto
```
