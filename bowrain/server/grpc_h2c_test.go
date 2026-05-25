package server

import (
	"context"
	"errors"
	"net"
	"net/http"
	"testing"
	"time"

	pb "github.com/neokapi/neokapi/bowrain/proto/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// freeAddr returns a localhost address whose port is free at call time. There
// is an inherent (small) race between releasing the port and the server
// re-binding it, which the readiness polling in the test tolerates.
func freeAddr(t *testing.T) string {
	t.Helper()
	var lc net.ListenConfig
	lis, err := lc.Listen(t.Context(), "tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := lis.Addr().String()
	require.NoError(t, lis.Close())
	return addr
}

// TestStartMultiplexesGRPCAndHTTP exercises the real Server.Start path, which
// serves gRPC (cleartext HTTP/2 / h2c with prior knowledge) and HTTP/1.1 on the
// same port via the standard library's protocol negotiation. This is the
// integration coverage the previous golang.org/x/net/http2/h2c handler lacked.
func TestStartMultiplexesGRPCAndHTTP(t *testing.T) {
	srv := NewServer(DefaultConfig())
	initTestStores(t, srv)

	grpcSrv := grpc.NewServer()
	pb.RegisterNeokapiServiceServer(grpcSrv, NewGRPCServer(srv))
	srv.GRPCServer = grpcSrv

	addr := freeAddr(t)
	serveErr := make(chan error, 1)
	go func() { serveErr <- srv.Start(addr) }()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		require.NoError(t, srv.Shutdown(ctx))
		// Start returns http.ErrServerClosed on graceful shutdown.
		if err := <-serveErr; err != nil && !errors.Is(err, http.ErrServerClosed) {
			t.Errorf("Start returned unexpected error: %v", err)
		}
	})

	baseURL := "http://" + addr
	httpClient := &http.Client{Timeout: 2 * time.Second}

	// Wait for the listener to come up by polling the HTTP/1.1 health route,
	// which also asserts the non-gRPC branch of the multiplexer.
	require.Eventually(t, func() bool {
		req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, baseURL+"/api/v1/health", nil)
		if err != nil {
			return false
		}
		resp, err := httpClient.Do(req)
		if err != nil {
			return false
		}
		defer resp.Body.Close()
		return resp.StatusCode == http.StatusOK && resp.ProtoMajor == 1
	}, 10*time.Second, 50*time.Millisecond, "HTTP/1.1 health endpoint never became ready")

	// gRPC over cleartext (h2c, prior knowledge) on the same port must reach the
	// gRPC server, confirming Content-Type-based multiplexing still works.
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	client := pb.NewNeokapiServiceClient(conn)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	resp, err := client.CreateProject(ctx, &pb.CreateProjectRequest{
		Name:          "h2c multiplex",
		SourceLocale:  "en",
		TargetLocales: []string{"fr"},
	})
	require.NoError(t, err)
	assert.Equal(t, "h2c multiplex", resp.Name)
	assert.NotEmpty(t, resp.Id)
}
