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
	assert.Nil(t, app.project)
}

func TestNewProject(t *testing.T) {
	app := NewApp()

	proj, err := app.NewProject("Test", "en-US", []string{"fr-FR", "de-DE"})
	require.NoError(t, err)
	require.NotNil(t, proj)

	assert.Equal(t, "Test", proj.Name)
	assert.Equal(t, "en-US", proj.SourceLanguage)
	assert.Equal(t, []string{"fr-FR", "de-DE"}, proj.TargetLanguages)
	assert.NotNil(t, proj.Flows)

	// Should be set as the current project.
	assert.Equal(t, proj, app.GetProject())
	assert.Equal(t, "", app.GetProjectPath())
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
	loaded, err := app.OpenProject(path)
	require.NoError(t, err)
	assert.Equal(t, "Roundtrip Test", loaded.Name)
	assert.Equal(t, path, app.GetProjectPath())

	// Modify and save.
	loaded.Name = "Updated"
	require.NoError(t, app.SaveProject())

	// Reopen to verify.
	reloaded, err := app.OpenProject(path)
	require.NoError(t, err)
	assert.Equal(t, "Updated", reloaded.Name)
}

func TestSaveProjectAs(t *testing.T) {
	app := NewApp()
	dir := t.TempDir()

	_, err := app.NewProject("SaveAs Test", "en", nil)
	require.NoError(t, err)

	path := dir + "/saveas.kapi"
	require.NoError(t, app.SaveProjectAs(path))
	assert.Equal(t, path, app.GetProjectPath())

	// Verify file was written.
	loaded, err := project.Load(path)
	require.NoError(t, err)
	assert.Equal(t, "SaveAs Test", loaded.Name)
}

func TestSaveProjectNoProject(t *testing.T) {
	app := NewApp()
	assert.Error(t, app.SaveProject())
}

func TestSaveProjectNoPath(t *testing.T) {
	app := NewApp()
	_, _ = app.NewProject("NoPath", "en", nil)
	assert.Error(t, app.SaveProject()) // no path set
}

func TestFlowOperations(t *testing.T) {
	app := NewApp()
	_, _ = app.NewProject("Flows", "en", nil)

	// Initially empty.
	assert.Empty(t, app.ListFlows())

	// Save a flow.
	spec := &flow.StepsSpec{
		Steps: []flow.FlowStep{{Tool: "qa-check"}},
	}
	require.NoError(t, app.SaveFlow("qa", spec))

	flows := app.ListFlows()
	assert.Len(t, flows, 1)
	assert.Equal(t, "qa", flows[0].Name)
	assert.Equal(t, 1, flows[0].StepCount)

	// Get flow.
	got := app.GetFlow("qa")
	require.NotNil(t, got)
	assert.Equal(t, "qa-check", got.Steps[0].Tool)

	// Delete flow.
	require.NoError(t, app.DeleteFlow("qa"))
	assert.Empty(t, app.ListFlows())
	assert.Nil(t, app.GetFlow("qa"))
}

func TestFlowOperationsNoProject(t *testing.T) {
	app := NewApp()

	assert.Nil(t, app.ListFlows())
	assert.Nil(t, app.GetFlow("anything"))
	assert.Error(t, app.SaveFlow("test", &flow.StepsSpec{}))
	assert.Error(t, app.DeleteFlow("test"))
}

func TestListTools(t *testing.T) {
	app := NewApp()
	tools := app.ListTools()
	assert.NotEmpty(t, tools, "should have built-in tools registered")

	// Verify a known tool exists.
	found := false
	for _, ti := range tools {
		if ti.Name == "pseudo-translate" {
			found = true
			break
		}
	}
	assert.True(t, found, "pseudo-translate should be registered")
}

func TestListFormats(t *testing.T) {
	app := NewApp()
	fmts := app.ListFormats()
	assert.NotEmpty(t, fmts, "should have built-in formats registered")

	// Verify a known format exists.
	found := false
	for _, f := range fmts {
		if f.Name == "json" {
			found = true
			break
		}
	}
	assert.True(t, found, "json format should be registered")
}

func TestGetVersion(t *testing.T) {
	app := NewApp()
	v := app.GetVersion()
	assert.NotEmpty(t, v)
}
