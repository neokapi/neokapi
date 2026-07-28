package cli

import (
	"context"
	"os"
	"path/filepath"
	"testing"

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

// writeMemoryProject creates a .kapi project with a .kapi/ state dir and returns the
// project root plus the conventional authoritative content memory path (.kapi/memory.db).
func writeMemoryProject(t *testing.T) (root, memoryPath string) {
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
	require.NoError(t, os.WriteFile(filepath.Join(root, "kapi.yaml"), []byte(recipe), 0o644))
	return root, filepath.Join(root, ".kapi", "memory.db")
}

// TestResolveMemoryCmdPath_ProjectAware asserts that with no flag inside a project,
// the tm commands resolve the project's authoritative content memory (.kapi/memory.db) — the
// same file kapi extract pre-fills from and kapi merge writes back to.
func TestResolveMemoryCmdPath_ProjectAware(t *testing.T) {
	root, memoryPath := writeMemoryProject(t)
	t.Chdir(root)

	a := &App{}
	got, err := a.ResolveMemoryCmdPath(newMemoryTestCmd())
	require.NoError(t, err)
	assert.Equal(t, memoryPath, got, "no flag inside a project must resolve .kapi/memory.db, not ./memory.db")
}

// TestResolveMemoryCmdPath_ExplicitFlagWins asserts that --local and --file override
// the project content memory (explicit user intent).
func TestResolveMemoryCmdPath_ExplicitFlagWins(t *testing.T) {
	root, _ := writeMemoryProject(t)
	t.Chdir(root)
	a := &App{}

	localCmd := newMemoryTestCmd()
	require.NoError(t, localCmd.Flags().Set("local", "true"))
	got, err := a.ResolveMemoryCmdPath(localCmd)
	require.NoError(t, err)
	assert.Equal(t, "memory.db", got, "--local must mean ./memory.db, not the project content memory")

	fileCmd := newMemoryTestCmd()
	explicit := filepath.Join(root, "custom.db")
	require.NoError(t, fileCmd.Flags().Set("file", explicit))
	got, err = a.ResolveMemoryCmdPath(fileCmd)
	require.NoError(t, err)
	assert.Equal(t, explicit, got, "--file must win over the project content memory")
}

// TestResolveMemoryCmdPath_NoProject asserts the fallback to ./memory.db when there is
// no project to bind.
func TestResolveMemoryCmdPath_NoProject(t *testing.T) {
	t.Chdir(t.TempDir())
	a := &App{}
	got, err := a.ResolveMemoryCmdPath(newMemoryTestCmd())
	require.NoError(t, err)
	assert.Equal(t, "memory.db", got, "outside a project, default to ./memory.db")
}

// TestOpenToolMemory_LeveragesProjectMemory asserts that inside a project the recycle
// tool command opens .kapi/memory.db and the resolved provider returns the exact
// match stored there — proving the leverage path is wired (not NullMemoryProvider).
func TestOpenToolMemory_LeveragesProjectMemory(t *testing.T) {
	root, memoryPath := writeMemoryProject(t)

	// Seed the project content memory with one en→fr exact match.
	tm, err := memory.NewSQLiteStore(memoryPath)
	require.NoError(t, err)
	require.NoError(t, tm.Add(t.Context(), memory.Entry{
		ID: "e1",
		Variants: map[model.LocaleID][]model.Run{
			"en": {{Text: &model.TextRun{Text: "Welcome back"}}},
			"fr": {{Text: &model.TextRun{Text: "Bon retour"}}},
		},
	}))
	require.NoError(t, tm.Close())

	t.Chdir(root)
	a := &App{}
	provider, cleanup, err := a.OpenToolMemory(newMemoryLeverageTestCmd())
	require.NoError(t, err)
	require.NotNil(t, provider, "inside a project the provider must be the project content memory, not nil/Null")
	defer cleanup()

	got, found := provider.LookupExact("Welcome back", "en", "fr")
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

	got, found := provider.LookupExact("Save", "en", "de")
	assert.True(t, found)
	assert.Equal(t, "Speichern", got)
}
