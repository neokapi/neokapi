package cli

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTermsTestCmd returns a bare command carrying the resource flags the
// terms subcommands share (--name/--file/--local), for exercising path
// resolution without running a full command.
func newTermsTestCmd() *EnvCommand {
	cmd := NewEnvCommand(context.Background(), "tb")
	AddResourceFlags(cmd)
	return cmd
}

// writeTermsProject creates a .kapi project binding defaults.terms and
// returns the project root plus the absolute bound path.
func writeTermsProject(t *testing.T) (root, boundPath string) {
	t.Helper()
	root = t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".kapi"), 0o755))
	recipe := `version: v1
name: tb
defaults:
  source_language: en
  target_languages: [fr]
  terms: .kapi/terms.db
content:
  - path: "locales/en/*.json"
    target: "locales/{lang}/*.json"
`
	require.NoError(t, os.WriteFile(filepath.Join(root, "kapi.yaml"), []byte(recipe), 0o644))
	return root, filepath.Join(root, ".kapi", "terms.db")
}

// TestResolveTermsCmdPath_ProjectAware asserts that with no flag inside a
// project, the terms store commands resolve the project's bound terms — matching
// what `kapi check --ship` and `kapi term-check` use.
func TestResolveTermsCmdPath_ProjectAware(t *testing.T) {
	root, bound := writeTermsProject(t)
	t.Chdir(root)

	a := &App{}
	got, err := a.ResolveTermsCmdPath(newTermsTestCmd())
	require.NoError(t, err)
	assert.Equal(t, bound, got, "no flag inside a project must resolve the bound terms, not ./terms.db")
}

// TestResolveTermsCmdPath_ExplicitFlagWins asserts that --local and --file
// override the project terms store (explicit user intent).
func TestResolveTermsCmdPath_ExplicitFlagWins(t *testing.T) {
	root, _ := writeTermsProject(t)
	t.Chdir(root)
	a := &App{}

	localCmd := newTermsTestCmd()
	require.NoError(t, localCmd.Flags().Set("local", "true"))
	got, err := a.ResolveTermsCmdPath(localCmd)
	require.NoError(t, err)
	assert.Equal(t, "terms.db", got, "--local must mean ./terms.db, not the project terms store")

	fileCmd := newTermsTestCmd()
	explicit := filepath.Join(root, "custom.db")
	require.NoError(t, fileCmd.Flags().Set("file", explicit))
	got, err = a.ResolveTermsCmdPath(fileCmd)
	require.NoError(t, err)
	assert.Equal(t, explicit, got, "--file must win over the project terms store")
}

// TestResolveTermsCmdPath_NoProject asserts the fallback to ./terms.db
// when there is no project to bind.
func TestResolveTermsCmdPath_NoProject(t *testing.T) {
	t.Chdir(t.TempDir())
	a := &App{}
	got, err := a.ResolveTermsCmdPath(newTermsTestCmd())
	require.NoError(t, err)
	assert.Equal(t, "terms.db", got, "outside a project, default to ./terms.db")
}
