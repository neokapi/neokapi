---
title: Server API
sidebar_position: 14
---

# Server API

neokapi provides both REST and gRPC APIs for programmatic access to the platform.

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

```
GET    /api/v1/connectors/types      # List connector types
GET    /api/v1/connectors            # List active connectors
POST   /api/v1/connectors            # Add connector
DELETE /api/v1/connectors/:id        # Remove connector
GET    /api/v1/connectors/:id/status # Sync status
POST   /api/v1/pull                  # Pull from connector
POST   /api/v1/push                  # Push to connector
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
  rpc StoreBlocks(StoreBlocksRequest) returns (StoreBlocksResponse);
  rpc StreamBlocks(StreamBlocksRequest) returns (stream BlockResponse);
  rpc CreateVersion(CreateVersionRequest) returns (VersionResponse);
  rpc ListVersions(ListVersionsRequest) returns (ListVersionsResponse);
  rpc PullContent(PullContentRequest) returns (PullContentResponse);
  rpc PushContent(PushContentRequest) returns (PushContentResponse);
  rpc ExecuteFlow(ExecuteFlowRequest) returns (stream FlowProgressResponse);
  rpc Subscribe(SubscribeRequest) returns (stream EventResponse);
}
```

### Streaming

Three RPCs use server-side streaming:

- **StreamBlocks**: Streams all blocks matching a query
- **ExecuteFlow**: Streams progress updates during flow execution
- **Subscribe**: Streams events matching the subscription filter

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

// Stream blocks
stream, _ := client.StreamBlocks(ctx, &serverv1.StreamBlocksRequest{
    ProjectId: "proj-1",
})
for {
    resp, err := stream.Recv()
    if err == io.EOF {
        break
    }
    fmt.Println(resp.Block.Source)
}
```

### Proto File Location

The proto definitions are at `proto/v1/neokapi_service.proto`. Generate Go code with:

```bash
make proto
```
