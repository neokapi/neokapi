package pluginhost

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRunContributionSubprocessNilContext verifies RunContributionSubprocess
// tolerates a nil context (defaults to context.Background) rather than
// panicking.
func TestRunContributionSubprocessNilContext(t *testing.T) {
	p := makeSleepPlugin(t, 0) // returns immediately

	err := RunContributionSubprocess(context.TODO(), p, []string{"command", "wait"}, p.Dir)
	assert.NoError(t, err)
}

// TestRunContributionSubprocessContextCancellation verifies that cancelling
// the context passed into RunContributionSubprocess terminates the Mode-A
// contribution child promptly (rather than blocking until the long sleep
// finishes) and that a context-cancellation error is surfaced, mirroring the
// behavior of the command-dispatch path (exec.go's runSubprocess).
func TestRunContributionSubprocessContextCancellation(t *testing.T) {
	p := makeSleepPlugin(t, 60) // would block for a minute if not killed

	ctx, cancel := context.WithCancel(context.Background())

	// Cancel shortly after start, simulating a SIGTERM to kapi.
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()

	done := make(chan error, 1)
	go func() {
		done <- RunContributionSubprocess(ctx, p, []string{"command", "wait"}, p.Dir)
	}()

	select {
	case err := <-done:
		require.Error(t, err, "cancelled contribution subprocess must return an error")
		require.ErrorIs(t, err, context.Canceled, "error must wrap the context cancellation")
	case <-time.After(10 * time.Second):
		t.Fatal("RunContributionSubprocess did not return after context cancellation; child likely outlived parent context")
	}
}
