package backend

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/contextop"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/host"
	"github.com/neokapi/neokapi/host/venue/schema"
	"github.com/neokapi/neokapi/kapi-desktop/backend/sample"
	"gopkg.in/yaml.v3"
)

// eventRecorder collects the names of the events an App emits.
type eventRecorder struct {
	mu    sync.Mutex
	names []string
}

func recordEvents(app *App) *eventRecorder {
	r := &eventRecorder{}
	InjectEventSink(app, func(name string, _ any) {
		r.mu.Lock()
		defer r.mu.Unlock()
		r.names = append(r.names, name)
	})
	return r
}

func (r *eventRecorder) count(name string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	n := 0
	for _, got := range r.names {
		if got == name {
			n++
		}
	}
	return n
}

// openSharingProject opens a project of its own in an App on a workspace of
// its own, and returns the tab id and the recipe path.
func openSharingProject(t *testing.T, app *App, name string) (string, string) {
	t.Helper()
	_, recipe := scaffoldNamedProject(t, filepath.Join(t.TempDir(), name), project.NewID(), name)
	tab, err := app.OpenProject(recipe)
	require.NoError(t, err)
	t.Cleanup(func() { app.CloseProject(tab.ID) })
	return tab.ID, recipe
}

// TestProjectSharing_TheRecipeNamesThePlace: a project keeps its context in
// the workspace until its recipe names a place, setting one writes the
// `context:` block and the home reads it back, and a local spec removes it.
func TestProjectSharing_TheRecipeNamesThePlace(t *testing.T) {
	app := newWorkspaceApp(t)
	events := recordEvents(app)
	tab, recipe := openSharingProject(t, app, "Sharing")
	location := workspaceRootOf(t, app)

	view, err := app.ShareView(tab)
	require.NoError(t, err)
	assert.Equal(t, &ProjectSharing{Kind: project.ContextBackendLocal, From: "default", Workspace: location}, view.Sharing)

	shared := filepath.Join(t.TempDir(), "team-share")
	got, err := app.SetProjectSharing(tab, project.ContextBackend{Backend: project.ContextBackendFile, Path: shared})
	require.NoError(t, err)
	assert.Equal(t, project.ContextBackendFile, got.Kind)
	assert.Equal(t, "recipe", got.From)
	assert.Equal(t, shared, got.Location)
	assert.Equal(t, location, got.Workspace)
	assert.Positive(t, events.count("workspace:changed"), "the home refetches the sharing it lists")

	data, err := os.ReadFile(recipe)
	require.NoError(t, err)
	assert.Contains(t, string(data), "context:")
	assert.Contains(t, string(data), "backend: file")
	loaded, err := project.Load(recipe)
	require.NoError(t, err)
	require.NotNil(t, loaded.Context)
	assert.Equal(t, shared, loaded.Context.Path)

	home, err := app.ListWorkspaceProjects()
	require.NoError(t, err)
	require.Len(t, home.Projects, 1)
	require.NotNil(t, home.Projects[0].Sharing)
	assert.Equal(t, project.ContextBackendFile, home.Projects[0].Sharing.Kind)

	got, err = app.SetProjectSharing(tab, project.ContextBackend{Backend: project.ContextBackendLocal})
	require.NoError(t, err)
	assert.Equal(t, project.ContextBackendLocal, got.Kind)
	assert.Equal(t, "default", got.From)
	data, err = os.ReadFile(recipe)
	require.NoError(t, err)
	assert.NotContains(t, string(data), "context:")
}

// TestSetProjectSharing_RefusesWhatTheRecipeWould: an unknown kind, or a kind
// missing what it needs, is refused with the words the recipe check uses, and
// the recipe is left as it was.
func TestSetProjectSharing_RefusesWhatTheRecipeWould(t *testing.T) {
	app := newWorkspaceApp(t)
	tab, recipe := openSharingProject(t, app, "Refuse")
	before, err := os.ReadFile(recipe)
	require.NoError(t, err)

	tests := []struct {
		name string
		spec project.ContextBackend
		want string
	}{
		{"unknown kind", project.ContextBackend{Backend: "dropbox"}, `context.backend: "dropbox" is not a backend; use local, file, git or s3`},
		{"a folder with no path", project.ContextBackend{Backend: project.ContextBackendFile}, "context.path: a file backend names the directory it keeps the context in"},
		{"a bucket with no name", project.ContextBackend{Backend: project.ContextBackendS3}, "context.bucket: an s3 backend names its bucket"},
		{"a branch for a ref", project.ContextBackend{Backend: project.ContextBackendGit, Ref: "refs/heads/main"}, `context.ref: "refs/heads/main" is a branch or a tag; keep the context on a ref outside them, such as refs/kapi/context`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := app.SetProjectSharing(tab, tt.spec)
			require.EqualError(t, err, tt.want)
			after, rerr := os.ReadFile(recipe)
			require.NoError(t, rerr)
			assert.Equal(t, string(before), string(after))
		})
	}
}

// TestProjectSharing_ReadsTheVenue: a recipe connected to a Bowrain server
// names its host beside the place the context is shared.
func TestProjectSharing_ReadsTheVenue(t *testing.T) {
	proj := &project.KapiProject{Extras: map[string]yaml.Node{
		schema.VenueKey: serverExtra(t, "url: https://app.bowrain.cloud/acme/help\n"),
	}}
	got := (&App{}).projectSharingAt(t.Context(), "", proj, "/ws")
	assert.Equal(t, project.ContextBackendLocal, got.Kind)
	assert.Equal(t, "app.bowrain.cloud", got.VenueHost)
	assert.Equal(t, "https://app.bowrain.cloud/acme/help", got.VenueURL)
}

// TestSyncProjectContext_PushesThroughTheNamedPlace: with a folder named,
// a sync pushes what this machine recorded and the home refetches; with none,
// the host's refusal comes back as it is.
func TestSyncProjectContext_PushesThroughTheNamedPlace(t *testing.T) {
	app := newWorkspaceApp(t)
	tab, recipe := openSharingProject(t, app, "Syncing")

	_, err := app.SyncProjectContext(tab)
	require.EqualError(t, err, "there is nowhere to sync with: the recipe declares no context backend. Declare one in kapi.yaml, for example `context: {backend: git}`")

	_, err = app.hostEngine().RecordContextObservation(t.Context(), host.ContextObserveRequest{
		Actor:   contextop.Actor{Kind: contextop.ActorAgent, Name: "claude-code", Session: "sync-test"},
		Project: recipe,
		Term:    "sign in", InsteadOf: []string{"log in"},
	})
	require.NoError(t, err)
	_, err = app.SetProjectSharing(tab, project.ContextBackend{Backend: project.ContextBackendFile, Path: filepath.Join(t.TempDir(), "share")})
	require.NoError(t, err)

	events := recordEvents(app)
	res, err := app.SyncProjectContext(tab)
	require.NoError(t, err)
	require.NotNil(t, res.Push)
	assert.Positive(t, res.Push.Pushed)
	assert.Equal(t, 1, events.count("workspace:changed"))

	view, err := app.ShareView(tab)
	require.NoError(t, err)
	require.NotNil(t, view.Sharing.Status, "a shared backend reports how far apart the two are")
	assert.Zero(t, view.Sharing.Status.ToPush)
}

// recordNote records one observation on a project, as an assistant's `kapi
// context note` does: one operation for a sync to push.
func recordNote(t *testing.T, app *App, recipe string) {
	t.Helper()
	_, err := app.hostEngine().RecordContextObservation(t.Context(), host.ContextObserveRequest{
		Actor:   contextop.Actor{Kind: contextop.ActorAgent, Name: "claude-code", Session: "share-view"},
		Project: recipe,
		Term:    "sign in", InsteadOf: []string{"log in"},
	})
	require.NoError(t, err)
}

// TestShareView_NoPlaceStandsOnThisMac: a recipe that names no place, and a
// tab over the context alone, stand on the first rung with the workspace as
// the only place the context is kept.
func TestShareView_NoPlaceStandsOnThisMac(t *testing.T) {
	app := newWorkspaceApp(t)
	tab, _ := openSharingProject(t, app, "Local")
	location := workspaceRootOf(t, app)

	view, err := app.ShareView(tab)
	require.NoError(t, err)
	assert.Equal(t, host.ShareRungMachine, view.Rung)
	assert.Equal(t, &ProjectSharing{Kind: project.ContextBackendLocal, From: "default", Workspace: location}, view.Sharing)
	assert.Empty(t, view.Error)
	require.NotNil(t, view.Rules)
	assert.Zero(t, *view.Rules)

	home, err := app.ListWorkspaceProjects()
	require.NoError(t, err)
	require.Len(t, home.Projects, 1)
	ctxTab, err := app.OpenWorkspaceContext(home.Projects[0].Key)
	require.NoError(t, err)
	t.Cleanup(func() { app.CloseProject(ctxTab.ID) })
	view, err = app.ShareView(ctxTab.ID)
	require.NoError(t, err)
	assert.Equal(t, host.ShareRungMachine, view.Rung)
	assert.Equal(t, &ProjectSharing{Kind: project.ContextBackendLocal, From: "default", Workspace: location}, view.Sharing)
	assert.Nil(t, view.Rules, "a tab with no checkout counts no rules")
	assert.False(t, view.BowrainPlugin, "nothing connects a project from a tab with no checkout")

	_, err = app.ShareView("no-such-tab")
	require.EqualError(t, err, `project tab "no-such-tab" not found`)
}

// TestShareView_ABrokenPlaceStandsOnThisMac: a `context:` block broken after
// the project opened leaves the page on the first rung with the recipe
// check's words, and naming a place again writes a block that reads.
func TestShareView_ABrokenPlaceStandsOnThisMac(t *testing.T) {
	app := newWorkspaceApp(t)
	tab, recipe := openSharingProject(t, app, "Broken")
	data, err := os.ReadFile(recipe)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(recipe, append(data, []byte("context:\n  backend: dropbox\n")...), 0o644))

	view, err := app.ShareView(tab)
	require.NoError(t, err)
	assert.Equal(t, host.ShareRungMachine, view.Rung)
	assert.Equal(t, project.ContextBackendLocal, view.Sharing.Kind)
	assert.Contains(t, view.Error, `"dropbox" is not a backend`)

	shared := filepath.Join(t.TempDir(), "team-share")
	_, err = app.SetProjectSharing(tab, project.ContextBackend{Backend: project.ContextBackendFile, Path: shared})
	require.NoError(t, err)
	view, err = app.ShareView(tab)
	require.NoError(t, err)
	assert.Empty(t, view.Error)
	assert.Equal(t, host.ShareRungTeam, view.Rung)
}

// TestShareView_KapiMartCountsTheMapsRules: the team rung's rule count is the
// number the Map shows for the same project.
func TestShareView_KapiMartCountsTheMapsRules(t *testing.T) {
	t.Setenv("KAPI_CONFIG_DIR", t.TempDir())
	t.Setenv("KAPI_DATA_DIR", t.TempDir())
	app := NewApp()
	t.Cleanup(func() { app.shutdownEngine() })
	dir := t.TempDir()
	require.NoError(t, sample.Scaffold("kapimart", dir))
	tab, err := app.OpenProject(filepath.Join(dir, project.RecipeFileName))
	require.NoError(t, err)
	t.Cleanup(func() { app.CloseProject(tab.ID) })
	_, err = app.SyncProjectStore(tab.ID)
	require.NoError(t, err)

	summary, err := app.MapSummary(tab.ID)
	require.NoError(t, err)
	view, err := app.ShareView(tab.ID)
	require.NoError(t, err)
	require.NotNil(t, view.Rules)
	assert.Positive(t, summary.Rules)
	assert.Equal(t, summary.Rules, *view.Rules)
}

// TestShareView_ThroughAFolder: a folder written by SetProjectSharing puts the
// project on the team rung, through that folder, with the note this machine
// recorded waiting to push until a sync pushes it.
func TestShareView_ThroughAFolder(t *testing.T) {
	app := newWorkspaceApp(t)
	tab, recipe := openSharingProject(t, app, "Folder")
	shared := filepath.Join(t.TempDir(), "team-share")
	_, err := app.SetProjectSharing(tab, project.ContextBackend{Backend: project.ContextBackendFile, Path: shared})
	require.NoError(t, err)
	recordNote(t, app, recipe)

	view, err := app.ShareView(tab)
	require.NoError(t, err)
	assert.Equal(t, host.ShareRungTeam, view.Rung)
	assert.Equal(t, project.ContextBackendFile, view.Sharing.Kind)
	assert.Equal(t, shared, view.Sharing.Location)
	require.NotNil(t, view.Sharing.Status)
	assert.Equal(t, 1, view.Sharing.Status.ToPush)
	assert.True(t, view.Sharing.Status.Contacted.IsZero(), "never synced from this machine")

	_, err = app.SyncProjectContext(tab)
	require.NoError(t, err)
	view, err = app.ShareView(tab)
	require.NoError(t, err)
	assert.Zero(t, view.Sharing.Status.ToPush)
	assert.Zero(t, view.Sharing.Status.ToPull)
	assert.False(t, view.Sharing.Status.Contacted.IsZero())
}

// runGit runs one git command in dir, with no configuration of the machine's
// own in force.
func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "git %s: %s", strings.Join(args, " "), out)
}

// TestShareView_ThroughGit: a project in a git repository whose remote is a
// bare repository standing in for the hosting service shares through a ref
// of its own, with one change to push after a note and none after a sync.
func TestShareView_ThroughGit(t *testing.T) {
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_SYSTEM", os.DevNull)
	t.Setenv("GIT_AUTHOR_NAME", "t")
	t.Setenv("GIT_AUTHOR_EMAIL", "t@example.com")
	t.Setenv("GIT_COMMITTER_NAME", "t")
	t.Setenv("GIT_COMMITTER_EMAIL", "t@example.com")

	app := newWorkspaceApp(t)
	dir, recipe := scaffoldNamedProject(t, filepath.Join(t.TempDir(), "repo"), project.NewID(), "Git")
	bare := filepath.Join(t.TempDir(), "origin.git")
	runGit(t, dir, "init", "--quiet", "--bare", bare)
	runGit(t, dir, "init", "--quiet")
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "--quiet", "-m", "project")
	runGit(t, dir, "remote", "add", "origin", bare)
	runGit(t, dir, "push", "--quiet", "origin", "HEAD:refs/heads/main")

	tab, err := app.OpenProject(recipe)
	require.NoError(t, err)
	t.Cleanup(func() { app.CloseProject(tab.ID) })
	_, err = app.SetProjectSharing(tab.ID, project.ContextBackend{Backend: project.ContextBackendGit})
	require.NoError(t, err)
	recordNote(t, app, recipe)

	view, err := app.ShareView(tab.ID)
	require.NoError(t, err)
	assert.Equal(t, host.ShareRungTeam, view.Rung)
	assert.Equal(t, project.ContextBackendGit, view.Sharing.Kind)
	assert.Equal(t, "origin refs/kapi/context", view.Sharing.Location)
	require.NotNil(t, view.Sharing.Status)
	assert.Equal(t, 1, view.Sharing.Status.ToPush)

	res, err := app.SyncProjectContext(tab.ID)
	require.NoError(t, err)
	require.NotNil(t, res.Push)
	assert.Equal(t, 1, res.Push.Pushed)
	runGit(t, dir, "--git-dir", bare, "rev-parse", "--verify", "refs/kapi/context")

	view, err = app.ShareView(tab.ID)
	require.NoError(t, err)
	assert.Zero(t, view.Sharing.Status.ToPush)
	assert.False(t, view.Sharing.Status.Contacted.IsZero())
}

// TestShareView_AnUnsavedFolder: a folder opened without a kapi.yaml stands
// on the first rung and nothing is written by reading it; naming a place
// writes the recipe first, and the project then stands on the team rung.
func TestShareView_AnUnsavedFolder(t *testing.T) {
	app := newWorkspaceApp(t)
	dir := contentFolder(t, "unsaved")
	tab, err := app.OpenProject(dir)
	require.NoError(t, err)
	t.Cleanup(func() { app.CloseProject(tab.ID) })
	recipe := filepath.Join(dir, project.RecipeFileName)

	view, err := app.ShareView(tab.ID)
	require.NoError(t, err)
	assert.Equal(t, host.ShareRungMachine, view.Rung)
	assert.Empty(t, view.Error, "a folder with no recipe yet is no error")
	assert.NoFileExists(t, recipe, "reading the Share page writes nothing")

	shared := filepath.Join(t.TempDir(), "team-share")
	_, err = app.SetProjectSharing(tab.ID, project.ContextBackend{Backend: project.ContextBackendFile, Path: shared})
	require.NoError(t, err)
	assert.FileExists(t, recipe)

	view, err = app.ShareView(tab.ID)
	require.NoError(t, err)
	assert.Equal(t, host.ShareRungTeam, view.Rung)
	assert.Equal(t, shared, view.Sharing.Location)
	assert.True(t, tabInfoOf(app.getOpenProject(tab.ID)).RecipeWritten)
}

// TestShareView_ConnectedToBowrain: a recipe with a `bowrain:` block stands on
// the third rung and names the server it connects to.
func TestShareView_ConnectedToBowrain(t *testing.T) {
	app := newWorkspaceApp(t)
	tab, recipe := openSharingProject(t, app, "Bowrain")
	data, err := os.ReadFile(recipe)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(recipe, append(data, []byte(schema.VenueKey+":\n  url: https://app.bowrain.cloud/acme/help\n")...), 0o644))

	view, err := app.ShareView(tab)
	require.NoError(t, err)
	assert.Equal(t, host.ShareRungBowrain, view.Rung)
	assert.Equal(t, "https://app.bowrain.cloud/acme/help", view.Sharing.VenueURL)
	assert.Equal(t, "app.bowrain.cloud", view.Sharing.VenueHost)
}
