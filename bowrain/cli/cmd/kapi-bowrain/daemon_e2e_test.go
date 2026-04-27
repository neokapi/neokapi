package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"syscall"
	"testing"
	"time"

	"github.com/neokapi/neokapi/cli/pluginhost"
	"github.com/neokapi/neokapi/core/plugin/manifest"
	pb "github.com/neokapi/neokapi/core/plugin/proto/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDaemon_E2EHealth builds the kapi-bowrain binary, spawns it as a
// Mode-C daemon via the DaemonPool, then calls the DaemonControlService.
// Health RPC. Verifies the full Mode-C handshake + gRPC plumbing.
func TestDaemon_E2EHealth(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Mode-C daemon is not supported on Windows")
	}
	if testing.Short() {
		t.Skip("skipping daemon e2e in -short mode")
	}

	binPath := buildKapiBowrain(t)

	pluginDir := t.TempDir()
	require.NoError(t, copyFile(binPath, filepath.Join(pluginDir, "kapi-bowrain")))
	require.NoError(t, os.Chmod(filepath.Join(pluginDir, "kapi-bowrain"), 0o755))

	// Drop a manifest beside the binary that declares Mode C.
	m := &manifest.Manifest{
		ManifestVersion: manifest.CurrentVersion,
		Plugin:          "bowrain",
		Version:         "0.0.0-test",
		Binary:          "kapi-bowrain",
		Capabilities: manifest.Capabilities{
			SourceConnectors: []manifest.SourceConnector{
				{ID: "bowrain-source"},
			},
		},
		Daemon: &manifest.DaemonConfig{
			IdleTimeoutSeconds:    300,
			StartupTimeoutSeconds: 30,
			Handshake: &manifest.Handshake{
				Type:   "stdio-handshake",
				Fields: []string{"socket", "version"},
			},
		},
	}
	require.NoError(t, m.Validate())
	enc, err := json.MarshalIndent(m, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(pluginDir, "manifest.json"), enc, 0o644))

	plugin := &pluginhost.Plugin{
		Dir:        pluginDir,
		BinaryPath: filepath.Join(pluginDir, "kapi-bowrain"),
		Manifest:   m,
		Source: pluginhost.Source{
			Order: 1,
			Label: "test",
			Path:  pluginDir,
		},
	}

	pool := pluginhost.NewDaemonPool(pluginhost.DaemonPoolOptions{
		MaxDaemons: 2,
	})
	t.Cleanup(pool.Shutdown)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client, err := pool.Acquire(ctx, plugin)
	require.NoError(t, err)
	require.NotNil(t, client.Conn)

	control := pb.NewDaemonControlServiceClient(client.Conn)
	resp, err := control.Health(ctx, &pb.HealthRequest{})
	require.NoError(t, err)
	assert.NotEmpty(t, resp.GetVersion())
	assert.GreaterOrEqual(t, resp.GetUptimeSeconds(), int64(0))

	// Idempotent shutdown must not panic. We don't assert process death
	// here — the daemon-side Shutdown handler returns BEFORE the
	// process has actually exited, and the parent (this test) doesn't
	// reap until pool.Shutdown(). So checking processAlive after
	// Shutdown RPC sees a zombie. The pool.Shutdown cleanup below is
	// what guarantees the process is reaped.
	_, _ = control.Shutdown(ctx, &pb.ShutdownRequest{GraceSeconds: 1})

	pid := client.PID()
	require.NotZero(t, pid)

	// pool.Shutdown is registered with t.Cleanup; running it now
	// proves the pool can reap a daemon that already requested its
	// own Shutdown.
	pool.Shutdown()
	require.Eventually(t, func() bool {
		return !processAlive(pid)
	}, 10*time.Second, 100*time.Millisecond, "daemon should be reaped after pool.Shutdown")
}

// buildKapiBowrain compiles the kapi-bowrain binary from this package
// and returns the absolute path to the produced executable.
func buildKapiBowrain(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	out := filepath.Join(dir, "kapi-bowrain")

	// We're in bowrain/cli/cmd/kapi-bowrain, so "." is correct.
	cmd := exec.CommandContext(t.Context(), "go", "build", "-o", out, ".")
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stderr
	cmd.Env = os.Environ()
	require.NoErrorf(t, cmd.Run(), "build kapi-bowrain")
	return out
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	defer out.Close()
	buf := make([]byte, 32*1024)
	for {
		n, rerr := in.Read(buf)
		if n > 0 {
			if _, werr := out.Write(buf[:n]); werr != nil {
				return werr
			}
		}
		if rerr != nil {
			if errors.Is(rerr, io.EOF) {
				return nil
			}
			return rerr
		}
	}
}

func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	if err := proc.Signal(syscall.Signal(0)); err != nil {
		return false
	}
	return true
}
