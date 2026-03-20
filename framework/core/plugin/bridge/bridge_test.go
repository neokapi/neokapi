package bridge

import (
	"context"
	"io"
	"log"
	"net"
	"testing"
	"time"

	pb "github.com/neokapi/neokapi/core/plugin/proto/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// mockBridgeServer implements the gRPC BridgeService for testing.
type mockBridgeServer struct {
	pb.UnimplementedBridgeServiceServer

	// Process fields
	processReadParts  []*pb.PartMessage
	processOutput     []byte
	processOutputPath string
	processErr        string
}

func (s *mockBridgeServer) Process(stream pb.BridgeService_ProcessServer) error {
	// Receive header.
	req, err := stream.Recv()
	if err != nil {
		return err
	}
	_ = req.GetHeader() // consume header

	// Send read-phase parts.
	for _, part := range s.processReadParts {
		if err := stream.Send(&pb.ProcessResponse{
			Response: &pb.ProcessResponse_Part{Part: part},
		}); err != nil {
			return err
		}
	}

	// Send ReadDone.
	if err := stream.Send(&pb.ProcessResponse{
		Response: &pb.ProcessResponse_ReadDone{ReadDone: &pb.ProcessReadDone{}},
	}); err != nil {
		return err
	}

	// Drain processed parts from client until CloseSend.
	for {
		_, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
	}

	// Send Complete.
	complete := &pb.ProcessComplete{
		Output:     s.processOutput,
		OutputPath: s.processOutputPath,
		Error:      s.processErr,
	}
	return stream.Send(&pb.ProcessResponse{
		Response: &pb.ProcessResponse_Complete{Complete: complete},
	})
}

func (s *mockBridgeServer) Shutdown(_ context.Context, _ *pb.ShutdownRequest) (*pb.ShutdownResponse, error) {
	return &pb.ShutdownResponse{}, nil
}

// startMockServer starts a gRPC server with the mock service on a random port.
func startMockServer(t *testing.T, srv *mockBridgeServer) (string, func()) {
	t.Helper()
	lis, err := net.Listen("tcp", "localhost:0")
	require.NoError(t, err)

	s := grpc.NewServer()
	pb.RegisterBridgeServiceServer(s, srv)

	go func() { _ = s.Serve(lis) }()

	return lis.Addr().String(), func() {
		s.GracefulStop()
	}
}

// newTestBridge creates a JavaBridge connected to a mock gRPC server.
func newTestBridge(t *testing.T, srv *mockBridgeServer) *JavaBridge {
	t.Helper()
	addr, cleanup := startMockServer(t, srv)
	t.Cleanup(cleanup)

	b := &JavaBridge{
		cfg: BridgeConfig{
			CommandTimeout: 5 * time.Second,
			StartupTimeout: 5 * time.Second,
		},
		logger:  log.New(io.Discard, "", 0),
		running: true,
	}

	conn, err := grpc.NewClient(grpcTarget(addr),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	b.conn = conn
	b.client = pb.NewBridgeServiceClient(conn)
	return b
}

func TestConfigWithDefaults(t *testing.T) {
	cfg := BridgeConfig{}
	cfg = cfg.withDefaults()
	assert.Equal(t, "java", cfg.Command)
	assert.Equal(t, DefaultStartupTimeout, cfg.StartupTimeout)
	assert.Equal(t, DefaultCommandTimeout, cfg.CommandTimeout)
}

func TestConfigPreservesValues(t *testing.T) {
	cfg := BridgeConfig{
		Command:        "/usr/local/bin/java",
		StartupTimeout: 10 * time.Second,
		CommandTimeout: 30 * time.Second,
	}
	cfg = cfg.withDefaults()
	assert.Equal(t, "/usr/local/bin/java", cfg.Command)
	assert.Equal(t, 10*time.Second, cfg.StartupTimeout)
	assert.Equal(t, 30*time.Second, cfg.CommandTimeout)
}
