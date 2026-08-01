package server

import (
	"context"
	"errors"
	"io"
	"net"
	"testing"

	pb "github.com/neokapi/neokapi/bowrain/proto/v1"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/core/tool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

const bufSize = 1024 * 1024

func setupGRPC(t *testing.T) pb.NeokapiServiceClient {
	t.Helper()
	client, _ := setupGRPCWithServer(t)
	return client
}

// setupGRPCWithServer is setupGRPC for tests that need to reach the server
// itself — registering a tool, say — rather than only its client.
func setupGRPCWithServer(t *testing.T) (pb.NeokapiServiceClient, *Server) {
	t.Helper()

	cfg := DefaultConfig()

	srv := shutdownOnCleanup(t, NewServer(cfg))
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

	return pb.NewNeokapiServiceClient(conn), srv
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

	// Execute a flow with the case-transform tool.
	stream, err := client.ExecuteFlow(ctx, &pb.ExecuteFlowRequest{
		FlowConfig: "name: test-flow\ntools:\n  - case-transform",
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

// failingTool drains its input and then reports a failure, which is what a tool
// that cannot finish its work does. Draining first keeps the stage above it
// from blocking, so the test exercises the reporting rather than a deadlock.
type failingTool struct {
	name string
	err  error
	// panicWith, when non-nil, panics instead of returning an error.
	panicWith any
}

func (f *failingTool) Name() string        { return f.name }
func (f *failingTool) Description() string { return "fails on purpose" }
func (f *failingTool) Process(_ context.Context, in <-chan *model.Part, _ chan<- *model.Part) error {
	for range in { //nolint:revive // draining is the point
	}
	if f.panicWith != nil {
		panic(f.panicWith)
	}
	return f.err
}
func (f *failingTool) Config() tool.ToolConfig         { return nil }
func (f *failingTool) SetConfig(tool.ToolConfig) error { return nil }

// A tool that fails mid-flow must fail the RPC. Before this was fixed the tool
// error was discarded, the stage below read the closed channel as a clean end
// of stream, and the handler sent Stage "complete" with Done true — so a flow
// that processed nothing reported success, and no field in FlowProgressResponse
// could tell a client otherwise.
func TestGRPCFlowExecutionReportsToolFailure(t *testing.T) {
	for _, tc := range []struct {
		name string
		tool *failingTool
		want string
	}{
		{
			name: "tool returns an error",
			tool: &failingTool{name: "boom", err: errors.New("upstream refused the batch")},
			want: "boom",
		},
		{
			name: "tool panics",
			tool: &failingTool{name: "kaboom", panicWith: "nil map write"},
			want: "kaboom",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client, srv := setupGRPCWithServer(t)
			ctx := t.Context()

			srv.ToolRegistry.Register(registry.ToolID(tc.tool.name), func() tool.Tool { return tc.tool })

			proj, err := client.CreateProject(ctx, &pb.CreateProjectRequest{
				Name: "Failing Flow", SourceLocale: "en",
			})
			require.NoError(t, err)
			_, err = client.StoreBlocks(ctx, &pb.StoreBlocksRequest{
				ProjectId: proj.Id,
				Blocks:    []*pb.BlockMessage{{Id: "b1", Source: "Hello world"}},
			})
			require.NoError(t, err)

			stream, err := client.ExecuteFlow(ctx, &pb.ExecuteFlowRequest{
				FlowConfig: "name: failing-flow\ntools:\n  - " + tc.tool.name,
				ProjectId:  proj.Id,
			})
			require.NoError(t, err)

			var last *pb.FlowProgressResponse
			var recvErr error
			for {
				resp, rerr := stream.Recv()
				if rerr != nil {
					recvErr = rerr
					break
				}
				last = resp
			}

			require.Error(t, recvErr, "the RPC must fail, not end cleanly")
			require.NotErrorIs(t, recvErr, io.EOF,
				"a clean EOF is exactly how the discarded error used to look")
			assert.Equal(t, codes.Internal, status.Code(recvErr))
			assert.Contains(t, status.Convert(recvErr).Message(), tc.want,
				"the failing tool is named so the operator knows where to look")

			if last != nil {
				assert.False(t, last.Done, "a failed flow must never report Done")
				assert.NotEqual(t, "complete", last.Stage)
			}
		})
	}
}

// The success path still reports the counts, and now reports them as fields
// rather than only inside the prose message.
func TestGRPCFlowExecutionReportsCounts(t *testing.T) {
	client := setupGRPC(t)
	ctx := t.Context()

	proj, err := client.CreateProject(ctx, &pb.CreateProjectRequest{
		Name: "Counting Flow", SourceLocale: "en",
	})
	require.NoError(t, err)
	_, err = client.StoreBlocks(ctx, &pb.StoreBlocksRequest{
		ProjectId: proj.Id,
		Blocks: []*pb.BlockMessage{
			{Id: "b1", Source: "one"},
			{Id: "b2", Source: "two"},
		},
	})
	require.NoError(t, err)

	stream, err := client.ExecuteFlow(ctx, &pb.ExecuteFlowRequest{
		FlowConfig: "name: counting\ntools:\n  - case-transform",
		ProjectId:  proj.Id,
	})
	require.NoError(t, err)

	var last *pb.FlowProgressResponse
	for {
		resp, rerr := stream.Recv()
		if errors.Is(rerr, io.EOF) {
			break
		}
		require.NoError(t, rerr)
		last = resp
	}
	require.NotNil(t, last)
	assert.True(t, last.Done)
	assert.Equal(t, int32(2), last.Total, "total was declared in the proto and never set")
	assert.Equal(t, int32(2), last.Processed)
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
