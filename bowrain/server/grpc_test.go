package server

import (
	"context"
	"errors"
	"io"
	"net"
	"testing"

	pb "github.com/neokapi/neokapi/bowrain/proto/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
)

const bufSize = 1024 * 1024

func setupGRPC(t *testing.T) pb.NeokapiServiceClient {
	t.Helper()

	cfg := DefaultConfig()

	srv := NewServer(cfg)
	initTestStores(t, srv)

	lis := bufconn.Listen(bufSize)
	grpcSrv := grpc.NewServer()
	pb.RegisterNeokapiServiceServer(grpcSrv, NewGRPCServer(srv))

	go func() {
		if err := grpcSrv.Serve(lis); err != nil {
			t.Logf("gRPC server error: %v", err)
		}
	}()
	t.Cleanup(func() { grpcSrv.GracefulStop() })

	conn, err := grpc.NewClient("passthrough://bufconn",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return lis.DialContext(ctx)
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	require.NoError(t, err)
	t.Cleanup(func() { conn.Close() })

	return pb.NewNeokapiServiceClient(conn)
}

func TestGRPCProjectCRUD(t *testing.T) {
	client := setupGRPC(t)
	ctx := t.Context()

	// Create project.
	resp, err := client.CreateProject(ctx, &pb.CreateProjectRequest{
		Name:          "Test Project",
		SourceLocale:  "en",
		TargetLocales: []string{"fr", "de"},
	})
	require.NoError(t, err)
	assert.Equal(t, "Test Project", resp.Name)
	assert.NotEmpty(t, resp.Id)

	projectID := resp.Id

	// Get project.
	proj, err := client.GetProject(ctx, &pb.GetProjectRequest{ProjectId: projectID})
	require.NoError(t, err)
	assert.Equal(t, "Test Project", proj.Name)

	// List projects.
	list, err := client.ListProjects(ctx, &pb.ListProjectsRequest{})
	require.NoError(t, err)
	assert.Len(t, list.Projects, 1)
}

func TestGRPCBlocksAndVersions(t *testing.T) {
	client := setupGRPC(t)
	ctx := t.Context()

	// Create project first.
	proj, err := client.CreateProject(ctx, &pb.CreateProjectRequest{
		Name:         "Block Test",
		SourceLocale: "en",
	})
	require.NoError(t, err)
	projectID := proj.Id

	// Store blocks.
	storeResp, err := client.StoreBlocks(ctx, &pb.StoreBlocksRequest{
		ProjectId: projectID,
		Blocks: []*pb.BlockMessage{
			{Id: "b1", Source: "Hello", Targets: map[string]string{"fr": "Bonjour"}},
			{Id: "b2", Source: "World"},
		},
	})
	require.NoError(t, err)
	assert.Equal(t, int32(2), storeResp.StoredCount)

	// Stream blocks.
	stream, err := client.StreamBlocks(ctx, &pb.StreamBlocksRequest{ProjectId: projectID})
	require.NoError(t, err)

	var blocks []*pb.BlockResponse
	for {
		resp, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		require.NoError(t, err)
		blocks = append(blocks, resp)
	}
	assert.Len(t, blocks, 2)

	// Create version.
	ver, err := client.CreateVersion(ctx, &pb.CreateVersionRequest{
		ProjectId:   projectID,
		Label:       "v1.0",
		Description: "initial",
	})
	require.NoError(t, err)
	assert.Equal(t, "v1.0", ver.Label)
	assert.Equal(t, int32(2), ver.BlockCount)

	// List versions.
	versions, err := client.ListVersions(ctx, &pb.ListVersionsRequest{ProjectId: projectID})
	require.NoError(t, err)
	assert.Len(t, versions.Versions, 1)
}

func TestGRPCFlowExecution(t *testing.T) {
	client := setupGRPC(t)
	ctx := t.Context()

	// Create a project with blocks first.
	proj, err := client.CreateProject(ctx, &pb.CreateProjectRequest{
		Name:         "Flow Test",
		SourceLocale: "en",
	})
	require.NoError(t, err)

	_, err = client.StoreBlocks(ctx, &pb.StoreBlocksRequest{
		ProjectId: proj.Id,
		Blocks: []*pb.BlockMessage{
			{Id: "b1", Source: "Hello world"},
		},
	})
	require.NoError(t, err)

	// Execute a flow with word-count tool.
	stream, err := client.ExecuteFlow(ctx, &pb.ExecuteFlowRequest{
		FlowConfig: "name: test-flow\ntools:\n  - word-count",
		ProjectId:  proj.Id,
	})
	require.NoError(t, err)

	// Collect all progress messages.
	var messages []*pb.FlowProgressResponse
	for {
		resp, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		require.NoError(t, err)
		messages = append(messages, resp)
	}

	require.GreaterOrEqual(t, len(messages), 2, "expected at least setup + complete messages")
	assert.Equal(t, "setup", messages[0].Stage)

	last := messages[len(messages)-1]
	assert.True(t, last.Done)
	assert.Equal(t, "complete", last.Stage)
}

func TestGRPCFlowExecutionInvalidConfig(t *testing.T) {
	client := setupGRPC(t)
	ctx := t.Context()

	// Empty tools should return an error.
	stream, err := client.ExecuteFlow(ctx, &pb.ExecuteFlowRequest{
		FlowConfig: "tools: []",
	})
	require.NoError(t, err)

	_, err = stream.Recv()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "at least one tool")
}
