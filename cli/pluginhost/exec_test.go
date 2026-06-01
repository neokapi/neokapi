package pluginhost

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"testing"
	"time"

	"github.com/neokapi/neokapi/core/plugin/manifest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// makeSleepPlugin writes a tiny shell-script "plugin" that sleeps for the
// given number of seconds, plus a matching manifest, and returns a
// *Plugin pointing at it. The script is the worst-case Mode-A child: a
// long-running process that ignores its argv. We rely on the propagated
// context to kill it.
func makeSleepPlugin(t *testing.T, sleepSeconds int) *Plugin {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("shell-script plugin stub is not portable to Windows")
	}

	dir := t.TempDir()
	binName := "sleeper"
	binPath := filepath.Join(dir, binName)
	script := "#!/bin/sh\nsleep " + strconv.Itoa(sleepSeconds) + "\n"
	require.NoError(t, os.WriteFile(binPath, []byte(script), 0o755))

	m := &manifest.Manifest{
		ManifestVersion: manifest.CurrentVersion,
		Plugin:          "sleepy",
		Version:         "0.0.1",
		Binary:          binName,
		Capabilities: manifest.Capabilities{
			Commands: []manifest.Command{{Name: "wait"}},
		},
	}
	require.NoError(t, m.Validate())

	return &Plugin{
		Dir:        dir,
		BinaryPath: binPath,
		Manifest:   m,
		Source:     Source{Order: 1, Label: "test", Path: dir},
	}
}

// TestRunSubprocessContextCancellation verifies that cancelling the
// context passed into runSubprocess terminates the Mode-A child promptly
// (rather than blocking until the long sleep finishes) and that a context
// error is surfaced to the caller.
func TestRunSubprocessContextCancellation(t *testing.T) {
	p := makeSleepPlugin(t, 60) // would block for a minute if not killed

	ctx, cancel := context.WithCancel(context.Background())

	// Cancel shortly after start, simulating a SIGTERM to kapi.
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()

	done := make(chan error, 1)
	start := time.Now()
	go func() {
		done <- runSubprocess(ctx, p, []string{"command", "wait"})
	}()

	select {
	case err := <-done:
		elapsed := time.Since(start)
		require.Error(t, err, "cancelled subprocess must return an error")
		require.ErrorIs(t, err, context.Canceled, "error must wrap the context cancellation")
		assert.Less(t, elapsed, 10*time.Second,
			"runSubprocess should return promptly after cancellation, not wait out the child's 60s sleep")
	case <-time.After(15 * time.Second):
		t.Fatal("runSubprocess did not return after context cancellation; child likely outlived parent context")
	}
}

// TestRunSubprocessSuccess verifies the happy path: a child that exits 0
// returns nil and the propagated context does not interfere.
func TestRunSubprocessSuccess(t *testing.T) {
	p := makeSleepPlugin(t, 0) // returns immediately

	err := runSubprocess(context.Background(), p, []string{"command", "wait"})
	require.NoError(t, err)
}

// TestRunSubprocessNilContext verifies runSubprocess tolerates a nil
// context (defaults to context.Background) rather than panicking.
func TestRunSubprocessNilContext(t *testing.T) {
	p := makeSleepPlugin(t, 0)

	//nolint:staticcheck // intentionally passing nil to exercise the guard
	err := runSubprocess(nil, p, []string{"command", "wait"})
	require.NoError(t, err)
}
