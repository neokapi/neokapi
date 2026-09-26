package host

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// runGit runs git for a test, with an identity and no user configuration.
func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "git %s: %s", strings.Join(args, " "), out)
}

// sharedGitProject writes one project into a repository whose context is kept
// on a git ref, pushes it to a bare repository standing in for the hosting
// service, and clones it a second time: two machines' checkouts of one
// project.
func sharedGitProject(t *testing.T) (first, second, bare string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	t.Setenv("GIT_CONFIG_GLOBAL", "/dev/null")
	t.Setenv("GIT_CONFIG_SYSTEM", "/dev/null")
	t.Setenv("GIT_AUTHOR_NAME", "t")
	t.Setenv("GIT_AUTHOR_EMAIL", "t@example.com")
	t.Setenv("GIT_COMMITTER_NAME", "t")
	t.Setenv("GIT_COMMITTER_EMAIL", "t@example.com")
	t.Setenv("KAPI_CONFIG_DIR", t.TempDir())

	first = contextOpsProject(t, "ctxsync-shared")
	recipe, err := os.ReadFile(recipeOf(first))
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(recipeOf(first), append(recipe, []byte("context:\n  backend: git\n")...), 0o600))

	bare = filepath.Join(t.TempDir(), "origin.git")
	runGit(t, first, "init", "--quiet", "--bare", bare)
	runGit(t, first, "init", "--quiet")
	runGit(t, first, "add", ".")
	runGit(t, first, "commit", "--quiet", "-m", "project")
	runGit(t, first, "remote", "add", "origin", bare)
	runGit(t, first, "push", "--quiet", "origin", "HEAD:refs/heads/main")
	second = filepath.Join(t.TempDir(), "second")
	runGit(t, filepath.Dir(second), "clone", "--quiet", "--branch", "main", bare, second)
	return first, second, bare
}

// TestContextTravelsThroughAGitRef records context on one machine, pushes it
// to the repository's context ref, pulls it on another machine and finds the
// same stores there; then the other direction, and the answers' sync line.
func TestContextTravelsThroughAGitRef(t *testing.T) {
	first, second, _ := sharedGitProject(t)
	ctx := t.Context()
	appA, _ := contextOpsApp(t)
	appB, _ := contextOpsApp(t)

	kept := proposeUtilise(t, appA, first, person)
	_, err := appA.KeepContextOperation(ctx, ContextKeepRequest{Actor: person, Project: recipeOf(first), ID: kept.ID})
	require.NoError(t, err)

	before := appA.ContextSyncStatus(ctx, first)
	require.NotNil(t, before, "a project with a shared backend reports how far apart it is")
	assert.Positive(t, before.ToPush)
	assert.True(t, before.Contacted.IsZero())

	pushed, err := appA.PushProjectContext(ctx, recipeOf(first))
	require.NoError(t, err)
	assert.Equal(t, before.ToPush, pushed.Pushed)
	assert.Zero(t, pushed.ToPush)

	pulled, err := appB.PullProjectContext(ctx, recipeOf(second))
	require.NoError(t, err)
	assert.Equal(t, pushed.Pushed, pulled.Merged)
	assert.Zero(t, pulled.ToPull)
	assert.Zero(t, pulled.ToPush, "what was pulled is not pushed back")

	dbA, err := appA.ProjectDB(ctx, first)
	require.NoError(t, err)
	dbB, err := appB.ProjectDB(ctx, second)
	require.NoError(t, err)
	rowsA := projectionRows(t, appA, dbA)
	require.NotEmpty(t, rowsA["tb_concepts"], "the kept rule is a term")
	assert.Equal(t, rowsA, projectionRows(t, appB, dbB), "the second machine's stores equal the first's")

	// The other way: a suggestion recorded on the second machine.
	proposeUtilise(t, appB, second, agentIn("s2"))
	status := appB.ContextSyncStatus(ctx, second)
	require.NotNil(t, status)
	assert.Equal(t, 1, status.ToPush)
	_, err = appB.PushProjectContext(ctx, recipeOf(second))
	require.NoError(t, err)
	back, err := appA.PullProjectContext(ctx, recipeOf(first))
	require.NoError(t, err)
	assert.Equal(t, 1, back.Merged)

	var text strings.Builder
	require.NoError(t, back.FormatText(&text))
	assert.Contains(t, text.String(), "Context: 0 to push, 0 to pull (as of ")
}

// TestContextSyncOfflineAndLocal covers the two ways a sync does not happen:
// a backend that cannot be reached, which exits with its own code and keeps
// the queue, and a machine that chose to keep the project's context local.
func TestContextSyncOfflineAndLocal(t *testing.T) {
	first, _, _ := sharedGitProject(t)
	ctx := t.Context()
	app, _ := contextOpsApp(t)
	proposeUtilise(t, app, first, person)

	runGit(t, first, "remote", "set-url", "origin", filepath.Join(t.TempDir(), "gone.git"))
	_, err := app.PushProjectContext(ctx, recipeOf(first))
	require.Error(t, err)
	assert.Equal(t, ExitUnreachable, ExitCode(nil, err))
	assert.Contains(t, err.Error(), "1 operation wait")
	status := app.ContextSyncStatus(ctx, first)
	require.NotNil(t, status)
	assert.Equal(t, 1, status.ToPush, "an unreachable backend leaves the queue as it was")
	assert.NotEmpty(t, status.Error)

	info, err := app.SetContextBackend(ctx, recipeOf(first), "local", "")
	require.NoError(t, err)
	assert.Equal(t, "local", info.Kind)
	assert.Equal(t, "machine", info.From)
	assert.Equal(t, "git", info.Recipe)
	assert.Nil(t, app.ContextSyncStatus(ctx, first), "a local backend reports no sync line")
	_, err = app.PullProjectContext(ctx, recipeOf(first))
	require.ErrorContains(t, err, "there is nowhere to sync with")

	info, err = app.SetContextBackend(ctx, recipeOf(first), "recipe", "")
	require.NoError(t, err)
	assert.Equal(t, "git", info.Kind)
	assert.Equal(t, "recipe", info.From)
}
