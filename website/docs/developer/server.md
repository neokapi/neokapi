---
title: Server API
sidebar_position: 14
---

# Server API

gokapi provides both REST and gRPC APIs for programmatic access to the platform.

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
bin/gokapi-server --port 8080 --host 0.0.0.0
```

Environment variables: `GOKAPI_PORT`, `GOKAPI_HOST`, `GOKAPI_DATA_DIR`.

## gRPC API

The gRPC API provides streaming access and is available on a separate port.

### Starting with gRPC

```bash
bin/gokapi-server --port 8080 --grpc-port 9090
```

Or via environment: `GOKAPI_GRPC_PORT=9090`.

### Service Definition

The `GokapiService` provides these RPCs:

```protobuf
service GokapiService {
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
conn, err := grpc.NewClient("localhost:9090",
    grpc.WithTransportCredentials(insecure.NewCredentials()),
)
client := serverv1.NewGokapiServiceClient(conn)

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

The proto definitions are at `proto/v1/gokapi_service.proto`. Generate Go code with:

```bash
make proto
```
