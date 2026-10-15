package backend

import (
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/contextop"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/host"
	"github.com/neokapi/neokapi/host/venue/schema"
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

	got, err := app.ProjectSharing(tab)
	require.NoError(t, err)
	assert.Equal(t, &ProjectSharing{Kind: project.ContextBackendLocal, From: "default", Workspace: location}, got)

	shared := filepath.Join(t.TempDir(), "team-share")
	got, err = app.SetProjectSharing(tab, project.ContextBackend{Backend: project.ContextBackendFile, Path: shared})
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

	sharing, err := app.ProjectSharing(tab)
	require.NoError(t, err)
	require.NotNil(t, sharing.Status, "a shared backend reports how far apart the two are")
	assert.Zero(t, sharing.Status.ToPush)
}
