package host

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/project"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

func newTestCmd() *EnvCommand {
	cmd := NewEnvCommand(context.Background(), "test")
	AddProjectFlag(cmd)
	return cmd
}

// writeProject writes a valid kapi.yaml recipe + adjacent .kapi/ state dir
// at `dir` so project.ResolveLayout recognizes it. `name` is the recipe's
// project label; the filename is always kapi.yaml.
func writeProject(t *testing.T, dir, name string) string {
	t.Helper()
	recipe := filepath.Join(dir, project.RecipeFileName)
	proj := &project.KapiProject{Version: "v1", Name: name}
	require.NoError(t, project.Save(recipe, proj))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, project.StateDirName), 0o755))
	return recipe
}

func TestResolveProjectPath_ExplicitFlagWins(t *testing.T) {
	unsetEnv(t, projectEnvVar)
	dir := t.TempDir()
	recipe := writeProject(t, dir, "flag")
	t.Chdir(t.TempDir()) // cwd has nothing — flag must be used

	cmd := newTestCmd()
	require.NoError(t, cmd.Flags().Set(projectFlagName, recipe))

	got, err := ResolveProjectPath(cmd)
	require.NoError(t, err)
	assert.Equal(t, recipe, got)
}

// TestResolveProjectPath_ExplicitPathIsAbsolute proves a relative -p resolves
// to the absolute recipe path. Callers take filepath.Dir of this value as the
// project root and relativize against it before writing the committed record;
// a root of "." relativizes nothing, which put an absolute machine path into
// `.kapi/state/` — a file that exists to travel in git.
func TestResolveProjectPath_ExplicitPathIsAbsolute(t *testing.T) {
	unsetEnv(t, projectEnvVar)
	dir := t.TempDir()
	recipe := writeProject(t, dir, "relative")
	t.Chdir(dir)

	cmd := newTestCmd()
	require.NoError(t, cmd.Flags().Set(projectFlagName, project.RecipeFileName))

	got, err := ResolveProjectPath(cmd)
	require.NoError(t, err)
	assert.True(t, filepath.IsAbs(got), "want an absolute recipe path, got %q", got)
	assertSamePath(t, recipe, got)
}

// TestResolveProjectPath_ExplicitDirectoryResolvesRecipe proves -p accepts a
// project directory, which the flag's own help promises.
func TestResolveProjectPath_ExplicitDirectoryResolvesRecipe(t *testing.T) {
	unsetEnv(t, projectEnvVar)
	dir := t.TempDir()
	recipe := writeProject(t, dir, "dir")
	t.Chdir(t.TempDir())

	cmd := newTestCmd()
	require.NoError(t, cmd.Flags().Set(projectFlagName, dir))

	got, err := ResolveProjectPath(cmd)
	require.NoError(t, err)
	assertSamePath(t, recipe, got)
}

// TestResolveProjectPath_MissingExplicitPathIsPassedThrough proves an
// unresolvable -p is returned unchanged, so the load that follows reports the
// missing recipe once rather than this resolution reporting it first.
func TestResolveProjectPath_MissingExplicitPathIsPassedThrough(t *testing.T) {
	unsetEnv(t, projectEnvVar)
	t.Chdir(t.TempDir())

	cmd := newTestCmd()
	require.NoError(t, cmd.Flags().Set(projectFlagName, "no/such/kapi.yaml"))

	got, err := ResolveProjectPath(cmd)
	require.NoError(t, err)
	assert.Equal(t, "no/such/kapi.yaml", got)
}

// assertSamePath compares two paths after resolving symlinks, so a macOS
// /var → /private/var temp dir does not read as a mismatch.
func assertSamePath(t *testing.T, want, got string) {
	t.Helper()
	w, err := filepath.EvalSymlinks(want)
	require.NoError(t, err)
	g, err := filepath.EvalSymlinks(got)
	require.NoError(t, err)
	assert.Equal(t, w, g)
}

func TestResolveProjectPath_EnvVarFallback(t *testing.T) {
	unsetEnv(t, noProjectEnvVar)
	dir := t.TempDir()
	recipe := writeProject(t, dir, "env")
	t.Setenv(projectEnvVar, recipe)
	t.Chdir(t.TempDir())

	got, err := ResolveProjectPath(newTestCmd())
	require.NoError(t, err)
	assert.Equal(t, recipe, got)
}

func TestResolveProjectPath_AutoDiscoveryFromCwd(t *testing.T) {
	unsetEnv(t, projectEnvVar)
	unsetEnv(t, noProjectEnvVar)
	root := t.TempDir()
	// Register real path (realpath resolves macOS symlinks like /var -> /private/var).
	realRoot, err := filepath.EvalSymlinks(root)
	require.NoError(t, err)
	recipe := writeProject(t, realRoot, "auto")

	// Run from a subdirectory N levels deep.
	sub := filepath.Join(realRoot, "src", "deep", "nested")
	require.NoError(t, os.MkdirAll(sub, 0o755))
	t.Chdir(sub)

	got, err := ResolveProjectPath(newTestCmd())
	require.NoError(t, err)
	assert.Equal(t, recipe, got)
}

func TestResolveProjectPath_NoProjectReturnsEmpty(t *testing.T) {
	unsetEnv(t, projectEnvVar)
	unsetEnv(t, noProjectEnvVar)
	empty := t.TempDir()
	realEmpty, err := filepath.EvalSymlinks(empty)
	require.NoError(t, err)
	t.Chdir(realEmpty)

	got, err := ResolveProjectPath(newTestCmd())
	require.NoError(t, err)
	assert.Empty(t, got, "no project found should return empty without error")
}

// A .kapi/ state dir with no adjacent kapi.yaml is a broken layout: the
// project's identity was lost. ResolveProjectPath surfaces that as an error
// (it is not the "no project here" case, which returns empty).
func TestResolveProjectPath_StateDirWithoutRecipeErrors(t *testing.T) {
	unsetEnv(t, projectEnvVar)
	unsetEnv(t, noProjectEnvVar)
	dir := t.TempDir()
	real, err := filepath.EvalSymlinks(dir)
	require.NoError(t, err)

	require.NoError(t, os.MkdirAll(filepath.Join(real, project.StateDirName), 0o755))
	t.Chdir(real)

	got, err := ResolveProjectPath(newTestCmd())
	require.Error(t, err)
	assert.Empty(t, got)
	require.ErrorIs(t, err, project.ErrRecipeMissing)
}

func TestRequireProjectPath_ErrorWhenMissing(t *testing.T) {
	unsetEnv(t, projectEnvVar)
	unsetEnv(t, noProjectEnvVar)
	empty := t.TempDir()
	realEmpty, err := filepath.EvalSymlinks(empty)
	require.NoError(t, err)
	t.Chdir(realEmpty)

	_, err = RequireProjectPath(newTestCmd())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no kapi project found")
}

// TestResolveProjectPath_NoProjectEnvVarSkipsDiscovery verifies that
// KAPI_NO_PROJECT suppresses the git-style upward walk even when a recipe is
// present in the cwd — the guard tests, scripts, and scene recorders rely on
// so an in-repo invocation never binds to a checked-in (e.g. dogfood) recipe.
func TestResolveProjectPath_NoProjectEnvVarSkipsDiscovery(t *testing.T) {
	unsetEnv(t, projectEnvVar)
	t.Setenv(noProjectEnvVar, "1")
	dir := t.TempDir()
	real, err := filepath.EvalSymlinks(dir)
	require.NoError(t, err)
	writeProject(t, real, "dogfood")
	t.Chdir(real)

	got, err := ResolveProjectPath(newTestCmd())
	require.NoError(t, err)
	assert.Empty(t, got, "KAPI_NO_PROJECT must skip discovery of a recipe in cwd")
}

// TestResolveProjectPath_NoProjectEnvVarSkipsEnvFallback verifies KAPI_NO_PROJECT
// also wins over the KAPI_PROJECT env fallback (an explicit -p flag still wins —
// see TestResolveProjectPath_ExplicitFlagBeatsNoProject).
func TestResolveProjectPath_NoProjectEnvVarSkipsEnvFallback(t *testing.T) {
	t.Setenv(projectEnvVar, "/some/where/kapi.yaml")
	t.Setenv(noProjectEnvVar, "1")

	got, err := ResolveProjectPath(newTestCmd())
	require.NoError(t, err)
	assert.Empty(t, got)
}

// TestResolveProjectPath_ExplicitFlagBeatsNoProject verifies an explicit -p
// flag overrides KAPI_NO_PROJECT — opting out of discovery never blocks a
// caller that names the recipe directly.
func TestResolveProjectPath_ExplicitFlagBeatsNoProject(t *testing.T) {
	t.Setenv(noProjectEnvVar, "1")
	dir := t.TempDir()
	recipe := writeProject(t, dir, "explicit")
	t.Chdir(t.TempDir())

	cmd := newTestCmd()
	require.NoError(t, cmd.Flags().Set(projectFlagName, recipe))

	got, err := ResolveProjectPath(cmd)
	require.NoError(t, err)
	assert.Equal(t, recipe, got)
}
