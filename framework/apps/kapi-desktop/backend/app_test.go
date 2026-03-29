package backend

import (
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

func TestNewProject(t *testing.T) {
	app := NewApp()

	tab, err := app.NewProject("Test", "en-US", []string{"fr-FR", "de-DE"})
	require.NoError(t, err)
	require.NotNil(t, tab)
	assert.NotEmpty(t, tab.ID)
	assert.Equal(t, "Test", tab.Name)
	assert.Empty(t, tab.Path)

	proj := app.GetProject(tab.ID)
	require.NotNil(t, proj)
	assert.Equal(t, "Test", proj.Name)
	assert.Equal(t, "en-US", proj.SourceLanguage)
}

func TestOpenSaveProject(t *testing.T) {
	app := NewApp()
	dir := t.TempDir()
	path := dir + "/test.kapi"

	// Save a project file first.
	proj := &project.KapiProject{
		Version:        "v1",
		Name:           "Roundtrip Test",
		SourceLanguage: "en",
		Flows: map[string]*flow.StepsSpec{
			"pseudo": {Steps: []flow.FlowStep{{Tool: "pseudo-translate"}}},
		},
	}
	require.NoError(t, project.Save(path, proj))

	// Open it.
	tab, err := app.OpenProject(path)
	require.NoError(t, err)
	assert.Equal(t, "Roundtrip Test", tab.Name)
	assert.Equal(t, path, tab.Path)

	// Modify and save.
	loaded := app.GetProject(tab.ID)
	loaded.Name = "Updated"
	require.NoError(t, app.SaveProject(tab.ID))

	// Reopen to verify.
	tab2, err := app.OpenProject(path)
	require.NoError(t, err)
	// Should return same tab since the file is already open.
	assert.Equal(t, tab.ID, tab2.ID)
}

func TestOpenProjectDeduplicate(t *testing.T) {
	app := NewApp()
	dir := t.TempDir()
	path := dir + "/dup.kapi"
	require.NoError(t, project.Save(path, &project.KapiProject{Version: "v1", Name: "Dup"}))

	tab1, _ := app.OpenProject(path)
	tab2, _ := app.OpenProject(path) // same file
	assert.Equal(t, tab1.ID, tab2.ID, "should return same tab for same file")
}

func TestSaveProjectAs(t *testing.T) {
	app := NewApp()
	dir := t.TempDir()

	tab, err := app.NewProject("SaveAs Test", "en", nil)
	require.NoError(t, err)

	path := dir + "/saveas.kapi"
	require.NoError(t, app.SaveProjectAs(tab.ID, path))
	assert.Equal(t, path, app.GetProjectPath(tab.ID))
}

func TestCloseProject(t *testing.T) {
	app := NewApp()

	tab, _ := app.NewProject("ToClose", "en", nil)
	assert.Len(t, app.ListTabs(), 1)

	app.CloseProject(tab.ID)
	assert.Empty(t, app.ListTabs())
	assert.Nil(t, app.GetProject(tab.ID))
}

func TestListTabs(t *testing.T) {
	app := NewApp()

	app.NewProject("A", "en", nil)
	app.NewProject("B", "fr", nil)
	app.NewProject("C", "de", nil)

	tabs := app.ListTabs()
	assert.Len(t, tabs, 3)
}

func TestSaveProjectNoPath(t *testing.T) {
	app := NewApp()
	tab, _ := app.NewProject("NoPath", "en", nil)
	assert.Error(t, app.SaveProject(tab.ID))
}

func TestSaveProjectBadTab(t *testing.T) {
	app := NewApp()
	assert.Error(t, app.SaveProject("nonexistent"))
}

func TestFlowOperations(t *testing.T) {
	app := NewApp()
	tab, _ := app.NewProject("Flows", "en", nil)

	assert.Empty(t, app.ListFlows(tab.ID))

	spec := &flow.StepsSpec{
		Steps: []flow.FlowStep{{Tool: "qa-check"}},
	}
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
	tools := app.ListTools()
	assert.NotEmpty(t, tools)
}

func TestListFormats(t *testing.T) {
	app := NewApp()
	fmts := app.ListFormats()
	assert.NotEmpty(t, fmts)
}

func TestGetVersion(t *testing.T) {
	app := NewApp()
	assert.NotEmpty(t, app.GetVersion())
}
