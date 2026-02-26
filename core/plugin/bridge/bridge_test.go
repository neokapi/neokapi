package bridge

import (
	"context"
	"io"
	"log"
	"net"
	"testing"
	"time"

	pb "github.com/gokapi/gokapi/core/plugin/proto/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// mockBridgeServer implements the gRPC BridgeService for testing.
type mockBridgeServer struct {
	pb.UnimplementedBridgeServiceServer

	infoResp        *pb.InfoResponse
	openResp        *pb.OpenResponse
	readParts       []*pb.PartMessage
	writeOutput     []byte
	closeResp       *pb.CloseResponse
	listFiltersResp *pb.ListFiltersResponse
	infoErr         error
	openErr         error
	readErr         error
	writeErr        error
}

func (s *mockBridgeServer) Info(_ context.Context, _ *pb.InfoRequest) (*pb.InfoResponse, error) {
	if s.infoErr != nil {
		return nil, s.infoErr
	}
	return s.infoResp, nil
}

func (s *mockBridgeServer) Open(_ context.Context, _ *pb.OpenRequest) (*pb.OpenResponse, error) {
	if s.openErr != nil {
		return nil, s.openErr
	}
	return s.openResp, nil
}

func (s *mockBridgeServer) Read(_ *pb.ReadRequest, stream pb.BridgeService_ReadServer) error {
	if s.readErr != nil {
		return s.readErr
	}
	for _, part := range s.readParts {
		if err := stream.Send(part); err != nil {
			return err
		}
	}
	return nil
}

func (s *mockBridgeServer) Write(stream pb.BridgeService_WriteServer) error {
	if s.writeErr != nil {
		return s.writeErr
	}
	// Drain all chunks.
	for {
		_, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
	}
	return stream.SendAndClose(&pb.WriteResponse{Output: s.writeOutput})
}

func (s *mockBridgeServer) Close(_ context.Context, _ *pb.CloseRequest) (*pb.CloseResponse, error) {
	if s.closeResp != nil {
		return s.closeResp, nil
	}
	return &pb.CloseResponse{}, nil
}

func (s *mockBridgeServer) ListFilters(_ context.Context, _ *pb.ListFiltersRequest) (*pb.ListFiltersResponse, error) {
	return s.listFiltersResp, nil
}

func (s *mockBridgeServer) Shutdown(_ context.Context, _ *pb.ShutdownRequest) (*pb.ShutdownResponse, error) {
	return &pb.ShutdownResponse{}, nil
}

// startMockServer starts a gRPC server with the mock service on a random port.
// Returns the server, address, and a cleanup function.
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

	conn, err := grpc.NewClient("passthrough:///"+addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	b.conn = conn
	b.client = pb.NewBridgeServiceClient(conn)
	return b
}

func TestBridgeInfo(t *testing.T) {
	srv := &mockBridgeServer{
		infoResp: &pb.InfoResponse{
			Name:        "html",
			DisplayName: "HTML Filter",
			MimeTypes:   []string{"text/html"},
			Extensions:  []string{".html", ".htm"},
		},
	}
	b := newTestBridge(t, srv)

	info, err := b.Info("net.sf.okapi.filters.html.HtmlFilter")
	require.NoError(t, err)
	assert.Equal(t, "html", info.Name)
	assert.Equal(t, "HTML Filter", info.DisplayName)
	assert.Contains(t, info.Extensions, ".html")
}

func TestBridgeOpen(t *testing.T) {
	srv := &mockBridgeServer{
		openResp: &pb.OpenResponse{},
	}
	b := newTestBridge(t, srv)

	err := b.Open(OpenParams{
		FilterClass:  "net.sf.okapi.filters.html.HtmlFilter",
		URI:          "test.html",
		SourceLocale: "en",
		Encoding:     "UTF-8",
		Content:      []byte("<html></html>"),
		MimeType:     "text/html",
	})
	require.NoError(t, err)
}

func TestBridgeRead(t *testing.T) {
	srv := &mockBridgeServer{
		readParts: []*pb.PartMessage{
			{PartType: 0, Layer: &pb.LayerMessage{Id: "doc1", Name: "test.html", Format: "html"}},
			{PartType: 4, Block: &pb.BlockMessage{Id: "tu1", Translatable: true, Source: []*pb.SegmentMessage{{Id: "s1", Content: &pb.FragmentMessage{CodedText: "Hello"}}}}},
			{PartType: 1, Layer: &pb.LayerMessage{Id: "doc1"}},
		},
	}
	b := newTestBridge(t, srv)

	parts, err := b.Read()
	require.NoError(t, err)
	require.Len(t, parts, 3)
	assert.Equal(t, int32(0), parts[0].PartType)
	assert.Equal(t, "doc1", parts[0].Layer.Id)
}

func TestBridgeWrite(t *testing.T) {
	srv := &mockBridgeServer{
		writeOutput: []byte("translated"),
	}
	b := newTestBridge(t, srv)

	output, err := b.Write(WriteParams{
		FilterClass:     "net.sf.okapi.filters.html.HtmlFilter",
		Parts:           []*pb.PartMessage{{PartType: 0}},
		Locale:          "fr",
		Encoding:        "UTF-8",
		OriginalContent: []byte("<html></html>"),
	})
	require.NoError(t, err)
	assert.Equal(t, []byte("translated"), output)
}

func TestBridgeCloseFilter(t *testing.T) {
	srv := &mockBridgeServer{
		closeResp: &pb.CloseResponse{},
	}
	b := newTestBridge(t, srv)

	err := b.CloseFilter()
	require.NoError(t, err)
}

func TestBridgeListFilters(t *testing.T) {
	srv := &mockBridgeServer{
		listFiltersResp: &pb.ListFiltersResponse{
			Filters: []*pb.FilterEntry{
				{FilterClass: "net.sf.okapi.filters.html.HtmlFilter", Name: "html", DisplayName: "HTML"},
			},
		},
	}
	b := newTestBridge(t, srv)

	lf, err := b.ListFilters()
	require.NoError(t, err)
	require.Len(t, lf.Filters, 1)
	assert.Equal(t, "html", lf.Filters[0].Name)
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
