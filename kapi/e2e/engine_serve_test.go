//go:build e2e

package e2e

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	enginev1 "github.com/neokapi/neokapi/core/proto/engine/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// TestEngineServe_HandshakeAndListFormats exercises the socket mode end to
// end: spawn `kapi engine serve`, parse the one-line JSON handshake, dial the
// Unix socket, and issue a unary RPC. The full extract → process → merge
// round trip is locked by the bufconn tests (cli/engineserve) and the
// Python/Node example clients (make engine-examples).
func TestEngineServe_HandshakeAndListFormats(t *testing.T) {
	// Unix socket paths cap at ~104 bytes; t.TempDir() (which embeds the
	// long test name) can exceed it, so use a plain short temp dir.
	sockDir, err := os.MkdirTemp("", "kapi-e2e")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(sockDir) })
	socket := filepath.Join(sockDir, "engine.sock")

	cmd := exec.Command(kapiBin, "engine", "serve", "--socket", socket)
	cmd.Env = append(os.Environ(), isoEnv...)
	stdout, err := cmd.StdoutPipe()
	require.NoError(t, err)
	require.NoError(t, cmd.Start())
	t.Cleanup(func() {
		_ = cmd.Process.Signal(os.Interrupt)
		_, _ = cmd.Process.Wait()
	})

	// First stdout line is the JSON handshake (plugin daemon convention).
	line, err := bufio.NewReader(stdout).ReadString('\n')
	require.NoError(t, err, "expected a handshake line on stdout")
	var hs struct {
		Socket  string `json:"socket"`
		Version string `json:"version"`
		PID     int    `json:"pid"`
	}
	require.NoError(t, json.Unmarshal([]byte(line), &hs), "handshake must be one-line JSON: %q", line)
	assert.Equal(t, socket, hs.Socket)
	assert.NotEmpty(t, hs.Version)
	assert.Equal(t, cmd.Process.Pid, hs.PID)

	conn, err := grpc.NewClient("unix://"+hs.Socket,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithContextDialer(func(ctx context.Context, addr string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, "unix", hs.Socket)
		}),
	)
	require.NoError(t, err)
	defer conn.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	client := enginev1.NewEngineServiceClient(conn)

	formats, err := client.ListFormats(ctx, &enginev1.ListFormatsRequest{})
	require.NoError(t, err)
	assert.NotEmpty(t, formats.GetFormats(), "the served engine must expose the built-in formats")

	resp, err := client.Detect(ctx, &enginev1.DetectRequest{Name: "messages.json"})
	require.NoError(t, err)
	assert.NotEmpty(t, resp.GetFormat())
}

// pipeAddr is the placeholder address for the client-side stdio conn.
type pipeAddr struct{}

func (pipeAddr) Network() string { return "stdio" }
func (pipeAddr) String() string  { return "stdio" }

// pipeConn adapts the spawned process's stdout/stdin pipes into the
// net.Conn a grpc-go custom dialer must return — the caller-side half of
// the --stdio transport. Close half-closes: it closes the child's stdin
// (the server's EOF signal) and lets process exit close the read side.
type pipeConn struct {
	in  io.Reader
	out io.WriteCloser
}

func (c *pipeConn) Read(p []byte) (int, error)       { return c.in.Read(p) }
func (c *pipeConn) Write(p []byte) (int, error)      { return c.out.Write(p) }
func (c *pipeConn) Close() error                     { return c.out.Close() }
func (c *pipeConn) LocalAddr() net.Addr              { return pipeAddr{} }
func (c *pipeConn) RemoteAddr() net.Addr             { return pipeAddr{} }
func (c *pipeConn) SetDeadline(time.Time) error      { return nil }
func (c *pipeConn) SetReadDeadline(time.Time) error  { return nil }
func (c *pipeConn) SetWriteDeadline(time.Time) error { return nil }

// TestEngineServe_Stdio exercises the --stdio transport end to end: spawn
// `kapi engine serve --stdio`, read the handshake from STDERR (stdout is
// exclusively the gRPC stream), dial with a custom dialer over the
// process's pipes, round-trip RPCs, then close stdin and verify the server
// exits cleanly on EOF.
func TestEngineServe_Stdio(t *testing.T) {
	cmd := exec.Command(kapiBin, "engine", "serve", "--stdio")
	cmd.Env = append(os.Environ(), isoEnv...)
	stdin, err := cmd.StdinPipe()
	require.NoError(t, err)
	stdout, err := cmd.StdoutPipe()
	require.NoError(t, err)
	stderr, err := cmd.StderrPipe()
	require.NoError(t, err)
	require.NoError(t, cmd.Start())
	exitErr := make(chan error, 1)
	exited := make(chan struct{})
	go func() {
		exitErr <- cmd.Wait()
		close(exited)
	}()
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		select {
		case <-exited:
		case <-time.After(10 * time.Second):
		}
	})

	// The handshake arrives on stderr in stdio mode.
	line, err := bufio.NewReader(stderr).ReadString('\n')
	require.NoError(t, err, "expected a handshake line on stderr")
	var hs struct {
		Transport string `json:"transport"`
		Version   string `json:"version"`
		PID       int    `json:"pid"`
	}
	require.NoError(t, json.Unmarshal([]byte(line), &hs), "handshake must be one-line JSON: %q", line)
	assert.Equal(t, "stdio", hs.Transport)
	assert.NotEmpty(t, hs.Version)
	assert.Equal(t, cmd.Process.Pid, hs.PID)

	// One connection is all the transport carries: the dialer hands out the
	// pipe pair exactly once.
	pc := &pipeConn{in: stdout, out: stdin}
	var dialed atomic.Bool
	conn, err := grpc.NewClient("passthrough:///stdio",
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
			if !dialed.CompareAndSwap(false, true) {
				return nil, errors.New("stdio transport carries a single connection")
			}
			return pc, nil
		}),
	)
	require.NoError(t, err)
	defer conn.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	client := enginev1.NewEngineServiceClient(conn)

	formats, err := client.ListFormats(ctx, &enginev1.ListFormatsRequest{})
	require.NoError(t, err)
	assert.NotEmpty(t, formats.GetFormats(), "the served engine must expose the built-in formats")

	resp, err := client.Detect(ctx, &enginev1.DetectRequest{Name: "messages.json"})
	require.NoError(t, err)
	assert.NotEmpty(t, resp.GetFormat())

	// Closing the child's stdin is the shutdown signal: the server sees
	// EOF, the serve loop ends, and the process exits with status 0.
	require.NoError(t, stdin.Close())
	select {
	case err := <-exitErr:
		require.NoError(t, err, "the server must exit cleanly on stdin EOF")
	case <-time.After(15 * time.Second):
		t.Fatal("the server did not exit after stdin EOF")
	}
}
