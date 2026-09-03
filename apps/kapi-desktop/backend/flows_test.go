package backend

import (
	"testing"

	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/host/flowdef"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// isolateConfig points KAPI_CONFIG_DIR + KAPI_DESKTOP_CONFIG_DIR at temp dirs
// so user-flow / settings writes never touch the developer's real config.
func isolateConfig(t *testing.T) {
	t.Helper()
	t.Setenv("KAPI_CONFIG_DIR", t.TempDir())
	t.Setenv("KAPI_DESKTOP_CONFIG_DIR", t.TempDir())
}

func TestFlowStepLabels(t *testing.T) {
	got := flowStepLabels([]flow.FlowStep{
		{Tool: "recycle"},
		{Tool: "translate", Label: "Translate"},
		{Parallel: []flow.FlowStep{{Tool: "qa"}}},
	})
	assert.Equal(t, []string{"recycle", "Translate", "qa"}, got)
}

func TestListUserFlows_StepChips(t *testing.T) {
	isolateConfig(t)
	app := NewApp()
	flows := app.ListUserFlows()
	require.NotEmpty(t, flows)

	named := false
	for _, f := range flows {
		// One chip per step.
		assert.Len(t, f.Steps, f.StepCount, "flow %q", f.Name)
		if len(f.Steps) > 0 {
			named = true
		}
	}
	assert.True(t, named, "at least one built-in flow names its steps")
}

func TestListFlows_StepsAndDefault(t *testing.T) {
	app := NewApp()
	tab := newTestProject(t, app, "FlowSteps")
	require.NoError(t, app.SaveFlow(tab.ID, "ship", &flow.StepsSpec{
		Steps: []flow.FlowStep{{Tool: "recycle"}, {Tool: "translate", Label: "Translate"}},
	}))
	require.NoError(t, app.SaveFlow(tab.ID, "check", &flow.StepsSpec{
		Steps: []flow.FlowStep{{Tool: "qa"}},
	}))

	// Mark "ship" as the project default.
	op := app.getOpenProject(tab.ID)
	require.NotNil(t, op)
	op.Project.Defaults.Flow = "ship"

	byName := map[string]FlowInfo{}
	for _, f := range app.ListFlows(tab.ID) {
		byName[f.Name] = f
	}
	assert.Equal(t, []string{"recycle", "Translate"}, byName["ship"].Steps)
	assert.True(t, byName["ship"].Default)
	assert.Equal(t, []string{"qa"}, byName["check"].Steps)
	assert.False(t, byName["check"].Default)
}

func TestAdoptUserFlowIntoProject(t *testing.T) {
	isolateConfig(t)
	app := NewApp()
	tab := newTestProject(t, app, "Adopt")

	// Save a user flow, then adopt it into the project.
	require.NoError(t, app.SaveUserFlow(SaveUserFlowRequest{
		ID:    "my-pseudo",
		Name:  "My Pseudo",
		Steps: []flow.FlowStep{{Tool: "pseudo-translate"}},
	}))

	res, err := app.AdoptUserFlowIntoProject(tab.ID, "my-pseudo")
	require.NoError(t, err)
	assert.Equal(t, "My Pseudo", res.Name)
	assert.False(t, res.Renamed)

	// Flow is now in the in-memory recipe...
	got := app.GetFlow(tab.ID, "My Pseudo")
	require.NotNil(t, got)
	require.Len(t, got.Steps, 1)
	assert.Equal(t, "pseudo-translate", got.Steps[0].Tool)

	// ...and persisted to disk.
	reloaded, err := project.Load(app.GetProjectPath(tab.ID))
	require.NoError(t, err)
	spec, ok := reloaded.Flows["My Pseudo"]
	require.True(t, ok, "adopted flow should be saved to the recipe file")
	require.Len(t, spec.Steps, 1)
	assert.Equal(t, "pseudo-translate", spec.Steps[0].Tool)
}

func TestAdoptUserFlowDedupesName(t *testing.T) {
	isolateConfig(t)
	app := NewApp()
	tab := newTestProject(t, app, "AdoptDup")

	// Project already declares a flow named "Dup".
	require.NoError(t, app.SaveFlow(tab.ID, "Dup", &flow.StepsSpec{Steps: []flow.FlowStep{{Tool: "qa"}}}))

	require.NoError(t, app.SaveUserFlow(SaveUserFlowRequest{
		ID:    "dup-src",
		Name:  "Dup",
		Steps: []flow.FlowStep{{Tool: "pseudo-translate"}},
	}))

	res, err := app.AdoptUserFlowIntoProject(tab.ID, "dup-src")
	require.NoError(t, err)
	assert.Equal(t, "Dup-2", res.Name, "collision should produce a deduped name")
	assert.True(t, res.Renamed)

	// Original flow is untouched.
	orig := app.GetFlow(tab.ID, "Dup")
	require.NotNil(t, orig)
	assert.Equal(t, "qa", orig.Steps[0].Tool)

	// Deduped flow holds the adopted steps.
	adopted := app.GetFlow(tab.ID, "Dup-2")
	require.NotNil(t, adopted)
	assert.Equal(t, "pseudo-translate", adopted.Steps[0].Tool)
}

func TestAdoptBuiltInFlowIntoProject(t *testing.T) {
	isolateConfig(t)
	app := NewApp()
	tab := newTestProject(t, app, "AdoptBuiltin")

	builtins := flowdef.BuiltInFlows()
	require.NotEmpty(t, builtins)
	id := builtins[0].ID

	res, err := app.AdoptUserFlowIntoProject(tab.ID, id)
	require.NoError(t, err)
	assert.NotEmpty(t, res.Name)

	got := app.GetFlow(tab.ID, res.Name)
	require.NotNil(t, got)
}

func TestAdoptUserFlowUnknownFlow(t *testing.T) {
	isolateConfig(t)
	app := NewApp()
	tab := newTestProject(t, app, "AdoptMissing")
	_, err := app.AdoptUserFlowIntoProject(tab.ID, "does-not-exist")
	assert.Error(t, err)
}

func TestAdoptUserFlowUnknownTab(t *testing.T) {
	isolateConfig(t)
	app := NewApp()
	require.NoError(t, app.SaveUserFlow(SaveUserFlowRequest{
		ID:    "f1",
		Name:  "F1",
		Steps: []flow.FlowStep{{Tool: "pseudo-translate"}},
	}))
	_, err := app.AdoptUserFlowIntoProject("nope", "f1")
	assert.Error(t, err)
}
