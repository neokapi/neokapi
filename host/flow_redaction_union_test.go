package host

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/project"
)

// A redact step detects with its own detectors and the policy's together, and
// takes the policy's rules file only when it names no rules of its own.
func TestApplyFlowRedaction_UnionsDetectors(t *testing.T) {
	isolateKapiEnv(t)
	recipe := redactProject(t, redactionYAML)
	proj, err := project.Load(recipe)
	require.NoError(t, err)
	a := &App{SourceLang: "en", ProjectContext: project.NewProjectContext(proj, recipe)}
	a.InitRegistries()
	rules := filepath.Join(filepath.Dir(recipe), "redact.yaml")

	out, err := a.applyFlowRedaction("redact", map[string]any{"detectors": []any{"entities"}}, nil)
	require.NoError(t, err)
	assert.Equal(t, []string{"entities", "rules"}, out["detectors"], "the step's entity detection stays beside the policy's rules")
	assert.Equal(t, rules, out["rulesPath"])

	own := filepath.Join(t.TempDir(), "own.yaml")
	out, err = a.applyFlowRedaction("redact", map[string]any{"rulesPath": own}, nil)
	require.NoError(t, err)
	assert.Equal(t, []string{"rules"}, out["detectors"])
	assert.Equal(t, own, out["rulesPath"], "a step that names rules keeps them")

	out, err = a.applyFlowRedaction("translate", map[string]any{"provider": "demo"}, nil)
	require.NoError(t, err)
	assert.Equal(t, map[string]any{"provider": "demo"}, out, "every other step is left as it is")
}
