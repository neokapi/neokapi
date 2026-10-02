package host

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/project"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A project flow with a parallel: step is refused before any step is built or
// any file is written, with an error that names the step and says steps run in
// order.
func TestRunFlowAllLocales_RefusesAParallelStep(t *testing.T) {
	a, _, recipe, dir := newLoopProject(t, map[string]string{
		"en.json": `{"greeting":"Hello world"}`,
	})
	proj, err := project.Load(recipe)
	require.NoError(t, err)

	_, err = a.RunFlowAllLocales(context.Background(), FlowRunOptions{
		FlowName:    "review",
		Project:     proj,
		ProjectPath: recipe,
		InputPaths:  []string{filepath.Join(dir, "src", "en.json")},
		Spec: &flow.StepsSpec{Steps: []flow.FlowStep{
			{Tool: "recycle"},
			{Parallel: []flow.FlowStep{{Tool: "qa"}, {Tool: "placeholder-check"}}},
		}},
	}, func(FlowRunEvent) {})
	require.Error(t, err)
	assert.Contains(t, err.Error(),
		`flow "review": step[1] holds a parallel: list (qa, placeholder-check), and flow steps run in order`)
	assert.NotContains(t, err.Error(), "unknown tool")

	_, statErr := os.Stat(filepath.Join(dir, "out"))
	assert.True(t, os.IsNotExist(statErr), "no file is written")
}
