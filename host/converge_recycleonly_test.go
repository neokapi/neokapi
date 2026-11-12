package host

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/project"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestConverge_RecycleOnlyFillsExactMatchesAndCallsNoProvider runs the fresh
// checkout under the built-in default flow, whose translate step has no
// provider configured, with a second string the content memory does not hold.
// A recycle-only pass fills the exact match and leaves the other string alone:
// had the translate step run, the pass would have failed for want of a
// provider.
func TestConverge_RecycleOnlyFillsExactMatchesAndCallsNoProvider(t *testing.T) {
	a, cmd, recipe := newFreshCheckoutProject(t)
	dir := filepath.Dir(recipe)
	proj, err := project.Load(recipe)
	require.NoError(t, err)
	proj.Defaults.Flow = ""
	proj.Flows = nil
	require.NoError(t, project.Save(recipe, proj))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "src", "en.json"),
		[]byte(`{"greeting":"Hello world","farewell":"Goodbye for now"}`), 0o644))
	readProjectContext(t, dir)

	proj, err = project.Load(recipe)
	require.NoError(t, err)
	var out ConvergeOutput
	require.NoError(t, a.RunDefaultFlowConverge(cmd, proj, recipe, ConvergeOptions{
		MaxPasses:   1,
		noChecks:    true,
		capture:     &out,
		recycleOnly: true,
	}))

	require.Len(t, out.Locales, 1)
	target, err := os.ReadFile(filepath.Join(dir, "src", "nb.json"))
	require.NoError(t, err)
	assert.Contains(t, string(target), `"greeting":"Hei verden"`, "the exact match filled its target")
	assert.Contains(t, string(target), `"farewell":"Goodbye for now"`, "the string the memory does not hold stays at its source")
	assert.Equal(t, recycleOnlyFlowName, out.Flow)
}

func TestRecycleOnlyFlowSpec_FillsExactMatchesOnly(t *testing.T) {
	spec := RecycleOnlyFlowSpec()
	require.Len(t, spec.Steps, 1)
	assert.Equal(t, "recycle", spec.Steps[0].Tool)
	assert.Equal(t, 100, spec.Steps[0].Config["fillTargetThreshold"])
	assert.Equal(t, 100, spec.Steps[0].Config["fuzzyThreshold"])
}
