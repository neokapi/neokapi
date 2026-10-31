package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/gate"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/memory"
)

// TestUp_MemoryFillsReadAsStampedByCheckShip: `kapi up` fills every target
// from content memory in a project no voice profile or terms govern, and holds
// the locale for review. The next `kapi check --ship` reads the stamp recycle
// left on each draft as matching the ungoverned context, so the staleness gate
// reports no unstamped target.
func TestUp_MemoryFillsReadAsStampedByCheckShip(t *testing.T) {
	t.Setenv("KAPI_ACTOR", "person")
	t.Setenv("KAPI_NO_PROJECT", "")
	a := processOnlyApp(t)
	fixture, root := convergeFixture(t, []model.LocaleID{"fr-FR"},
		gate.Gate{"translated": {Pct: 100}, "established": {Pct: 100}})
	proj, err := project.Load(fixture)
	require.NoError(t, err)
	require.NoError(t, os.Remove(fixture))
	recipe := filepath.Join(root, project.RecipeFileName)
	proj.Defaults.Flow = "memory-recycle"
	proj.Flows = map[string]*flow.StepsSpec{
		"memory-recycle": {Steps: []flow.FlowStep{{Tool: "recycle"}}},
	}
	require.NoError(t, project.Save(recipe, proj))

	seedProjectMemory(t, root,
		memory.Entry{ID: "hello", Variants: map[model.LocaleID][]model.Run{
			"en-US": {model.TextR("Hello, world.")},
			"fr-FR": {model.TextR("Bonjour le monde.")},
		}},
		memory.Entry{ID: "bye", Variants: map[model.LocaleID][]model.Run{
			"en-US": {model.TextR("Goodbye.")},
			"fr-FR": {model.TextR("Au revoir.")},
		}},
	)

	out, err := runUp(t, a, recipe)
	require.NoError(t, err, out)
	require.Contains(t, out, "content memory 2", "both blocks are filled from content memory")

	t.Chdir(root)
	check := &App{}
	cmd := NewCheckCmd(check)
	require.NoError(t, cmd.Flags().Set("ship", "true"))
	require.NoError(t, cmd.Flags().Set("project", recipe))
	shipOut, runErr := captureStdout(t, func() error { return check.RunCheck(cmd, nil) })

	require.ErrorIs(t, runErr, ErrQualityGate, "approval is still owed: %s", shipOut)
	assert.Contains(t, shipOut, "staleness")
	assert.NotContains(t, shipOut, "governing-context stamp",
		"recycle stamped every draft it wrote")
}
