//go:build !js

package host

import (
	"context"
	"errors"
	"io"
	"net"
	"testing"
	"time"

	neokapiconfig "github.com/neokapi/neokapi/core/config"
	"github.com/neokapi/neokapi/core/format/schema"
	"github.com/neokapi/neokapi/core/formats"
	enginev1 "github.com/neokapi/neokapi/core/proto/engine/v1"
	"github.com/neokapi/neokapi/core/registry"
	libtools "github.com/neokapi/neokapi/core/tools"
	"github.com/neokapi/neokapi/host/engineserve"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// pipePair builds the two net.Conn ends of a simulated stdio session:
// what the server sees (its stdin/stdout) and what the client sees (the
// child process's stdout/stdin pipes).
func pipePair() (server, client *stdioConn) {
	clientToServer, serverIn := io.Pipe()  // client writes → server stdin
	serverToClient, serverOut := io.Pipe() // server stdout → client reads
	server = newStdioConn(clientToServer, serverOut)
	client = newStdioConn(serverToClient, serverIn)
	return server, client
}

func TestStdioConnDeadlinesAreNoOps(t *testing.T) {
	conn, _ := pipePair()
	// grpc-go arms a deadline around the connection handshake; the stdio
	// conn must accept (and may ignore) it rather than erroring.
	assert.NoError(t, conn.SetDeadline(time.Now().Add(time.Second)))
	assert.NoError(t, conn.SetReadDeadline(time.Now().Add(time.Second)))
	assert.NoError(t, conn.SetWriteDeadline(time.Time{}))
	assert.Equal(t, "stdio", conn.LocalAddr().Network())
	assert.Equal(t, "stdio", conn.RemoteAddr().String())
}

func TestStdioConnReadWrite(t *testing.T) {
	server, client := pipePair()
	go func() {
		_, err := client.Write([]byte("ping"))
		assert.NoError(t, err)
	}()
	buf := make([]byte, 4)
	_, err := io.ReadFull(server, buf)
	require.NoError(t, err)
	assert.Equal(t, "ping", string(buf))

	go func() {
		_, err := server.Write([]byte("pong"))
		assert.NoError(t, err)
	}()
	_, err = io.ReadFull(client, buf)
	require.NoError(t, err)
	assert.Equal(t, "pong", string(buf))
}

func TestStdioConnEOFAndClose(t *testing.T) {
	server, client := pipePair()

	// The peer hanging up (client closes → server stdin EOF).
	require.NoError(t, client.Close())
	_, err := server.Read(make([]byte, 1))
	require.Error(t, err, "read after the peer closed must fail")

	// Close is idempotent and signals the closed channel exactly once.
	require.NoError(t, server.Close())
	assert.NoError(t, server.Close())
	select {
	case <-server.closed:
	default:
		t.Fatal("Close must signal the closed channel")
	}
	_, err = server.Read(make([]byte, 1))
	require.Error(t, err)
	_, err = server.Write([]byte("x"))
	require.Error(t, err)
}

func TestStdioListenerAcceptsExactlyOnce(t *testing.T) {
	serverConn, _ := pipePair()
	lis := &stdioListener{conn: serverConn, closedCh: make(chan struct{})}

	conn, err := lis.Accept()
	require.NoError(t, err)
	assert.Same(t, serverConn, conn)

	// The second Accept blocks until the connection closes, then reports
	// net.ErrClosed — that is what ends grpc.Server.Serve on stdin EOF.
	acceptErr := make(chan error, 1)
	go func() {
		_, err := lis.Accept()
		acceptErr <- err
	}()
	select {
	case err := <-acceptErr:
		t.Fatalf("Accept must block while the connection lives, got %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	require.NoError(t, conn.Close())
	select {
	case err := <-acceptErr:
		require.ErrorIs(t, err, net.ErrClosed)
	case <-time.After(5 * time.Second):
		t.Fatal("Accept did not unblock after the connection closed")
	}
}

func TestStdioListenerCloseUnblocksAccept(t *testing.T) {
	serverConn, _ := pipePair()
	lis := &stdioListener{conn: serverConn, closedCh: make(chan struct{})}
	_, err := lis.Accept()
	require.NoError(t, err)

	acceptErr := make(chan error, 1)
	go func() {
		_, err := lis.Accept()
		acceptErr <- err
	}()
	require.NoError(t, lis.Close())
	assert.NoError(t, lis.Close(), "Close must be idempotent")
	select {
	case err := <-acceptErr:
		require.ErrorIs(t, err, net.ErrClosed)
	case <-time.After(5 * time.Second):
		t.Fatal("Accept did not unblock after the listener closed")
	}
}

// TestStdioGRPCRoundTrip proves the transport carries real gRPC: the
// EngineService served over a one-shot stdio listener, dialed by a client
// whose custom dialer returns the other pipe end, and shut down cleanly by
// the client hanging up (the stdio equivalent of stdin EOF).
func TestStdioGRPCRoundTrip(t *testing.T) {
	serverConn, clientConn := pipePair()
	lis := &stdioListener{conn: serverConn, closedCh: make(chan struct{})}

	formatReg := registry.NewFormatRegistry()
	schemaReg := schema.NewSchemaRegistry()
	formats.RegisterAll(formatReg, formats.RegisterOptions{
		SchemaReg: schemaReg,
		ConfigReg: neokapiconfig.DefaultRegistry,
	})
	toolReg := registry.NewToolRegistry()
	libtools.RegisterAll(toolReg)

	gs := grpc.NewServer()
	enginev1.RegisterEngineServiceServer(gs, engineserve.New(formatReg, toolReg))
	serveErr := make(chan error, 1)
	go func() { serveErr <- gs.Serve(lis) }()
	t.Cleanup(gs.Stop)

	conn, err := grpc.NewClient("passthrough:///stdio",
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
			return clientConn, nil
		}),
	)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	client := enginev1.NewEngineServiceClient(conn)
	resp, err := client.ListFormats(ctx, &enginev1.ListFormatsRequest{})
	require.NoError(t, err)
	assert.NotEmpty(t, resp.GetFormats())

	// Hang up: the connection closes, the one-shot listener reports
	// net.ErrClosed, and Serve returns — the clean stdio shutdown path.
	require.NoError(t, conn.Close())
	select {
	case err := <-serveErr:
		if err != nil && !errors.Is(err, grpc.ErrServerStopped) {
			require.ErrorIs(t, err, net.ErrClosed)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("grpc.Server.Serve did not return after the connection closed")
	}
}
