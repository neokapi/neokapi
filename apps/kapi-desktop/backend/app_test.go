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
	assert.Empty(t, app.projects)
}

func newTestProject(t *testing.T, app *App, name string) *TabInfo {
	t.Helper()
	path := filepath.Join(t.TempDir(), name, "project.kapi")
	tab, err := app.NewProject("", "en-US", nil, path)
	require.NoError(t, err)
	t.Cleanup(func() { app.CloseProject(tab.ID) })
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
		Version:  "v1",
		Name:     "Roundtrip Test",
		Defaults: project.Defaults{SourceLanguage: "en"},
		Flows: map[string]*flow.StepsSpec{
			"pseudo": {Steps: []flow.FlowStep{{Tool: "pseudo-translate"}}},
		},
	}
	require.NoError(t, project.Save(path, proj))

	tab, err := app.OpenProject(path)
	require.NoError(t, err)
	t.Cleanup(func() { app.CloseProject(tab.ID) })
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

func TestListToolsExposesSourceTransform(t *testing.T) {
	app := NewApp()
	byName := map[string]ToolInfo{}
	for _, ti := range app.ListTools() {
		byName[ti.Name] = ti
	}
	if redact, ok := byName["redact"]; ok {
		assert.True(t, redact.IsSourceTransform, "redact should be source-transform-capable")
	}
	if wc, ok := byName["word-count"]; ok {
		assert.False(t, wc.IsSourceTransform, "word-count should not be source-transform-capable")
	}
}

func TestFlowSourceTransformsRoundTrip(t *testing.T) {
	app := NewApp()
	tab := newTestProject(t, app, "ST")

	spec := &flow.StepsSpec{
		SourceTransforms: []flow.FlowStep{{Tool: "redact"}},
		Steps:            []flow.FlowStep{{Tool: "ai-translate"}},
	}
	require.NoError(t, app.SaveFlow(tab.ID, "secure", spec))

	got := app.GetFlow(tab.ID, "secure")
	require.NotNil(t, got)
	require.Len(t, got.SourceTransforms, 1)
	assert.Equal(t, "redact", got.SourceTransforms[0].Tool)
	require.Len(t, got.Steps, 1)
	assert.Equal(t, "ai-translate", got.Steps[0].Tool)
}

func TestListFormats(t *testing.T) {
	app := NewApp()
	assert.NotEmpty(t, app.ListFormats())
}

func TestListProjectFormats_NoProject(t *testing.T) {
	app := NewApp()
	// No open project — should return all formats.
	formats := app.ListProjectFormats("nonexistent")
	assert.NotEmpty(t, formats)
	assert.Equal(t, len(app.ListFormats()), len(formats))
}

func TestListProjectTools_NoProject(t *testing.T) {
	app := NewApp()
	// No open project — should return all tools.
	tools := app.ListProjectTools("nonexistent")
	assert.NotEmpty(t, tools)
	assert.Equal(t, len(app.ListTools()), len(tools))
}

func TestListFlows_ValidatesTools(t *testing.T) {
	app := NewApp()
	tab := newTestProject(t, app, "FlowValidation")

	// Add a flow with a valid tool and an invalid tool.
	_ = app.SaveFlow(tab.ID, "good-flow", &flow.StepsSpec{
		Steps: []flow.FlowStep{{Tool: "pseudo-translate"}},
	})
	_ = app.SaveFlow(tab.ID, "bad-flow", &flow.StepsSpec{
		Steps: []flow.FlowStep{{Tool: "nonexistent-magic-tool"}},
	})

	flows := app.ListFlows(tab.ID)
	require.Len(t, flows, 2)

	// Find each flow.
	var good, bad *FlowInfo
	for i := range flows {
		switch flows[i].Name {
		case "good-flow":
			good = &flows[i]
		case "bad-flow":
			bad = &flows[i]
		}
	}

	require.NotNil(t, good)
	assert.True(t, good.Valid)
	assert.Empty(t, good.Issues)

	require.NotNil(t, bad)
	assert.False(t, bad.Valid)
	require.Len(t, bad.Issues, 1)
	assert.Equal(t, "unknown", bad.Issues[0].Type)
	assert.Equal(t, "nonexistent-magic-tool", bad.Issues[0].Tool)
}

func TestDetectProjectFormat_NoProject(t *testing.T) {
	app := NewApp()
	// No open project — should use global detection.
	fmt := app.DetectProjectFormat("nonexistent", "test.json")
	assert.Equal(t, "json", fmt)
}

func TestGetPluginDocsNil(t *testing.T) {
	app := NewApp()
	// No plugins installed — docs should be nil.
	result := app.GetPluginDocs()
	assert.Nil(t, result)
}

func TestGetFilterDocNil(t *testing.T) {
	app := NewApp()
	// No plugins installed — individual filter doc should be nil.
	result := app.GetFilterDoc("okf_json")
	assert.Nil(t, result)
}

func TestGetStepDocNil(t *testing.T) {
	app := NewApp()
	// No plugins installed — individual step doc should be nil.
	result := app.GetStepDoc("batch-translation")
	assert.Nil(t, result)
}

func TestGetVersion(t *testing.T) {
	app := NewApp()
	assert.NotEmpty(t, app.GetVersion())
}
