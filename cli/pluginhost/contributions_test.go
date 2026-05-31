package pluginhost

import (
	"context"
	"testing"
	"time"

	"github.com/neokapi/neokapi/core/plugin/manifest"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func contribPlugin(name string, cc manifest.CommandContribution) *Plugin {
	return &Plugin{
		Manifest: &manifest.Manifest{
			ManifestVersion: "1",
			Plugin:          name,
			Version:         "0.0.0",
			Binary:          "bin",
			Capabilities:    manifest.Capabilities{CommandContributions: []manifest.CommandContribution{cc}},
		},
	}
}

func TestAttachContributions_registersFlagsOnBuiltin(t *testing.T) {
	host := NewHost([]*Plugin{contribPlugin("bowrain", manifest.CommandContribution{
		Command:    "init",
		Handler:    "init-connect",
		EngageWhen: "server",
		Flags: []manifest.FlagSpec{
			{Name: "server", Type: "string", Description: "server URL"},
			{Name: "anonymous", Type: "bool"},
		},
	})}, nil)

	root := &cobra.Command{Use: "kapi"}
	initCmd := &cobra.Command{Use: "init", RunE: func(*cobra.Command, []string) error { return nil }}
	root.AddCommand(initCmd)

	var warns []string
	AttachContributions(root, host, func(m string) { warns = append(warns, m) })

	assert.Empty(t, warns)
	assert.NotNil(t, initCmd.Flags().Lookup("server"), "contributed --server flag should be registered")
	assert.NotNil(t, initCmd.Flags().Lookup("anonymous"), "contributed --anonymous flag should be registered")
}

func TestAttachContributions_unknownCommandWarns(t *testing.T) {
	host := NewHost([]*Plugin{contribPlugin("p", manifest.CommandContribution{
		Command: "does-not-exist", Handler: "h",
	})}, nil)

	root := &cobra.Command{Use: "kapi"}
	var warns []string
	AttachContributions(root, host, func(m string) { warns = append(warns, m) })

	require.Len(t, warns, 1)
	assert.Contains(t, warns[0], "not a built-in command")
}

func TestContributionEngaged(t *testing.T) {
	cmd := &cobra.Command{Use: "init"}
	cmd.Flags().String("server", "", "")
	cmd.Flags().Bool("anonymous", false, "")

	withEngage := manifest.CommandContribution{
		EngageWhen: "server",
		Flags:      []manifest.FlagSpec{{Name: "server", Type: "string"}, {Name: "anonymous", Type: "bool"}},
	}
	assert.False(t, contributionEngaged(cmd, withEngage), "not engaged before any flag is set")
	require.NoError(t, cmd.Flags().Set("server", "http://localhost:8080"))
	assert.True(t, contributionEngaged(cmd, withEngage), "engaged once the engage_when flag is set")

	// Without EngageWhen, any contributed flag engages it.
	anyFlag := manifest.CommandContribution{Flags: []manifest.FlagSpec{{Name: "anonymous", Type: "bool"}}}
	assert.False(t, contributionEngaged(&cobra.Command{Use: "x"}, anyFlag))
	require.NoError(t, cmd.Flags().Set("anonymous", "true"))
	assert.True(t, contributionEngaged(cmd, anyFlag))
}

// TestRunContributionSubprocessContextCancellation verifies that cancelling
// the context passed into runContributionSubprocess terminates the Mode-A
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
		done <- runContributionSubprocess(ctx, p, []string{"command", "wait"}, p.Dir)
	}()

	select {
	case err := <-done:
		require.Error(t, err, "cancelled contribution subprocess must return an error")
		assert.ErrorIs(t, err, context.Canceled, "error must wrap the context cancellation")
	case <-time.After(10 * time.Second):
		t.Fatal("runContributionSubprocess did not return after context cancellation; child likely outlived parent context")
	}
}

// TestRunContributionSubprocessNilContext verifies runContributionSubprocess
// tolerates a nil context (defaults to context.Background) rather than
// panicking.
func TestRunContributionSubprocessNilContext(t *testing.T) {
	p := makeSleepPlugin(t, 0) // returns immediately

	err := runContributionSubprocess(nil, p, []string{"command", "wait"}, p.Dir)
	assert.NoError(t, err)
}
