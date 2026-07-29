package pluginhost

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/neokapi/neokapi/core/plugin/manifest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// sharedFakeDaemon builds the fake daemon once per test binary and returns
// the path to it, already executed once. Every test in the package shares
// that one binary, and both properties are load-bearing.
//
// Built once: a `go build` per caller is roughly a second of nested toolchain
// work each, and it is the daemon spawns themselves that then compete for the
// CPU it consumed.
//
// Executed once, here, outside any startup budget: on macOS the *first* exec
// of a given inode pays a code-signature/policy assessment that the kernel
// then caches per inode. Measured on this tree, that first exec costs
// 330-590ms with the assessment path warm and 5.1-6.6s with it cold, against
// 10-20ms for every later exec of the same inode. DaemonPool.spawn starts its
// startup budget before cmd.Start, so a binary materialised per test put that
// one-off platform cost inside the handshake window — where, cold, it can
// exceed the window on its own. Paying it here takes it off the clock.
//
// Fixtures reach this binary through linkFakeDaemon, which hard-links rather
// than copies so the plugin dir shares this inode and inherits the warm
// assessment.
var sharedFakeDaemon = sync.OnceValues(buildAndWarmFakeDaemon)

// fakeDaemonDir holds the shared build; TestMain removes it.
var fakeDaemonDir string

func buildAndWarmFakeDaemon() (string, error) {
	dir, err := os.MkdirTemp("", "kapi-pluginhost-fakedaemon-")
	if err != nil {
		return "", fmt.Errorf("temp dir: %w", err)
	}
	fakeDaemonDir = dir
	binPath := filepath.Join(dir, "fakedaemon")

	// Build from this package's testdata.
	cmd := exec.CommandContext(context.Background(), "go", "build", "-o", binPath, "./testdata/fakedaemon")
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stderr
	cmd.Env = os.Environ()
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("build fakedaemon: %w", err)
	}
	if err := warmFakeDaemon(binPath); err != nil {
		return "", err
	}
	return binPath, nil
}

// warmFakeDaemon runs the freshly built binary once and waits for its
// handshake, so the platform's first-exec cost for this inode is paid here
// rather than inside a DaemonPool startup budget.
//
// The FAKE_DAEMON_* knobs are cleared explicitly: this runs lazily from
// whichever test happens to call buildFakeDaemon first, and that test may
// already have t.Setenv'd one of them (FAKE_DAEMON_NO_HANDSHAKE would hang
// the warm-up, FAKE_DAEMON_SPAWN_LOG would corrupt the spawn count
// TestDaemonPool_ConcurrentAcquireSpawnsOneDaemon asserts on).
func warmFakeDaemon(binPath string) error {
	cmd := exec.CommandContext(context.Background(), binPath, "daemon")
	cmd.Env = append(os.Environ(),
		"FAKE_DAEMON_NAME=warmup",
		"FAKE_DAEMON_NO_HANDSHAKE=",
		"FAKE_DAEMON_CRASH_AFTER=",
		"FAKE_DAEMON_STARTUP_DELAY=",
		"FAKE_DAEMON_SPAWN_LOG=",
		"FAKE_DAEMON_BRIDGE=",
	)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("warm fakedaemon: stdout pipe: %w", err)
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("warm fakedaemon: start: %w", err)
	}

	// The bound is deliberately generous — this is the one place that pays a
	// cold first-exec assessment, and the point of it is not to fail on one.
	read := make(chan error, 1)
	go func() {
		_, rerr := bufio.NewReader(stdout).ReadString('\n')
		read <- rerr
	}()
	var readErr error
	select {
	case readErr = <-read:
	case <-time.After(60 * time.Second):
		// Join the reader before Wait: os/exec documents reading a
		// StdoutPipe concurrently with Wait as unsafe (Wait closes the pipe
		// under the reader) — the same hazard DaemonClient.close handles
		// with drainDone. Kill unblocks the read by closing stdout.
		readErr = errors.New("no handshake within 60s")
		_ = cmd.Process.Kill()
		<-read
	}

	// SIGTERM lets the daemon unlink its own socket (a no-op if we killed it).
	_ = cmd.Process.Signal(syscall.SIGTERM)
	_ = cmd.Wait()
	if readErr != nil {
		return fmt.Errorf("warm fakedaemon: read handshake: %w", readErr)
	}
	return nil
}

// buildFakeDaemon returns the shared fake daemon binary.
func buildFakeDaemon(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("Mode-C daemon is not supported on Windows")
	}
	binPath, err := sharedFakeDaemon()
	require.NoError(t, err)
	return binPath
}

// linkFakeDaemon materialises the shared daemon binary at dst.
//
// It hard-links, so dst is the *same inode* as the warmed binary and costs
// no first-exec assessment. Copying — what the fixtures used to do — makes a
// new inode and puts that cost back inside the caller's startup budget.
// Falls back to a byte copy if the link cannot be made (different volume).
func linkFakeDaemon(t *testing.T, src, dst string) {
	t.Helper()
	if err := os.Link(src, dst); err == nil {
		return
	}
	require.NoError(t, copyFile(src, dst))
	require.NoError(t, os.Chmod(dst, 0o755))
}

// makePlugin assembles a plugin install dir with a manifest.json and the
// given daemon binary, then returns a *Plugin pointing at it.
func makePlugin(t *testing.T, name, daemonBin string, daemonCfg *manifest.DaemonConfig) *Plugin {
	t.Helper()
	dir := t.TempDir()
	binDest := filepath.Join(dir, "fakedaemon")
	linkFakeDaemon(t, daemonBin, binDest)

	m := &manifest.Manifest{
		ManifestVersion: manifest.CurrentVersion,
		Plugin:          name,
		Version:         "0.0.1",
		Binary:          "fakedaemon",
		Capabilities: manifest.Capabilities{
			SourceConnectors: []manifest.SourceConnector{
				{ID: name + "-source"},
			},
		},
		Daemon: daemonCfg,
	}
	require.NoError(t, m.Validate())

	manPath := filepath.Join(dir, "manifest.json")
	enc, err := json.MarshalIndent(m, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(manPath, enc, 0o644))

	return &Plugin{
		Dir:        dir,
		BinaryPath: binDest,
		Manifest:   m,
		Source: Source{
			Order: 1,
			Label: "test",
			Path:  dir,
		},
	}
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
		n, err := in.Read(buf)
		if n > 0 {
			if _, werr := out.Write(buf[:n]); werr != nil {
				return werr
			}
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
	}
}

func TestDaemonPool_AcquireReturnsHealthyClient(t *testing.T) {
	bin := buildFakeDaemon(t)
	plugin := makePlugin(t, "fake", bin, &manifest.DaemonConfig{
		StartupTimeoutSeconds: 5,
		IdleTimeoutSeconds:    300,
	})

	pool := NewDaemonPool(DaemonPoolOptions{MaxDaemons: 4})
	t.Cleanup(pool.Shutdown)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client, err := pool.Acquire(ctx, plugin)
	require.NoError(t, err)
	require.NotNil(t, client)
	require.NotNil(t, client.Conn)
	assert.Equal(t, "fake", client.Plugin.Name())
	assert.NotEmpty(t, client.Socket)
	assert.NotZero(t, client.PID())
}

func TestDaemonPool_AcquireReusesDaemon(t *testing.T) {
	bin := buildFakeDaemon(t)
	plugin := makePlugin(t, "fake", bin, &manifest.DaemonConfig{
		StartupTimeoutSeconds: 5,
		IdleTimeoutSeconds:    300,
	})

	pool := NewDaemonPool(DaemonPoolOptions{MaxDaemons: 4})
	t.Cleanup(pool.Shutdown)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	c1, err := pool.Acquire(ctx, plugin)
	require.NoError(t, err)
	pid1 := c1.PID()

	c2, err := pool.Acquire(ctx, plugin)
	require.NoError(t, err)
	assert.Equal(t, pid1, c2.PID(), "Acquire should reuse the running daemon")
	assert.Same(t, c1, c2)
	assert.Len(t, pool.Active(), 1)
}

func TestDaemonPool_LRUEviction(t *testing.T) {
	bin := buildFakeDaemon(t)
	pluginA := makePlugin(t, "fake-a", bin, &manifest.DaemonConfig{
		StartupTimeoutSeconds: 5,
		IdleTimeoutSeconds:    300,
	})
	pluginB := makePlugin(t, "fake-b", bin, &manifest.DaemonConfig{
		StartupTimeoutSeconds: 5,
		IdleTimeoutSeconds:    300,
	})
	pluginC := makePlugin(t, "fake-c", bin, &manifest.DaemonConfig{
		StartupTimeoutSeconds: 5,
		IdleTimeoutSeconds:    300,
	})

	pool := NewDaemonPool(DaemonPoolOptions{MaxDaemons: 2})
	t.Cleanup(pool.Shutdown)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err := pool.Acquire(ctx, pluginA)
	require.NoError(t, err)
	time.Sleep(20 * time.Millisecond)
	_, err = pool.Acquire(ctx, pluginB)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"fake-a", "fake-b"}, pool.Active())

	// A is now the LRU (B touched more recently). Acquiring C should
	// evict A.
	time.Sleep(20 * time.Millisecond)
	_, err = pool.Acquire(ctx, pluginC)
	require.NoError(t, err)

	// Give the eviction goroutine a moment to tear A down.
	require.Eventually(t, func() bool {
		active := pool.Active()
		if len(active) != 2 {
			return false
		}
		return !slices.Contains(active, "fake-a")
	}, 5*time.Second, 50*time.Millisecond, "fake-a should be evicted")
	assert.ElementsMatch(t, []string{"fake-b", "fake-c"}, pool.Active())
}

func TestDaemonPool_ShutdownKillsAll(t *testing.T) {
	bin := buildFakeDaemon(t)
	pluginA := makePlugin(t, "fake-a", bin, &manifest.DaemonConfig{
		StartupTimeoutSeconds: 5,
		IdleTimeoutSeconds:    300,
	})
	pluginB := makePlugin(t, "fake-b", bin, &manifest.DaemonConfig{
		StartupTimeoutSeconds: 5,
		IdleTimeoutSeconds:    300,
	})

	pool := NewDaemonPool(DaemonPoolOptions{MaxDaemons: 4})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cA, err := pool.Acquire(ctx, pluginA)
	require.NoError(t, err)
	cB, err := pool.Acquire(ctx, pluginB)
	require.NoError(t, err)
	pidA := cA.PID()
	pidB := cB.PID()
	require.NotZero(t, pidA)
	require.NotZero(t, pidB)

	pool.Shutdown()
	assert.Empty(t, pool.Active())
	assert.True(t, cA.IsClosed())
	assert.True(t, cB.IsClosed())

	// Both processes should have exited.
	require.Eventually(t, func() bool {
		return !processAlive(pidA) && !processAlive(pidB)
	}, 5*time.Second, 50*time.Millisecond, "daemon processes should exit after Shutdown")

	// Acquire after Shutdown returns an error.
	_, err = pool.Acquire(ctx, pluginA)
	require.Error(t, err)
}

func TestDaemonPool_HandshakeTimeout(t *testing.T) {
	bin := buildFakeDaemon(t)
	plugin := makePlugin(t, "fake-no-handshake", bin, &manifest.DaemonConfig{
		StartupTimeoutSeconds: 1,
	})

	pool := NewDaemonPool(DaemonPoolOptions{
		MaxDaemons:     4,
		StartupTimeout: 500 * time.Millisecond,
	})
	t.Cleanup(pool.Shutdown)

	// Inject FAKE_DAEMON_NO_HANDSHAKE=1 via env. The pool's spawn pulls
	// from os.Environ(), so set it here.
	t.Setenv("FAKE_DAEMON_NO_HANDSHAKE", "1")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := pool.Acquire(ctx, plugin)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "handshake timeout")
	assert.Empty(t, pool.Active())
}

func TestDaemonPool_RespawnAfterCrash(t *testing.T) {
	bin := buildFakeDaemon(t)
	plugin := makePlugin(t, "fake-crashy", bin, &manifest.DaemonConfig{
		StartupTimeoutSeconds: 5,
		IdleTimeoutSeconds:    300,
	})

	pool := NewDaemonPool(DaemonPoolOptions{MaxDaemons: 4})
	t.Cleanup(pool.Shutdown)

	// First acquire: daemon will crash 200ms in.
	t.Setenv("FAKE_DAEMON_CRASH_AFTER", "200ms")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	c1, err := pool.Acquire(ctx, plugin)
	require.NoError(t, err)
	pid1 := c1.PID()

	// Wait until the gRPC conn observes the crash.
	require.Eventually(t, func() bool {
		s := c1.Conn.GetState().String()
		return s == "TRANSIENT_FAILURE" || s == "IDLE" || s == "CONNECTING" || s == "SHUTDOWN" || !processAlive(pid1)
	}, 5*time.Second, 50*time.Millisecond, "expected daemon to die")

	// Force the pool to consider the existing entry stale by closing the
	// conn under it. (The pool detects "IsClosed" on the client; conn
	// state alone isn't a trigger today, so we close.)
	c1.close(time.Second)

	// Now subsequent Acquire should respawn (no env crash this time).
	t.Setenv("FAKE_DAEMON_CRASH_AFTER", "")
	c2, err := pool.Acquire(ctx, plugin)
	require.NoError(t, err)
	require.NotNil(t, c2)
	assert.NotEqual(t, pid1, c2.PID(), "expected a fresh process after crash")
	assert.NotSame(t, c1, c2)
}

// TestDaemonPool_ConcurrentAcquireSpawnsOneDaemon validates that N
// concurrent Acquire() calls for the same plugin coordinate via the
// per-plugin spawn lock and only spawn ONE daemon process. Without the
// lock, each goroutine races past the cache-miss check and spawns its
// own JVM — the pool then returns the same winning client to all
// callers (so a PID-equality test would still pass), but N-1 daemon
// processes were started and killed for nothing. The spawn-log file
// counts actual spawns regardless of which client was returned.
func TestDaemonPool_ConcurrentAcquireSpawnsOneDaemon(t *testing.T) {
	bin := buildFakeDaemon(t)
	plugin := makePlugin(t, "fake", bin, &manifest.DaemonConfig{
		StartupTimeoutSeconds: 5,
		IdleTimeoutSeconds:    300,
	})

	// Daemon appends its PID to this file on startup. A 100ms startup
	// delay widens the race window so a missing spawn lock will be
	// caught reliably rather than relying on timing.
	spawnLog := filepath.Join(t.TempDir(), "spawns.log")
	t.Setenv("FAKE_DAEMON_SPAWN_LOG", spawnLog)
	t.Setenv("FAKE_DAEMON_STARTUP_DELAY", "100ms")

	pool := NewDaemonPool(DaemonPoolOptions{MaxDaemons: 4})
	t.Cleanup(pool.Shutdown)

	const concurrency = 10
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	start := make(chan struct{})
	results := make(chan *DaemonClient, concurrency)
	errs := make(chan error, concurrency)
	for range concurrency {
		go func() {
			<-start
			c, err := pool.Acquire(ctx, plugin)
			if err != nil {
				errs <- err
				return
			}
			results <- c
		}()
	}
	close(start)

	pids := map[int]struct{}{}
	for range concurrency {
		select {
		case c := <-results:
			pids[c.PID()] = struct{}{}
		case err := <-errs:
			t.Fatalf("Acquire failed: %v", err)
		case <-ctx.Done():
			t.Fatalf("timed out waiting for Acquire results")
		}
	}
	assert.Len(t, pids, 1, "all callers should receive the same client")
	assert.Len(t, pool.Active(), 1, "pool should hold exactly one client")

	logBytes, err := os.ReadFile(spawnLog)
	require.NoError(t, err)
	spawnLines := 0
	for _, b := range logBytes {
		if b == '\n' {
			spawnLines++
		}
	}
	assert.Equal(t, 1, spawnLines, "expected exactly one daemon to be spawned across %d concurrent Acquires (spawn log: %q)", concurrency, string(logBytes))
}

// TestDaemonPool_IdleWatcherKeepsRecentlyUsedDaemon validates the #37
// fix: the idle watcher must not tear down a daemon that a concurrent
// Acquire just handed out. With a short idle timeout, we keep re-acquiring
// (each Acquire touches the client) faster than the timeout across several
// watcher ticks. The under-lock LastUsed re-check must observe each fresh
// touch and never close the live daemon, so every Acquire returns the same
// healthy client and the pool keeps exactly one entry.
func TestDaemonPool_IdleWatcherKeepsRecentlyUsedDaemon(t *testing.T) {
	bin := buildFakeDaemon(t)
	plugin := makePlugin(t, "fake", bin, &manifest.DaemonConfig{StartupTimeoutSeconds: 5})

	// Short idle timeout → watcher ticks every 50ms. We re-acquire every
	// ~20ms, comfortably inside the window, for long enough to span many
	// ticks (so a missing re-check would tear the daemon down).
	pool := NewDaemonPool(DaemonPoolOptions{
		MaxDaemons:  4,
		IdleTimeout: 100 * time.Millisecond,
	})
	t.Cleanup(pool.Shutdown)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	first, err := pool.Acquire(ctx, plugin)
	require.NoError(t, err)
	firstPID := first.PID()
	require.NotZero(t, firstPID)

	deadline := time.Now().Add(600 * time.Millisecond)
	for time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
		c, err := pool.Acquire(ctx, plugin)
		require.NoError(t, err)
		require.False(t, c.IsClosed(), "idle watcher tore down a just-acquired daemon (TOCTOU)")
		assert.Same(t, first, c, "Acquire should keep reusing the same live daemon")
		assert.Equal(t, firstPID, c.PID(), "daemon process must not be respawned while in active use")
	}

	assert.Len(t, pool.Active(), 1, "pool should still hold the one live daemon")
	assert.False(t, first.IsClosed())
}

// TestDaemonPool_IdleWatcherClosesStaleDaemon is the companion to the
// keep-alive test: once a daemon goes untouched past its idle timeout, the
// watcher must tear it down and drop it from the pool.
func TestDaemonPool_IdleWatcherClosesStaleDaemon(t *testing.T) {
	bin := buildFakeDaemon(t)
	plugin := makePlugin(t, "fake", bin, &manifest.DaemonConfig{StartupTimeoutSeconds: 5})

	pool := NewDaemonPool(DaemonPoolOptions{
		MaxDaemons:  4,
		IdleTimeout: 100 * time.Millisecond,
	})
	t.Cleanup(pool.Shutdown)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client, err := pool.Acquire(ctx, plugin)
	require.NoError(t, err)

	// Stop touching it; the watcher should evict it after the idle window.
	require.Eventually(t, func() bool {
		return client.IsClosed() && len(pool.Active()) == 0
	}, 5*time.Second, 25*time.Millisecond, "idle daemon should be torn down once untouched past the timeout")
}

// processAlive reports whether a process with the given pid is alive on
// the host. It uses kill(pid, 0) which returns nil for a live process
// and an error for a reaped one.
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
