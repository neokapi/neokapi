package host

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/workspace"
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
// the queue, and a recipe that declares no backend.
func TestContextSyncOfflineAndLocal(t *testing.T) {
	first, _, _ := sharedGitProject(t)
	ctx := t.Context()
	app, _ := contextOpsApp(t)
	proposeUtilise(t, app, first, person)

	runGit(t, first, "remote", "set-url", "origin", filepath.Join(t.TempDir(), "gone.git"))
	_, err := app.PushProjectContext(ctx, recipeOf(first))
	require.Error(t, err)
	assert.Equal(t, ExitUnreachable, ExitCode(nil, err))
	assert.Contains(t, err.Error(), "1 operation waits")
	status := app.ContextSyncStatus(ctx, first)
	require.NotNil(t, status)
	assert.Equal(t, 1, status.ToPush, "an unreachable backend leaves the queue as it was")
	assert.NotEmpty(t, status.Error)

	_, err = app.SyncProjectContext(ctx, ContextSyncRequest{Project: recipeOf(first)})
	require.Error(t, err)
	assert.Equal(t, ExitUnreachable, ExitCode(nil, err), "a sync that cannot pull exits the same way")

	info, err := app.ContextBackend(ctx, recipeOf(first))
	require.NoError(t, err)
	assert.Equal(t, "git", info.Kind)
	assert.Equal(t, "recipe", info.From)

	local := contextOpsProject(t, "ctxsync-local")
	assert.Nil(t, app.ContextSyncStatus(ctx, local), "a local backend reports no sync line")
	_, err = app.SyncProjectContext(ctx, ContextSyncRequest{Project: recipeOf(local)})
	require.ErrorContains(t, err, "there is nowhere to sync with")
}

// TestAResetTravelsWithTheContext syncs a reset from one machine to another:
// the second machine's stores are rebuilt without what the reset set aside,
// and both machines' logs keep it.
func TestAResetTravelsWithTheContext(t *testing.T) {
	first, second, _ := sharedGitProject(t)
	ctx := t.Context()
	appA, _ := contextOpsApp(t)
	appB, _ := contextOpsApp(t)

	kept := proposeUtilise(t, appA, first, person)
	_, err := appA.KeepContextOperation(ctx, ContextKeepRequest{Actor: person, Project: recipeOf(first), ID: kept.ID})
	require.NoError(t, err)
	synced, err := appA.SyncProjectContext(ctx, ContextSyncRequest{Project: recipeOf(first)})
	require.NoError(t, err)
	require.NotNil(t, synced.Pull)
	require.NotNil(t, synced.Push)
	assert.Positive(t, synced.Push.Pushed)

	_, err = appB.SyncProjectContext(ctx, ContextSyncRequest{Project: recipeOf(second)})
	require.NoError(t, err)
	require.NotEmpty(t, vocabularyFindings(checkWith(t, appB, second)), "the kept rule reached the second machine")

	_, err = appA.ResetContext(ctx, ContextResetRequest{Actor: person, Project: recipeOf(first), Before: kept.ID})
	require.NoError(t, err)
	_, err = appA.SyncProjectContext(ctx, ContextSyncRequest{Project: recipeOf(first)})
	require.NoError(t, err)
	_, err = appB.SyncProjectContext(ctx, ContextSyncRequest{Project: recipeOf(second)})
	require.NoError(t, err)

	assert.Empty(t, vocabularyFindings(checkWith(t, appB, second)), "the reset reached the second machine's stores")
	log, err := appB.ContextOperations(ctx, ContextLogRequest{Project: recipeOf(second)})
	require.NoError(t, err)
	statuses := map[string]string{}
	for _, op := range log.Operations {
		statuses[op.ID] = string(op.Status)
	}
	assert.Equal(t, "reset", statuses[kept.ID], "the log keeps what the reset set aside")
}

// TestContextTravelsThroughARemoteTheCallerHolds is the browser engine's
// sync: a remote the caller opened, not one the recipe declares. One machine
// pushes, another pulls the same stores and has nothing of its own to push,
// and a second sync moves nothing.
func TestContextTravelsThroughARemoteTheCallerHolds(t *testing.T) {
	ctx := t.Context()
	first := contextOpsProject(t, "ctxsync-held")
	second := t.TempDir()
	require.NoError(t, os.CopyFS(second, os.DirFS(first)))
	shared := t.TempDir()
	appA, _ := contextOpsApp(t)
	appB, _ := contextOpsApp(t)

	kept := proposeUtilise(t, appA, first, person)
	_, err := appA.KeepContextOperation(ctx, ContextKeepRequest{Actor: person, Project: recipeOf(first), ID: kept.ID})
	require.NoError(t, err)

	a, err := appA.SyncProjectContextWith(ctx, recipeOf(first), workspace.NewFileRemote(shared), true, true)
	require.NoError(t, err)
	require.NotNil(t, a.Pull)
	assert.True(t, a.Pull.Empty, "nothing was pushed to the folder before")
	require.NotNil(t, a.Push)
	assert.Positive(t, a.Push.Pushed)

	b, err := appB.SyncProjectContextWith(ctx, recipeOf(second), workspace.NewFileRemote(shared), true, true)
	require.NoError(t, err)
	assert.Equal(t, a.Push.Pushed, b.Pull.Merged)
	assert.Zero(t, b.Push.Pushed, "what was pulled is not pushed back")

	dbA, err := appA.ProjectDB(ctx, first)
	require.NoError(t, err)
	dbB, err := appB.ProjectDB(ctx, second)
	require.NoError(t, err)
	rowsA := projectionRows(t, appA, dbA)
	require.NotEmpty(t, rowsA["tb_concepts"])
	assert.Equal(t, rowsA, projectionRows(t, appB, dbB))

	again, err := appA.SyncProjectContextWith(ctx, recipeOf(first), workspace.NewFileRemote(shared), true, false)
	require.NoError(t, err)
	assert.Zero(t, again.Pull.Merged)
	assert.Nil(t, again.Push, "a push that was not asked for does not run")
}
