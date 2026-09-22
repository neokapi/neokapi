package backend

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/host"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newCheckoutWithContextFiles writes a project whose checkout carries a voice
// profile and a terms bundle, the shape a clone of a project set up before the
// store arrives in.
func newCheckoutWithContextFiles(t *testing.T, dir, name string) string {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, project.StateDirName), 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, project.RelStatePath("voice.yaml")),
		[]byte("name: Northsea\ntone:\n  formality: neutral\n"), 0o644))
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, project.RelStatePath("terms.json")),
		[]byte(`{"schemaVersion":"1.0","kind":"kapi-terms","concepts":[]}`), 0o644))

	path := filepath.Join(dir, project.RecipeFileName)
	require.NoError(t, project.Save(path, &project.KapiProject{
		Version: project.CurrentVersion,
		Name:    name,
		Defaults: project.Defaults{
			SourceLanguage: "en-US",
			Voice:          &project.VoiceBinding{ProfileFile: project.RelStatePath("voice.yaml")},
			TermsSource:    project.RelStatePath("terms.json"),
		},
		Collections: []project.Collection{{
			Name:    "Docs",
			Content: []project.ContentItem{{Path: "docs/*.md", Target: "i18n/{lang}/docs/*.md"}},
		}},
	}))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "docs"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "docs", "index.md"), []byte("# Berths\n"), 0o644))
	return path
}

func TestProjectStatusNamesTheContextFilesNobodyHasRead(t *testing.T) {
	app := newWorkspaceApp(t)
	path := newCheckoutWithContextFiles(t, t.TempDir(), "northsea")

	tab, err := app.OpenProject(path)
	require.NoError(t, err)
	t.Cleanup(func() { app.CloseProject(tab.ID) })

	status, err := app.GetProjectStatus(tab.ID)
	require.NoError(t, err)
	require.NotNil(t, status.ContextFiles)
	assert.Equal(t, ".kapi/terms.json", status.ContextFiles.Files[0])
	assert.Contains(t, status.ContextFiles.Files, ".kapi/voice.yaml")
	assert.Equal(t, "kapi context import", status.ContextFiles.Command)
	assert.Contains(t, status.ContextFiles.Message, "kapi context import")
}

func TestProjectStatusSaysNothingOnceTheContextIsRead(t *testing.T) {
	app := newWorkspaceApp(t)
	root := t.TempDir()
	path := newCheckoutWithContextFiles(t, root, "northsea-read")
	readContextInto(t, app, path)

	tab, err := app.OpenProject(path)
	require.NoError(t, err)
	t.Cleanup(func() { app.CloseProject(tab.ID) })

	status, err := app.GetProjectStatus(tab.ID)
	require.NoError(t, err)
	assert.Nil(t, status.ContextFiles)
}

// A project carrying no context file has nothing to say either, which is every
// project scaffolded since the store became the only place context lives.
func TestProjectStatusSaysNothingWithoutContextFiles(t *testing.T) {
	app := newWorkspaceApp(t)
	root := t.TempDir()
	path := filepath.Join(root, project.RecipeFileName)
	require.NoError(t, project.Save(path, &project.KapiProject{
		Version:  project.CurrentVersion,
		Name:     "Bare",
		Defaults: project.Defaults{SourceLanguage: "en-US", TargetLanguages: []model.LocaleID{"nb-NO"}},
	}))

	tab, err := app.OpenProject(path)
	require.NoError(t, err)
	t.Cleanup(func() { app.CloseProject(tab.ID) })

	status, err := app.GetProjectStatus(tab.ID)
	require.NoError(t, err)
	assert.Nil(t, status.ContextFiles)
}

func TestWorkspaceHomeNamesTheContextFilesNobodyHasRead(t *testing.T) {
	app := newWorkspaceApp(t)
	path := newCheckoutWithContextFiles(t, filepath.Join(t.TempDir(), "northsea"), "northsea-home")

	tab, err := app.OpenProject(path)
	require.NoError(t, err)
	t.Cleanup(func() { app.CloseProject(tab.ID) })

	home, err := app.ListWorkspaceProjects()
	require.NoError(t, err)
	row := workspaceRowOf(t, home, "northsea-home")
	require.NotNil(t, row.ContextFiles)
	assert.Contains(t, row.ContextFiles.Files, ".kapi/voice.yaml")
	assert.Equal(t, "kapi context import", row.ContextFiles.Command)
}

func TestWorkspaceHomeSaysNothingOnceTheContextIsRead(t *testing.T) {
	app := newWorkspaceApp(t)
	path := newCheckoutWithContextFiles(t, filepath.Join(t.TempDir(), "tidewatch"), "tidewatch-home")
	readContextInto(t, app, path)

	tab, err := app.OpenProject(path)
	require.NoError(t, err)
	t.Cleanup(func() { app.CloseProject(tab.ID) })

	home, err := app.ListWorkspaceProjects()
	require.NoError(t, err)
	assert.Nil(t, workspaceRowOf(t, home, "tidewatch-home").ContextFiles)
}

// readContextInto reads a checkout's context files into the workspace this app
// keeps its projects in, which is what a person running `kapi context import`
// does for the store the app then answers from.
func readContextInto(t *testing.T, app *App, recipe string) {
	t.Helper()
	_, err := app.hostEngine().ImportProjectContext(
		context.Background(), recipe, host.ContextImportRequest{})
	require.NoError(t, err)
}

// workspaceRowOf finds one project on the home screen by key.
func workspaceRowOf(t *testing.T, home *WorkspaceHome, key string) WorkspaceProject {
	t.Helper()
	for _, p := range home.Projects {
		if p.Key == key {
			return p
		}
	}
	t.Fatalf("the home screen lists no project %q", key)
	return WorkspaceProject{}
}
