package brandscope

import (
	"context"
	"errors"
	"testing"

	"github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/core/profile"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeScopeStore serves fixed project/stream/collection records.
type fakeScopeStore struct {
	project    *store.Project
	stream     *store.Stream
	collection *store.Collection
}

func (f *fakeScopeStore) GetProject(_ context.Context, _ string) (*store.Project, error) {
	if f.project == nil {
		return nil, errors.New("not found")
	}
	return f.project, nil
}

func (f *fakeScopeStore) GetStream(_ context.Context, _, _ string) (*store.Stream, error) {
	if f.stream == nil {
		return nil, errors.New("not found")
	}
	return f.stream, nil
}

func (f *fakeScopeStore) GetCollection(_ context.Context, _, _ string) (*store.Collection, error) {
	if f.collection == nil {
		return nil, errors.New("not found")
	}
	return f.collection, nil
}

// fakeWorkspaceDefault returns a fixed workspace default profile ID.
type fakeWorkspaceDefault struct{ id string }

func (f fakeWorkspaceDefault) WorkspaceBrandProfileID(_ context.Context, _ string) (string, error) {
	return f.id, nil
}

// fakeBrandStore resolves profiles by ID from a fixed set.
type fakeBrandStore struct {
	profile.Store // embed so we only implement GetProfile
	profiles      map[string]*profile.VoiceProfile
}

func (f *fakeBrandStore) GetProfile(_ context.Context, id string) (*profile.VoiceProfile, error) {
	p, ok := f.profiles[id]
	if !ok {
		return nil, errors.New("profile not found: " + id)
	}
	return p, nil
}

func newBrandStore() *fakeBrandStore {
	return &fakeBrandStore{profiles: map[string]*profile.VoiceProfile{
		"explicit":  {ID: "explicit", Name: "Explicit"},
		"project":   {ID: "project", Name: "Project"},
		"stream":    {ID: "stream", Name: "Stream"},
		"workspace": {ID: "workspace", Name: "Workspace"},
	}}
}

func TestResolve_ExplicitWins(t *testing.T) {
	cs := &fakeScopeStore{
		project: &store.Project{Properties: map[string]string{profile.PropertyProfileID: "project"}},
	}
	got, err := Resolve(t.Context(), cs, fakeWorkspaceDefault{id: "workspace"}, newBrandStore(), Scope{
		ExplicitProfileID: "explicit",
		WorkspaceID:       "ws1",
		ProjectID:         "p1",
	})
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "explicit", got.ID, "explicit profile is the top of the ladder")
}

func TestResolve_ProjectBeatsWorkspaceDefault(t *testing.T) {
	cs := &fakeScopeStore{
		project: &store.Project{
			WorkspaceID: "ws1",
			Properties:  map[string]string{profile.PropertyProfileID: "project"},
		},
	}
	got, err := Resolve(t.Context(), cs, fakeWorkspaceDefault{id: "workspace"}, newBrandStore(), Scope{
		ProjectID: "p1",
	})
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "project", got.ID, "a project binding wins over the workspace default")
}

func TestResolve_StreamBeatsProject(t *testing.T) {
	cs := &fakeScopeStore{
		project: &store.Project{Properties: map[string]string{profile.PropertyProfileID: "project"}},
		stream:  &store.Stream{Properties: map[string]string{profile.PropertyProfileID: "stream"}},
	}
	got, err := Resolve(t.Context(), cs, nil, newBrandStore(), Scope{
		ProjectID: "p1",
		Stream:    "v2",
	})
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "stream", got.ID, "a stream binding wins over the project binding")
}

func TestResolve_UnsetFallsThroughToWorkspaceDefault(t *testing.T) {
	cs := &fakeScopeStore{
		project: &store.Project{WorkspaceID: "ws1"}, // no brand binding
	}
	got, err := Resolve(t.Context(), cs, fakeWorkspaceDefault{id: "workspace"}, newBrandStore(), Scope{
		ProjectID: "p1",
	})
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "workspace", got.ID, "with no scope binding, the workspace default is the base")
}

func TestResolve_WorkspaceDefaultFromProjectWorkspaceID(t *testing.T) {
	// Scope carries no WorkspaceID; it must be derived from the project so the
	// workspace default still applies (the MCP path, which only knows a project).
	cs := &fakeScopeStore{
		project: &store.Project{WorkspaceID: "ws-derived"},
	}
	var seen string
	wd := workspaceDefaultFunc(func(_ context.Context, id string) (string, error) {
		seen = id
		return "workspace", nil
	})
	got, err := Resolve(t.Context(), cs, wd, newBrandStore(), Scope{ProjectID: "p1"})
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "workspace", got.ID)
	assert.Equal(t, "ws-derived", seen, "workspace default is looked up by the project's WorkspaceID")
}

func TestResolve_NothingBound(t *testing.T) {
	cs := &fakeScopeStore{project: &store.Project{}}
	got, err := Resolve(t.Context(), cs, fakeWorkspaceDefault{id: ""}, newBrandStore(), Scope{
		ProjectID: "p1",
	})
	require.NoError(t, err)
	assert.Nil(t, got, "no profile bound anywhere resolves to nil, the current no-profile behavior")
}

// workspaceDefaultFunc adapts a func to the WorkspaceDefault interface.
type workspaceDefaultFunc func(ctx context.Context, workspaceID string) (string, error)

func (f workspaceDefaultFunc) WorkspaceBrandProfileID(ctx context.Context, workspaceID string) (string, error) {
	return f(ctx, workspaceID)
}
