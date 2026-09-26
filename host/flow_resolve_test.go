package host

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/project"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// resolveFixture is a recipe in a temp directory with inline flows, flows_dir
// files and a file that will not load. `translate` is declared both inline and
// as a file, and `pseudo-translate` as a file: both are built-in names.
func resolveFixture(t *testing.T) (*project.KapiProject, string) {
	t.Helper()
	dir := t.TempDir()
	flows := filepath.Join(dir, "flows")
	require.NoError(t, os.MkdirAll(flows, 0o755))
	for name, body := range map[string]string{
		"from-file.yaml":        "steps:\n  - tool: qa\n",
		"broken.yaml":           "steps: []\n",
		"translate.yaml":        "steps:\n  - tool: no-such-tool\n",
		"pseudo-translate.yaml": "steps:\n  - tool: qa\n  - tool: qa\n",
	} {
		require.NoError(t, os.WriteFile(filepath.Join(flows, name), []byte(body), 0o644))
	}
	proj := &project.KapiProject{
		Version:  project.CurrentVersion,
		Name:     "resolve",
		FlowsDir: "flows",
		Flows: map[string]*flow.StepsSpec{
			"inline":    {Steps: []flow.FlowStep{{Tool: "qa"}, {Tool: "qa"}}},
			"translate": {Steps: []flow.FlowStep{{Tool: "qa"}}},
		},
	}
	return proj, dir
}

// Every project surface resolves a flow name by one rule, the most specific
// definition first: the recipe's inline flow, then its flows_dir file, then
// the built-in flow of that name.
func TestResolveProjectFlow(t *testing.T) {
	proj, dir := resolveFixture(t)

	tests := []struct {
		name, flow, source string
		firstTool          string
	}{
		{"an inline flow wins over a file and a built-in of its name", "translate", FlowSourceInline, "qa"},
		{"a flows_dir file wins over a built-in of its name", "pseudo-translate", FlowSourceFile, "qa"},
		{"an inline flow", "inline", FlowSourceInline, "qa"},
		{"a flows_dir file", "from-file", FlowSourceFile, "qa"},
		{"a built-in the recipe does not declare", "translate-qa", FlowSourceBuiltin, "translate"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pf, err := ResolveProjectFlow(proj, dir, tt.flow, nil)
			require.NoError(t, err)
			require.NotNil(t, pf)
			assert.Equal(t, tt.source, pf.Source)
			require.NotEmpty(t, pf.Spec.Steps)
			assert.Equal(t, tt.firstTool, pf.Spec.Steps[0].Tool)
		})
	}

	t.Run("a flows_dir file carries its path", func(t *testing.T) {
		pf, err := ResolveProjectFlow(proj, dir, "from-file", nil)
		require.NoError(t, err)
		assert.Equal(t, filepath.Join(dir, "flows", "from-file.yaml"), pf.Path)
	})
	t.Run("an undeclared name resolves to nothing", func(t *testing.T) {
		pf, err := ResolveProjectFlow(proj, dir, "nowhere", nil)
		require.NoError(t, err)
		assert.Nil(t, pf)
	})
	t.Run("a file that will not load is an error", func(t *testing.T) {
		_, err := ResolveProjectFlow(proj, dir, "broken", nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "declares no steps")
	})
}

// `kapi up` resolves defaults.flow by the same rule as `kapi run`, so a
// built-in name the recipe does not declare runs the built-in, and a flow in
// flows_dir is a default flow like an inline one.
func TestConvergeFlowSpec_ResolvesDefaultsFlowLikeKapiRun(t *testing.T) {
	stepTools := func(spec *flow.StepsSpec) []string {
		var tools []string
		for _, s := range spec.Steps {
			tools = append(tools, s.Tool)
		}
		return tools
	}

	tests := []struct {
		name, flow string
		want       []string
	}{
		{"a built-in the recipe does not declare", "translate-qa", []string{"translate", "qa"}},
		{"a flows_dir file", "from-file", []string{"qa"}},
		{"an inline flow over a built-in of its name", "translate", []string{"qa"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			proj, dir := resolveFixture(t)
			proj.Defaults.Flow = tt.flow
			proj.Defaults.TranslateAfter = "none"
			cf, err := convergeFlowSpec(proj, dir)
			require.NoError(t, err)
			assert.Equal(t, tt.want, stepTools(cf.spec))
		})
	}
	t.Run("an undeclared name", func(t *testing.T) {
		proj, dir := resolveFixture(t)
		proj.Defaults.Flow = "nowhere"
		_, err := convergeFlowSpec(proj, dir)
		require.Error(t, err)
		assert.Contains(t, err.Error(), `"nowhere"`)
	})
}
