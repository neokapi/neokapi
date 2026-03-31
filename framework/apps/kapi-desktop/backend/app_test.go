package backend

import (
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/project"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewApp(t *testing.T) {
	app := NewApp()
	require.NotNil(t, app)
	assert.NotNil(t, app.formatReg)
	assert.NotNil(t, app.toolReg)
	assert.NotNil(t, app.schemaReg)
	assert.NotNil(t, app.pluginLoader)
	assert.Empty(t, app.projects)
}

func newTestProject(t *testing.T, app *App, name string) *TabInfo {
	t.Helper()
	path := filepath.Join(t.TempDir(), name, "project.kapi")
	tab, err := app.NewProject("", "en-US", nil, path)
	require.NoError(t, err)
	return tab
}

func TestNewProject(t *testing.T) {
	app := NewApp()
	dir := filepath.Join(t.TempDir(), "TestProject")
	path := filepath.Join(dir, "project.kapi")
	tab, err := app.NewProject("", "en-US", nil, path)
	require.NoError(t, err)

	assert.NotEmpty(t, tab.ID)
	assert.Equal(t, "TestProject", tab.Name, "display name should be derived from folder")
	assert.NotEmpty(t, tab.Path)

	proj := app.GetProject(tab.ID)
	require.NotNil(t, proj)
	assert.Empty(t, proj.Name, "YAML name should be empty")
}

func TestNewProjectDefaultPath(t *testing.T) {
	app := NewApp()
	tab, err := app.NewProject("MyApp", "en", nil, "")
	require.NoError(t, err)
	assert.Contains(t, tab.Path, filepath.Join("KapiProjects", "MyApp", "project.kapi"))
	assert.Equal(t, "MyApp", tab.Name)
}

func TestNewProjectRequiresNameOrPath(t *testing.T) {
	app := NewApp()
	_, err := app.NewProject("", "en", nil, "")
	assert.Error(t, err)
}

func TestOpenSaveProject(t *testing.T) {
	app := NewApp()
	dir := t.TempDir()
	path := dir + "/test.kapi"

	proj := &project.KapiProject{
		Version:        "v1",
		Name:           "Roundtrip Test",
		SourceLanguage: "en",
		Flows: map[string]*flow.StepsSpec{
			"pseudo": {Steps: []flow.FlowStep{{Tool: "pseudo-translate"}}},
		},
	}
	require.NoError(t, project.Save(path, proj))

	tab, err := app.OpenProject(path)
	require.NoError(t, err)
	assert.Equal(t, "Roundtrip Test", tab.Name)

	loaded := app.GetProject(tab.ID)
	loaded.Name = "Updated"
	require.NoError(t, app.SaveProject(tab.ID))

	tab2, err := app.OpenProject(path)
	require.NoError(t, err)
	assert.Equal(t, tab.ID, tab2.ID)
}

func TestSaveProjectAs(t *testing.T) {
	app := NewApp()
	tab := newTestProject(t, app, "SaveAs")

	newPath := filepath.Join(t.TempDir(), "saveas.kapi")
	require.NoError(t, app.SaveProjectAs(tab.ID, newPath))
	assert.Equal(t, newPath, app.GetProjectPath(tab.ID))
}

func TestCloseProject(t *testing.T) {
	app := NewApp()
	tab := newTestProject(t, app, "ToClose")
	assert.Len(t, app.ListTabs(), 1)

	app.CloseProject(tab.ID)
	assert.Empty(t, app.ListTabs())
}

func TestListTabs(t *testing.T) {
	app := NewApp()
	newTestProject(t, app, "A")
	newTestProject(t, app, "B")
	newTestProject(t, app, "C")
	assert.Len(t, app.ListTabs(), 3)
}

func TestFlowOperations(t *testing.T) {
	app := NewApp()
	tab := newTestProject(t, app, "Flows")

	assert.Empty(t, app.ListFlows(tab.ID))

	spec := &flow.StepsSpec{Steps: []flow.FlowStep{{Tool: "qa-check"}}}
	require.NoError(t, app.SaveFlow(tab.ID, "qa", spec))

	flows := app.ListFlows(tab.ID)
	assert.Len(t, flows, 1)
	assert.Equal(t, "qa", flows[0].Name)

	got := app.GetFlow(tab.ID, "qa")
	require.NotNil(t, got)
	assert.Equal(t, "qa-check", got.Steps[0].Tool)

	require.NoError(t, app.DeleteFlow(tab.ID, "qa"))
	assert.Empty(t, app.ListFlows(tab.ID))
}

func TestFlowOperationsBadTab(t *testing.T) {
	app := NewApp()
	assert.Nil(t, app.ListFlows("bad"))
	assert.Nil(t, app.GetFlow("bad", "x"))
	assert.Error(t, app.SaveFlow("bad", "x", &flow.StepsSpec{}))
	assert.Error(t, app.DeleteFlow("bad", "x"))
}

func TestListTools(t *testing.T) {
	app := NewApp()
	assert.NotEmpty(t, app.ListTools())
}

func TestListFormats(t *testing.T) {
	app := NewApp()
	assert.NotEmpty(t, app.ListFormats())
}

func TestGetVersion(t *testing.T) {
	app := NewApp()
	assert.NotEmpty(t, app.GetVersion())
}
