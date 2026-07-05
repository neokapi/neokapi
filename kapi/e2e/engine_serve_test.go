//go:build e2e

package e2e

import (
	"bufio"
	"context"
	"encoding/json"
	"net"
	"os"
	"os/exec"
	"path/filepath"
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
