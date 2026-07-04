package backend

import (
	"testing"

	"github.com/neokapi/neokapi/core/project"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// serverExtra builds the yaml.Node the framework stores under a recipe's
// unknown `server:` key (the bowrain schema extension), so the venue binding
// can be exercised without a full project load.
func serverExtra(t *testing.T, body string) yaml.Node {
	t.Helper()
	var doc yaml.Node
	require.NoError(t, yaml.Unmarshal([]byte(body), &doc))
	require.NotEmpty(t, doc.Content, "expected a mapping node")
	return *doc.Content[0]
}

func venueApp(proj *project.KapiProject) *App {
	return &App{projects: map[string]*openProject{"t": {ID: "t", Path: "proj.kapi", Project: proj}}}
}

func TestGetProjectServer_Connected(t *testing.T) {
	proj := &project.KapiProject{
		Extras: map[string]yaml.Node{
			"server": serverExtra(t, "url: https://bowrain.example.com/my-team/abc123\nstream: main\n"),
		},
	}
	got, err := venueApp(proj).GetProjectServer("t")
	require.NoError(t, err)
	assert.True(t, got.Connected)
	assert.Equal(t, "https://bowrain.example.com/my-team/abc123", got.URL)
	assert.Equal(t, "bowrain.example.com", got.Host)
	assert.Equal(t, "https://bowrain.example.com", got.ServerURL)
}

func TestGetProjectServer_DirectProjectURL(t *testing.T) {
	proj := &project.KapiProject{
		Extras: map[string]yaml.Node{
			"server": serverExtra(t, "url: https://bowrain.example.com/projects/abc123\n"),
		},
	}
	got, err := venueApp(proj).GetProjectServer("t")
	require.NoError(t, err)
	assert.True(t, got.Connected)
	assert.Equal(t, "bowrain.example.com", got.Host)
}

func TestGetProjectServer_NoServerBlock(t *testing.T) {
	got, err := venueApp(&project.KapiProject{}).GetProjectServer("t")
	require.NoError(t, err)
	assert.False(t, got.Connected)
	assert.Empty(t, got.Host)
}

func TestGetProjectServer_EmptyURL(t *testing.T) {
	proj := &project.KapiProject{
		Extras: map[string]yaml.Node{"server": serverExtra(t, "stream: main\n")},
	}
	got, err := venueApp(proj).GetProjectServer("t")
	require.NoError(t, err)
	assert.False(t, got.Connected)
}

func TestGetProjectServer_NoProjectLoaded(t *testing.T) {
	app := &App{projects: map[string]*openProject{"t": {ID: "t"}}}
	got, err := app.GetProjectServer("t")
	require.NoError(t, err)
	assert.False(t, got.Connected)
}

func TestGetProjectServer_TabNotFound(t *testing.T) {
	_, err := venueApp(&project.KapiProject{}).GetProjectServer("missing")
	require.Error(t, err)
}
