package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/project"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func withCwd(t *testing.T, dir string) {
	t.Helper()
	prev, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(dir))
	t.Cleanup(func() { _ = os.Chdir(prev) })
}

func withEnv(t *testing.T, key, value string) {
	t.Helper()
	prev, had := os.LookupEnv(key)
	require.NoError(t, os.Setenv(key, value))
	t.Cleanup(func() {
		if had {
			_ = os.Setenv(key, prev)
		} else {
			_ = os.Unsetenv(key)
		}
	})
}

func unsetEnv(t *testing.T, key string) {
	t.Helper()
	prev, had := os.LookupEnv(key)
	_ = os.Unsetenv(key)
	t.Cleanup(func() {
		if had {
			_ = os.Setenv(key, prev)
		}
	})
}

func newTestCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "test"}
	AddProjectFlag(cmd)
	return cmd
}

// writeProject writes a valid {name}.kapi recipe + adjacent .kapi/ state dir
// at `dir` so project.ResolveLayout recognizes it.
func writeProject(t *testing.T, dir, name string) string {
	t.Helper()
	recipe := filepath.Join(dir, name+".kapi")
	proj := &project.KapiProject{Version: "v1", Name: name}
	require.NoError(t, project.Save(recipe, proj))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, project.StateDirName), 0o755))
	return recipe
}

func TestResolveProjectPath_ExplicitFlagWins(t *testing.T) {
	unsetEnv(t, ProjectEnvVar)
	dir := t.TempDir()
	recipe := writeProject(t, dir, "flag")
	withCwd(t, t.TempDir()) // cwd has nothing — flag must be used

	cmd := newTestCmd()
	require.NoError(t, cmd.Flags().Set(ProjectFlagName, recipe))

	got, err := ResolveProjectPath(cmd)
	require.NoError(t, err)
	assert.Equal(t, recipe, got)
}

func TestResolveProjectPath_EnvVarFallback(t *testing.T) {
	dir := t.TempDir()
	recipe := writeProject(t, dir, "env")
	withEnv(t, ProjectEnvVar, recipe)
	withCwd(t, t.TempDir())

	got, err := ResolveProjectPath(newTestCmd())
	require.NoError(t, err)
	assert.Equal(t, recipe, got)
}

func TestResolveProjectPath_AutoDiscoveryFromCwd(t *testing.T) {
	unsetEnv(t, ProjectEnvVar)
	root := t.TempDir()
	// Register real path (realpath resolves macOS symlinks like /var -> /private/var).
	realRoot, err := filepath.EvalSymlinks(root)
	require.NoError(t, err)
	recipe := writeProject(t, realRoot, "auto")

	// Run from a subdirectory N levels deep.
	sub := filepath.Join(realRoot, "src", "deep", "nested")
	require.NoError(t, os.MkdirAll(sub, 0o755))
	withCwd(t, sub)

	got, err := ResolveProjectPath(newTestCmd())
	require.NoError(t, err)
	assert.Equal(t, recipe, got)
}

func TestResolveProjectPath_NoProjectReturnsEmpty(t *testing.T) {
	unsetEnv(t, ProjectEnvVar)
	empty := t.TempDir()
	realEmpty, err := filepath.EvalSymlinks(empty)
	require.NoError(t, err)
	withCwd(t, realEmpty)

	got, err := ResolveProjectPath(newTestCmd())
	require.NoError(t, err)
	assert.Empty(t, got, "no project found should return empty without error")
}

func TestResolveProjectPath_AmbiguousLayoutWrapsError(t *testing.T) {
	unsetEnv(t, ProjectEnvVar)
	dir := t.TempDir()
	real, err := filepath.EvalSymlinks(dir)
	require.NoError(t, err)

	// Two sibling recipes — ambiguous.
	require.NoError(t, project.Save(filepath.Join(real, "a.kapi"), &project.KapiProject{Version: "v1", Name: "A"}))
	require.NoError(t, project.Save(filepath.Join(real, "b.kapi"), &project.KapiProject{Version: "v1", Name: "B"}))
	require.NoError(t, os.MkdirAll(filepath.Join(real, project.StateDirName), 0o755))
	withCwd(t, real)

	got, err := ResolveProjectPath(newTestCmd())
	require.Error(t, err)
	assert.Empty(t, got)
	assert.ErrorIs(t, err, project.ErrAmbiguousLayout)
	assert.Contains(t, err.Error(), "-p")
}

func TestRequireProjectPath_ErrorWhenMissing(t *testing.T) {
	unsetEnv(t, ProjectEnvVar)
	empty := t.TempDir()
	realEmpty, err := filepath.EvalSymlinks(empty)
	require.NoError(t, err)
	withCwd(t, realEmpty)

	_, err = RequireProjectPath(newTestCmd())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no .kapi project found")
}
