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
// terms subcommands share (--name/--file/--local), for exercising store
// resolution without running a full command.
func newTermsTestCmd() *EnvCommand {
	cmd := NewEnvCommand(context.Background(), "tb")
	AddResourceFlags(cmd)
	return cmd
}

// writeTermsProject creates a plain project — no terms binding, because a recipe
// no longer has one to write. The vocabulary lives in the project's own store.
func writeTermsProject(t *testing.T) (root string) {
	t.Helper()
	root = t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".kapi"), 0o755))
	recipe := `version: v1
name: tb
defaults:
  source_language: en
  target_languages: [fr]
content:
  - path: "locales/en/*.json"
    target: "locales/{lang}/*.json"
`
	require.NoError(t, os.WriteFile(filepath.Join(root, "kapi.yaml"), []byte(recipe), 0o644))
	return root
}

// writeProfileTermsProject creates a project whose profile binds a STANDALONE
// terms store to a point in the context space. That binding is the one place a
// recipe still names a terms file: it is a path into the local project with
// nothing on the wire for it, so it stays a path.
func writeProfileTermsProject(t *testing.T) (root, boundPath string) {
	t.Helper()
	root = t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".kapi"), 0o755))
	recipe := `version: v1
name: tb
defaults:
  source_language: en
  target_languages: [fr]
coordinates:
  product:
    - id: press
profiles:
  - terms: press-terms.db
content:
  - name: docs
    path: "locales/en/*.json"
    target: "locales/{lang}/*.json"
`
	require.NoError(t, os.WriteFile(filepath.Join(root, "kapi.yaml"), []byte(recipe), 0o644))
	return root, filepath.Join(root, "press-terms.db")
}

// TestResolveTermsCmdStore_ProjectAware asserts that with no flag inside a
// project, the terms commands resolve the PROJECT's own store — the same
// vocabulary `kapi check --ship` and `kapi term-check` enforce.
func TestResolveTermsCmdStore_ProjectAware(t *testing.T) {
	root := writeTermsProject(t)
	t.Chdir(root)

	a := &App{}
	got, err := a.ResolveTermsCmdStore(newTermsTestCmd())
	require.NoError(t, err)
	assert.True(t, got.InProject(), "no flag inside a project must resolve the project store")
	assert.Empty(t, got.Path, "the project store is not addressed by path")
	assertSameDir(t, root, got.Root)
}

// TestResolveTermsCmdStore_ProfileBindingWins asserts a profile's standalone
// `terms:` still selects that file — the surviving per-point binding.
func TestResolveTermsCmdStore_ProfileBindingWins(t *testing.T) {
	root, bound := writeProfileTermsProject(t)
	t.Chdir(root)

	a := &App{}
	got, err := a.ResolveTermsCmdStore(newTermsTestCmd())
	require.NoError(t, err)
	assert.False(t, got.InProject(), "a profile's terms: names a standalone store")
	assert.Equal(t, bound, got.Path)
}

// TestResolveTermsCmdStore_ExplicitFlagWins asserts that --local and --file
// override the project store (explicit user intent).
func TestResolveTermsCmdStore_ExplicitFlagWins(t *testing.T) {
	root := writeTermsProject(t)
	t.Chdir(root)
	a := &App{}

	localCmd := newTermsTestCmd()
	require.NoError(t, localCmd.Flags().Set("local", "true"))
	got, err := a.ResolveTermsCmdStore(localCmd)
	require.NoError(t, err)
	assert.False(t, got.InProject(), "--local must not select the project store")
	assert.Equal(t, "terms.db", got.Path, "--local must mean ./terms.db")

	fileCmd := newTermsTestCmd()
	explicit := filepath.Join(root, "custom.db")
	require.NoError(t, fileCmd.Flags().Set("file", explicit))
	got, err = a.ResolveTermsCmdStore(fileCmd)
	require.NoError(t, err)
	assert.Equal(t, explicit, got.Path, "--file must win over the project store")
}

// TestResolveTermsCmdStore_NoProject asserts the fallback to ./terms.db when
// there is no project to bind.
func TestResolveTermsCmdStore_NoProject(t *testing.T) {
	t.Chdir(t.TempDir())
	a := &App{}
	got, err := a.ResolveTermsCmdStore(newTermsTestCmd())
	require.NoError(t, err)
	assert.False(t, got.InProject())
	assert.Equal(t, "terms.db", got.Path, "outside a project, default to ./terms.db")
}
