package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTermbaseTestCmd returns a bare command carrying the resource flags the
// termbase subcommands share (--name/--file/--local), for exercising path
// resolution without running a full command.
func newTermbaseTestCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "tb"}
	AddResourceFlags(cmd)
	return cmd
}

// writeTermbaseProject creates a .kapi project binding defaults.termbase and
// returns the project root plus the absolute bound path.
func writeTermbaseProject(t *testing.T) (root, boundPath string) {
	t.Helper()
	root = t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".kapi"), 0o755))
	recipe := `version: v1
name: tb
defaults:
  source_language: en
  target_languages: [fr]
  termbase: .kapi/termbase.db
content:
  - path: "locales/en/*.json"
    target: "locales/{lang}/*.json"
`
	require.NoError(t, os.WriteFile(filepath.Join(root, "tb.kapi"), []byte(recipe), 0o644))
	return root, filepath.Join(root, ".kapi", "termbase.db")
}

// TestResolveTermbaseCmdPath_ProjectAware asserts that with no flag inside a
// project, the termbase commands resolve the project's bound termbase — matching
// what `kapi verify` and `kapi term-check` use.
func TestResolveTermbaseCmdPath_ProjectAware(t *testing.T) {
	root, bound := writeTermbaseProject(t)
	t.Chdir(root)

	a := &App{}
	got, err := a.resolveTermbaseCmdPath(newTermbaseTestCmd())
	require.NoError(t, err)
	assert.Equal(t, bound, got, "no flag inside a project must resolve the bound termbase, not ./termbase.db")
}

// TestResolveTermbaseCmdPath_ExplicitFlagWins asserts that --local and --file
// override the project termbase (explicit user intent).
func TestResolveTermbaseCmdPath_ExplicitFlagWins(t *testing.T) {
	root, _ := writeTermbaseProject(t)
	t.Chdir(root)
	a := &App{}

	localCmd := newTermbaseTestCmd()
	require.NoError(t, localCmd.Flags().Set("local", "true"))
	got, err := a.resolveTermbaseCmdPath(localCmd)
	require.NoError(t, err)
	assert.Equal(t, "termbase.db", got, "--local must mean ./termbase.db, not the project termbase")

	fileCmd := newTermbaseTestCmd()
	explicit := filepath.Join(root, "custom.db")
	require.NoError(t, fileCmd.Flags().Set("file", explicit))
	got, err = a.resolveTermbaseCmdPath(fileCmd)
	require.NoError(t, err)
	assert.Equal(t, explicit, got, "--file must win over the project termbase")
}

// TestResolveTermbaseCmdPath_NoProject asserts the fallback to ./termbase.db
// when there is no project to bind.
func TestResolveTermbaseCmdPath_NoProject(t *testing.T) {
	t.Chdir(t.TempDir())
	a := &App{}
	got, err := a.resolveTermbaseCmdPath(newTermbaseTestCmd())
	require.NoError(t, err)
	assert.Equal(t, "termbase.db", got, "outside a project, default to ./termbase.db")
}
