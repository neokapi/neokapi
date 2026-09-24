package host

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/host/output"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// listFlowsJSON runs `kapi flows --json -p <recipe>` and returns what it
// listed and what it wrote to stderr.
func listFlowsJSON(t *testing.T, a *App, recipe string, opts FlowCmdOptions) ([]output.FlowInfo, string) {
	t.Helper()
	cmd := NewEnvCommand(context.Background(), "flows")
	fs := cmd.Flags()
	fs.String("project", "", "")
	fs.Bool("json", true, "")
	fs.String("output", "json", "")
	require.NoError(t, fs.Set("project", recipe))
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	require.NoError(t, a.ListFlows(cmd, opts))
	var got output.FlowsListOutput
	require.NoError(t, json.Unmarshal(out.Bytes(), &got), out.String())
	return got.Flows, errOut.String()
}

// A recipe that does not load still lists the built-in flows, and says why the
// project's are missing.
func TestListFlows_SaysWhenTheRecipeDoesNotLoad(t *testing.T) {
	a, recipe, _ := dirFlowProject(t, "guard", pseudoDirFlow)
	body, err := os.ReadFile(recipe)
	require.NoError(t, err)
	broken := strings.Replace(string(body), "flows_dir: flows", "flows_dir: .kapi/flows", 1)
	require.NotEqual(t, string(body), broken)
	require.NoError(t, os.WriteFile(recipe, []byte(broken), 0o644))

	flows, stderr := listFlowsJSON(t, a, recipe, FlowCmdOptions{})
	assert.NotEmpty(t, flows, "the built-in flows still list")
	for _, f := range flows {
		assert.NotEqual(t, "guard", f.Name)
	}
	assert.Contains(t, stderr, "disposable cache")
	assert.Contains(t, stderr, "its flows are not listed")
}

// `kapi flows` lists the project's own flows, inline and file-per-flow, beside
// the built-in compositions, each name once and for the flow `kapi run` runs.
func TestListFlows_ListsTheProjectsFlows(t *testing.T) {
	a, recipe, dir := dirFlowProject(t, "guard", pseudoDirFlow)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "flows", "broken.yaml"), []byte("steps: []\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "flows", "inline.yaml"), []byte(pseudoDirFlow), 0o644))

	loaded, err := project.Load(recipe)
	require.NoError(t, err)
	loaded.Flows = map[string]*flow.StepsSpec{
		"inline":       {Steps: []flow.FlowStep{{Tool: "qa"}, {Tool: "pseudo-translate"}}},
		"translate-qa": {Steps: []flow.FlowStep{{Tool: "qa"}}},
	}
	require.NoError(t, project.Save(recipe, loaded))

	flows, _ := listFlowsJSON(t, a, recipe, FlowCmdOptions{
		ExtraFlows: func() []output.FlowInfo {
			return []output.FlowInfo{{Name: "guard", Description: "from a plugin"}, {Name: "plugin-only"}}
		},
	})
	byName := map[string][]output.FlowInfo{}
	for _, f := range flows {
		byName[f.Name] = append(byName[f.Name], f)
	}

	require.Len(t, byName["translate-qa"], 1, "a built-in name is listed once")
	assert.Equal(t, 1, byName["translate-qa"][0].Steps, "as the recipe's flow, which kapi run resolves it to")
	assert.Equal(t, recipe, byName["translate-qa"][0].Path)

	require.Len(t, byName["inline"], 1, "an inline flow wins over a file of its name")
	assert.Equal(t, 2, byName["inline"][0].Steps)

	require.Len(t, byName["guard"], 1, "a file-per-flow definition, listed once")
	assert.Equal(t, "Generate pseudo-translations for testing", byName["guard"][0].Description)

	require.Len(t, byName["broken"], 1, "a file that will not run is listed with its problem")
	assert.Contains(t, byName["broken"][0].Description, "declares no steps")

	assert.Len(t, byName["plugin-only"], 1, "a plugin's flows are still listed")
}
