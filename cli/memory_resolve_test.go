package cli

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	corememory "github.com/neokapi/neokapi/core/memory"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/memory"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newMemoryTestCmd returns a bare command carrying the resource flags the tm
// subcommands share (--name/--file/--local), for exercising path resolution
// without running a full command.
func newMemoryTestCmd() *EnvCommand {
	cmd := NewEnvCommand(context.Background(), "tm")
	AddResourceFlags(cmd)
	return cmd
}

// newMemoryLeverageTestCmd returns a bare command carrying the --memory flag that the
// recycle tool command registers, for exercising OpenToolMemory resolution.
func newMemoryLeverageTestCmd() *EnvCommand {
	cmd := NewEnvCommand(context.Background(), "recycle")
	cmd.Flags().String("memory", "", "named Memory or path")
	return cmd
}

// writeMemoryProject creates a .kapi project with a .kapi/ state dir and returns
// the project root.
func writeMemoryProject(t *testing.T) (root string) {
	t.Helper()
	root = t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".kapi"), 0o755))
	recipe := `version: v1
name: tm
defaults:
  source_language: en
  target_languages: [fr]
collections:
  - path: "locales/en/*.json"
    target: "locales/{lang}/*.json"
`
	require.NoError(t, os.WriteFile(filepath.Join(root, "kapi.yaml"), []byte(recipe), 0o644))
	return root
}

// TestResolveMemoryStore_ProjectAware asserts that with no flag inside a project,
// the memory commands resolve the PROJECT's own store — the same content memory
// kapi extract pre-fills from and kapi merge writes back to. The selection names
// the project rather than a path, because the store is no longer a file this
// command may open for itself.
func TestResolveMemoryStore_ProjectAware(t *testing.T) {
	root := writeMemoryProject(t)
	t.Chdir(root)

	a := &App{}
	got, err := a.ResolveMemoryStore(newMemoryTestCmd())
	require.NoError(t, err)
	assert.True(t, got.InProject(), "no flag inside a project must resolve the project store")
	assert.Empty(t, got.Path, "the project store is not addressed by path")
	assertSameDir(t, root, got.Root)
}

// TestResolveMemoryStore_ExplicitFlagWins asserts that --local and --file override
// the project store (explicit user intent) and select a standalone file.
func TestResolveMemoryStore_ExplicitFlagWins(t *testing.T) {
	root := writeMemoryProject(t)
	t.Chdir(root)
	a := &App{}

	localCmd := newMemoryTestCmd()
	require.NoError(t, localCmd.Flags().Set("local", "true"))
	got, err := a.ResolveMemoryStore(localCmd)
	require.NoError(t, err)
	assert.False(t, got.InProject(), "--local must not select the project store")
	assert.Equal(t, "memory.db", got.Path, "--local must mean ./memory.db")

	fileCmd := newMemoryTestCmd()
	explicit := filepath.Join(root, "custom.db")
	require.NoError(t, fileCmd.Flags().Set("file", explicit))
	got, err = a.ResolveMemoryStore(fileCmd)
	require.NoError(t, err)
	assert.Equal(t, explicit, got.Path, "--file must win over the project store")
}

// TestResolveMemoryStore_NoProject asserts the fallback to ./memory.db when there
// is no project to bind.
func TestResolveMemoryStore_NoProject(t *testing.T) {
	t.Chdir(t.TempDir())
	a := &App{}
	got, err := a.ResolveMemoryStore(newMemoryTestCmd())
	require.NoError(t, err)
	assert.False(t, got.InProject())
	assert.Equal(t, "memory.db", got.Path, "outside a project, default to ./memory.db")
}

// assertSameDir compares two directory paths through their resolved symlinks,
// so a t.TempDir() under /var vs /private/var does not read as a mismatch.
func assertSameDir(t *testing.T, want, got string) {
	t.Helper()
	wantReal, err := filepath.EvalSymlinks(want)
	require.NoError(t, err)
	gotReal, err := filepath.EvalSymlinks(got)
	require.NoError(t, err)
	assert.Equal(t, wantReal, gotReal)
}

// TestOpenToolMemory_LeveragesProjectMemory asserts that inside a project the
// recycle tool command reads the project store's content memory and the resolved
// provider returns the exact match stored there — proving the leverage path is
// wired (not NullMemoryProvider).
func TestOpenToolMemory_LeveragesProjectMemory(t *testing.T) {
	root := writeMemoryProject(t)

	// Seed the project store's content memory with one en→fr exact match.
	a := &App{}
	defer a.Shutdown()
	db, err := a.ProjectDB(t.Context(), root)
	require.NoError(t, err)
	require.NoError(t, db.Memory().Add(t.Context(), memory.Entry{
		ID: "e1",
		Variants: map[model.LocaleID][]model.Run{
			"en": {{Text: &model.TextRun{Text: "Welcome back"}}},
			"fr": {{Text: &model.TextRun{Text: "Bon retour"}}},
		},
	}))

	t.Chdir(root)
	provider, cleanup, err := a.OpenToolMemory(newMemoryLeverageTestCmd())
	require.NoError(t, err)
	require.NotNil(t, provider, "inside a project the provider must be the project content memory, not nil/Null")
	defer cleanup()

	got, found := lookupText(t.Context(), provider, "Welcome back", "en", "fr")
	assert.True(t, found, "exact match must be found in the project content memory")
	assert.Equal(t, "Bon retour", got)
}

// TestOpenToolMemory_NoProjectNoFlag asserts that outside a project with no --memory
// flag, OpenToolMemory returns no provider (a noop cleanup) so the tool falls back
// to today's no-match behavior rather than erroring.
func TestOpenToolMemory_NoProjectNoFlag(t *testing.T) {
	t.Chdir(t.TempDir())
	a := &App{}
	provider, cleanup, err := a.OpenToolMemory(newMemoryLeverageTestCmd())
	require.NoError(t, err)
	assert.Nil(t, provider, "no project + no --memory flag means no provider")
	require.NotNil(t, cleanup)
	cleanup() // must be safe to call
}

// TestOpenToolMemory_ExplicitFileFlag asserts that --memory <path> wins over project
// resolution and opens the named file.
func TestOpenToolMemory_ExplicitFileFlag(t *testing.T) {
	dir := t.TempDir()
	explicit := filepath.Join(dir, "named.db")
	tm, err := memory.NewSQLiteStore(explicit)
	require.NoError(t, err)
	require.NoError(t, tm.Add(t.Context(), memory.Entry{
		ID: "e1",
		Variants: map[model.LocaleID][]model.Run{
			"en": {{Text: &model.TextRun{Text: "Save"}}},
			"de": {{Text: &model.TextRun{Text: "Speichern"}}},
		},
	}))
	require.NoError(t, tm.Close())

	cmd := newMemoryLeverageTestCmd()
	require.NoError(t, cmd.Flags().Set("memory", explicit))
	a := &App{}
	provider, cleanup, err := a.OpenToolMemory(cmd)
	require.NoError(t, err)
	require.NotNil(t, provider)
	defer cleanup()

	got, found := lookupText(t.Context(), provider, "Save", "en", "de")
	assert.True(t, found)
	assert.Equal(t, "Speichern", got)
}

// lookupText is the flattened exact lookup these tests assert on, expressed
// against the one provider interface.
func lookupText(ctx context.Context, p corememory.Provider, text string, source, target model.LocaleID) (string, bool) {
	m, ok := p.Lookup(ctx, corememory.Request{
		Text:     text,
		Source:   source,
		Target:   target,
		MinScore: 100,
		Verbatim: true,
	})
	if !ok {
		return "", false
	}
	return model.RunsText(m.TargetRuns), true
}
