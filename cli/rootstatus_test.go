package cli

import (
	"bytes"
	"strings"
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
// prints the Map (the `kapi look` renderer) with its languages line, then the
// line naming where the commands are listed.
func TestRootRunE_InProject(t *testing.T) {
	root := writeStatusProject(t)
	t.Chdir(root)

	a := processOnlyApp(t)
	cmd, buf := newBareRoot(a)
	cmd.SetArgs(nil)

	// The Map prints through output.Print, which writes to the redirected
	// stdout; capture both.
	stdout, err := captureStdout(t, cmd.Execute)
	require.NoError(t, err)

	combined := stdout + buf.String()
	assert.Contains(t, combined, "1 file · 3 blocks", "the Map's counts render")
	assert.Contains(t, combined, "3 languages · ", "a target language adds the languages line")
	assert.Contains(t, combined, "kapi status for coverage")
	assert.Contains(t, combined, "See a file with `kapi look <path>`.")
	assert.True(t, strings.HasSuffix(combined, "\nkapi --help for the commands\n"), combined)
	assert.NotContains(t, combined, "Usage:", "in-project bare kapi shows the Map, not help")
}

// TestRootRunE_OneLanguageProject: in a project that names no target language
// the Map has no languages line and names `kapi check` as the next step.
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
	assert.Contains(t, combined, "docs/berths.md", "the Map lists the file")
	assert.Contains(t, combined, "Check with `kapi check`. See a file with `kapi look <path>`.")
	assert.Contains(t, combined, "kapi --help for the commands")
	assert.NotContains(t, combined, "kapi up")
	assert.NotContains(t, combined, "languages")
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
	assert.NotContains(t, buf.String(), "See a file with `kapi look <path>`.")
}
