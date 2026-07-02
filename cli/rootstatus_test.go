package cli

import (
	"bytes"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newBareRoot builds a minimal root command wired the way kapi/cmd/kapi wires
// its root: RunE delegates to App.RootRunE.
func newBareRoot(a *App) (*cobra.Command, *bytes.Buffer) {
	root := &cobra.Command{
		Use:           "kapi",
		Short:         "test root",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE:          a.RootRunE,
	}
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	return root, &out
}

// TestRootRunE_InProject: bare `kapi` inside a project prints the status
// summary (the `kapi status` renderer) plus the `kapi up` hint lines.
func TestRootRunE_InProject(t *testing.T) {
	root := writeStatusProject(t)
	t.Chdir(root)

	a := processOnlyApp(t)
	cmd, buf := newBareRoot(a)
	cmd.SetArgs(nil)

	// runStatus prints through output.Print on the fresh status command,
	// which writes to the redirected stdout; capture both.
	stdout, err := captureStdout(t, cmd.Execute)
	require.NoError(t, err)

	combined := stdout + buf.String()
	assert.Contains(t, combined, "scope", "the status coverage grid renders")
	assert.Contains(t, combined, "run `kapi up` to reconcile")
	assert.Contains(t, combined, "kapi --help for all commands")
	assert.NotContains(t, combined, "Usage:", "in-project bare kapi shows status, not help")
}

// TestRootRunE_NoProject: outside a project the bare root falls back to the
// standard cobra help.
func TestRootRunE_NoProject(t *testing.T) {
	t.Setenv("KAPI_NO_PROJECT", "1")
	a := processOnlyApp(t)
	cmd, buf := newBareRoot(a)
	cmd.SetArgs(nil)

	require.NoError(t, cmd.Execute())
	assert.Contains(t, buf.String(), "Usage:", "outside a project the bare root shows help")
	assert.NotContains(t, buf.String(), "run `kapi up` to reconcile")
}
