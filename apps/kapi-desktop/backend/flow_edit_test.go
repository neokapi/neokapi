package backend

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/project"
)

// reopenProject opens a fresh App on the recipe at root and returns the loaded
// project, so a test can assert what actually reached disk.
func reopenProject(t *testing.T, root string) *project.KapiProject {
	t.Helper()
	fresh := NewApp()
	tab, err := fresh.OpenProject(filepath.Join(root, "kapi.yaml"))
	require.NoError(t, err)
	t.Cleanup(func() { fresh.CloseProject(tab.ID) })
	op := fresh.projects[tab.ID]
	require.NotNil(t, op)
	require.NotNil(t, op.Project)
	return op.Project
}

func TestSaveProjectFlow_PersistsToRecipe(t *testing.T) {
	app := NewApp()
	tab, root := newContextProject(t, app)

	spec := &flow.StepsSpec{Steps: []flow.FlowStep{
		{Tool: "recycle"},
		{Tool: "translate", Config: map[string]any{"provider": "anthropic"}},
	}}
	require.NoError(t, app.SaveProjectFlow(tab.ID, "translate-and-qa", spec))

	got := reopenProject(t, root).Flows["translate-and-qa"]
	require.NotNil(t, got)
	require.Len(t, got.Steps, 2)
	assert.Equal(t, "recycle", got.Steps[0].Tool)
	assert.Equal(t, "translate", got.Steps[1].Tool)
	assert.Equal(t, "anthropic", got.Steps[1].Config["provider"])
}

func TestSetDefaultFlow_WritesAndClears(t *testing.T) {
	app := NewApp()
	tab, root := newContextProject(t, app)
	require.NoError(t, app.SaveProjectFlow(tab.ID, "convert",
		&flow.StepsSpec{Steps: []flow.FlowStep{{Tool: "word-count"}}}))

	require.NoError(t, app.SetDefaultFlow(tab.ID, "convert"))
	assert.Equal(t, "convert", reopenProject(t, root).Defaults.Flow)

	require.NoError(t, app.SetDefaultFlow(tab.ID, ""))
	assert.Empty(t, reopenProject(t, root).Defaults.Flow)

	// An unknown flow is refused.
	require.Error(t, app.SetDefaultFlow(tab.ID, "nope"))
}

func TestRenameProjectFlow_MovesDefaultAndRefusesClash(t *testing.T) {
	app := NewApp()
	tab, root := newContextProject(t, app)
	require.NoError(t, app.SaveProjectFlow(tab.ID, "a",
		&flow.StepsSpec{Steps: []flow.FlowStep{{Tool: "word-count"}}}))
	require.NoError(t, app.SaveProjectFlow(tab.ID, "b",
		&flow.StepsSpec{Steps: []flow.FlowStep{{Tool: "qa"}}}))
	require.NoError(t, app.SetDefaultFlow(tab.ID, "a"))

	require.NoError(t, app.RenameProjectFlow(tab.ID, "a", "a2"))
	proj := reopenProject(t, root)
	assert.Nil(t, proj.Flows["a"])
	require.NotNil(t, proj.Flows["a2"])
	assert.Equal(t, "a2", proj.Defaults.Flow, "the default moves with the rename")

	// Renaming onto an existing name is refused.
	require.Error(t, app.RenameProjectFlow(tab.ID, "b", "a2"))
	// Renaming an unknown source is refused.
	require.Error(t, app.RenameProjectFlow(tab.ID, "ghost", "x"))
}
