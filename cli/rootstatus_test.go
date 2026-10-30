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
		RunE:          func(cmd *cobra.Command, args []string) error { return RootRunE(a, cmd, args) },
	}
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	return root, &out
}

// TestRootRunE_InProject: bare `kapi` inside a project with a target language
// prints the status summary (the `kapi status` renderer) plus the `kapi up`
// hint lines.
func TestRootRunE_InProject(t *testing.T) {
	root := writeStatusProject(t)
	t.Chdir(root)

	a := processOnlyApp(t)
	cmd, buf := newBareRoot(a)
	cmd.SetArgs(nil)

	// RunStatus prints through output.Print on the fresh status command,
	// which writes to the redirected stdout; capture both.
	stdout, err := captureStdout(t, cmd.Execute)
	require.NoError(t, err)

	combined := stdout + buf.String()
	assert.Contains(t, combined, "scope", "the status coverage grid renders")
	assert.Contains(t, combined, "run `kapi up` to catch up")
	assert.Contains(t, combined, "kapi --help for the commands")
	assert.NotContains(t, combined, "run `kapi check` to check the content")
	assert.NotContains(t, combined, "Usage:", "in-project bare kapi shows status, not help")
}

// TestRootRunE_OneLanguageProject: in a project that names no target language
// the hint is `kapi check`, since there is nothing for `kapi up` to catch up.
func TestRootRunE_OneLanguageProject(t *testing.T) {
	recipe, _ := monolingualProject(t)
	t.Setenv("KAPI_NO_PROJECT", "")
	t.Setenv("KAPI_PROJECT", recipe)

	a := processOnlyApp(t)
	cmd, buf := newBareRoot(a)
	cmd.SetArgs(nil)

	stdout, err := captureStdout(t, cmd.Execute)
	require.NoError(t, err)

	combined := stdout + buf.String()
	assert.Contains(t, combined, "source", "the status summary renders")
	assert.Contains(t, combined, "run `kapi check` to check the content")
	assert.Contains(t, combined, "kapi --help for the commands")
	assert.NotContains(t, combined, "kapi up")
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
	assert.NotContains(t, buf.String(), "run `kapi up` to catch up")
}
