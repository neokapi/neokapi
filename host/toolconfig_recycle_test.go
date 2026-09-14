package host

import (
	"testing"

	coreprofile "github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/core/project"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestToolConfigForUnit_GovernsRecycle: recycle refuses a match that breaks a
// term rule, which it can only do if it is handed the rules governing the unit.
// It gets the same rules translate gets for the same unit, resolved for the
// unit's own locale.
func TestToolConfigForUnit_GovernsRecycle(t *testing.T) {
	a, recipe, _, srcFile := unitProject(t)
	proj, err := project.LoadWithOptions(recipe, project.LoadOptions{SkipRequiresCheck: true})
	require.NoError(t, err)
	unit := UnitRef{Path: srcFile, TargetLang: "nb"}

	recycleCfg, releaseRecycle, err := a.ToolConfigForUnit(t.Context(), proj, recipe, "recycle", unit, nil)
	require.NoError(t, err)
	defer releaseRecycle()
	translateCfg, releaseTranslate, err := a.ToolConfigForUnit(t.Context(), proj, recipe, "translate", unit, nil)
	require.NoError(t, err)
	defer releaseTranslate()

	rules, ok := recycleCfg["term_rules"].([]coreprofile.TermRule)
	require.True(t, ok, "the governing term rules reach recycle")
	assert.Equal(t, map[string]string{"content memory": "innholdsminne"}, coreprofile.TermRuleMap(rules))
	assert.Equal(t, translateCfg["term_rules"], recycleCfg["term_rules"], "recycle and translate are held to the same rules")
}
