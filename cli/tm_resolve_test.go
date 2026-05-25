package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/sievepen"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTMTestCmd returns a bare command carrying the resource flags the tm
// subcommands share (--name/--file/--local), for exercising path resolution
// without running a full command.
func newTMTestCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "tm"}
	AddResourceFlags(cmd)
	return cmd
}

// newTMLeverageTestCmd returns a bare command carrying the --tm flag that the
// tm-leverage tool command registers, for exercising openToolTM resolution.
func newTMLeverageTestCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "tm-leverage"}
	cmd.Flags().String("tm", "", "named TM or path")
	return cmd
}

// writeTMProject creates a .kapi project with a .kapi/ state dir and returns the
// project root plus the conventional authoritative TM path (.kapi/tm.db).
func writeTMProject(t *testing.T) (root, tmPath string) {
	t.Helper()
	root = t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".kapi"), 0o755))
	recipe := `version: v1
name: tm
defaults:
  source_language: en
  target_languages: [fr]
content:
  - path: "locales/en/*.json"
    target: "locales/{lang}/*.json"
`
	require.NoError(t, os.WriteFile(filepath.Join(root, "tm.kapi"), []byte(recipe), 0o644))
	return root, filepath.Join(root, ".kapi", "tm.db")
}

// TestResolveTMCmdPath_ProjectAware asserts that with no flag inside a project,
// the tm commands resolve the project's authoritative TM (.kapi/tm.db) — the
// same file kapi extract pre-fills from and kapi merge writes back to.
func TestResolveTMCmdPath_ProjectAware(t *testing.T) {
	root, tmPath := writeTMProject(t)
	t.Chdir(root)

	a := &App{}
	got, err := a.resolveTMCmdPath(newTMTestCmd())
	require.NoError(t, err)
	assert.Equal(t, tmPath, got, "no flag inside a project must resolve .kapi/tm.db, not ./tm.db")
}

// TestResolveTMCmdPath_ExplicitFlagWins asserts that --local and --file override
// the project TM (explicit user intent).
func TestResolveTMCmdPath_ExplicitFlagWins(t *testing.T) {
	root, _ := writeTMProject(t)
	t.Chdir(root)
	a := &App{}

	localCmd := newTMTestCmd()
	require.NoError(t, localCmd.Flags().Set("local", "true"))
	got, err := a.resolveTMCmdPath(localCmd)
	require.NoError(t, err)
	assert.Equal(t, "tm.db", got, "--local must mean ./tm.db, not the project TM")

	fileCmd := newTMTestCmd()
	explicit := filepath.Join(root, "custom.db")
	require.NoError(t, fileCmd.Flags().Set("file", explicit))
	got, err = a.resolveTMCmdPath(fileCmd)
	require.NoError(t, err)
	assert.Equal(t, explicit, got, "--file must win over the project TM")
}

// TestResolveTMCmdPath_NoProject asserts the fallback to ./tm.db when there is
// no project to bind.
func TestResolveTMCmdPath_NoProject(t *testing.T) {
	t.Chdir(t.TempDir())
	a := &App{}
	got, err := a.resolveTMCmdPath(newTMTestCmd())
	require.NoError(t, err)
	assert.Equal(t, "tm.db", got, "outside a project, default to ./tm.db")
}

// TestOpenToolTM_LeveragesProjectTM asserts that inside a project the tm-leverage
// tool command opens .kapi/tm.db and the resolved provider returns the exact
// match stored there — proving the leverage path is wired (not NullTMProvider).
func TestOpenToolTM_LeveragesProjectTM(t *testing.T) {
	root, tmPath := writeTMProject(t)

	// Seed the project TM with one en→fr exact match.
	tm, err := sievepen.NewSQLiteTM(tmPath)
	require.NoError(t, err)
	require.NoError(t, tm.Add(sievepen.TMEntry{
		ID: "e1",
		Variants: map[model.LocaleID][]model.Run{
			"en": {{Text: &model.TextRun{Text: "Welcome back"}}},
			"fr": {{Text: &model.TextRun{Text: "Bon retour"}}},
		},
	}))
	require.NoError(t, tm.Close())

	t.Chdir(root)
	a := &App{}
	provider, cleanup, err := a.openToolTM(newTMLeverageTestCmd())
	require.NoError(t, err)
	require.NotNil(t, provider, "inside a project the provider must be the project TM, not nil/Null")
	defer cleanup()

	got, found := provider.LookupExact("Welcome back", "en", "fr")
	assert.True(t, found, "exact match must be found in the project TM")
	assert.Equal(t, "Bon retour", got)
}

// TestOpenToolTM_NoProjectNoFlag asserts that outside a project with no --tm
// flag, openToolTM returns no provider (a noop cleanup) so the tool falls back
// to today's no-match behavior rather than erroring.
func TestOpenToolTM_NoProjectNoFlag(t *testing.T) {
	t.Chdir(t.TempDir())
	a := &App{}
	provider, cleanup, err := a.openToolTM(newTMLeverageTestCmd())
	require.NoError(t, err)
	assert.Nil(t, provider, "no project + no --tm flag means no provider")
	require.NotNil(t, cleanup)
	cleanup() // must be safe to call
}

// TestOpenToolTM_ExplicitFileFlag asserts that --tm <path> wins over project
// resolution and opens the named file.
func TestOpenToolTM_ExplicitFileFlag(t *testing.T) {
	dir := t.TempDir()
	explicit := filepath.Join(dir, "named.db")
	tm, err := sievepen.NewSQLiteTM(explicit)
	require.NoError(t, err)
	require.NoError(t, tm.Add(sievepen.TMEntry{
		ID: "e1",
		Variants: map[model.LocaleID][]model.Run{
			"en": {{Text: &model.TextRun{Text: "Save"}}},
			"de": {{Text: &model.TextRun{Text: "Speichern"}}},
		},
	}))
	require.NoError(t, tm.Close())

	cmd := newTMLeverageTestCmd()
	require.NoError(t, cmd.Flags().Set("tm", explicit))
	a := &App{}
	provider, cleanup, err := a.openToolTM(cmd)
	require.NoError(t, err)
	require.NotNil(t, provider)
	defer cleanup()

	got, found := provider.LookupExact("Save", "en", "de")
	assert.True(t, found)
	assert.Equal(t, "Speichern", got)
}
