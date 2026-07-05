package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/gate"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDemoteFailing: a failing unit reads at draft from translated and above;
// states below translated pass through.
func TestDemoteFailing(t *testing.T) {
	assert.Equal(t, "draft", DemoteFailing("translated"))
	assert.Equal(t, "draft", DemoteFailing("reviewed"))
	assert.Equal(t, "draft", DemoteFailing("signed-off"))
	assert.Equal(t, "draft", DemoteFailing("draft"), "draft demotes to itself (rank below translated is untouched)")
	assert.Empty(t, DemoteFailing(""))
}

// TestUp_ChecksInLoop_FailingPlaceholderParks: a produced unit that drops a
// printf placeholder fails the loop's QA check, reads at draft rather than
// translated, and keeps the locale below its translated:100 gate — parked, with
// the failing count surfaced. Fixing the source lets the next up converge.
func TestUp_ChecksInLoop_FailingPlaceholderParks(t *testing.T) {
	a := processOnlyApp(t)
	recipe, root := convergeFixture(t, []model.LocaleID{"nb-NO"}, gate.Gate{"translated": gate.Threshold{Pct: 100}})
	src := filepath.Join(root, "src/locales/en/a.json")
	// The pseudo flow accent-transforms text (it protects `{...}` but not
	// printf verbs), so the literal `%s` cannot survive into the target
	// verbatim — a placeholder integrity failure.
	require.NoError(t, os.WriteFile(src, []byte(`{"greeting":"Hello %s, welcome."}`), 0o644))

	out, err := runUp(t, a, recipe)
	require.NoError(t, err, "failing checks park the locale — never a build failure")
	assert.Contains(t, out, "parked (needs human)", out)
	assert.Contains(t, out, "1 failing check(s)", out)
	assert.Contains(t, out, "Not fully converged", out)

	// The pass DID produce the target file — the unit is produced but failing
	// guardrails, so it is held at draft, not counted as translated.
	_, statErr := os.Stat(filepath.Join(root, "src/locales/nb-NO", "a.json"))
	require.NoError(t, statErr)

	// Fix the source (no placeholder): the guardrail passes and up converges.
	require.NoError(t, os.WriteFile(src, []byte(`{"greeting":"Hello friend, welcome."}`), 0o644))
	a2 := processOnlyApp(t)
	out2, err := runUp(t, a2, recipe)
	require.NoError(t, err, out2)
	assert.Contains(t, out2, "Converged: every gated scope is shippable", out2)
	assert.NotContains(t, out2, "failing check(s)", out2)
}

// TestUp_NoChecksOptsOut: --no-checks skips the loop checks, so the same
// placeholder-dropping output counts as translated and the gate is met.
func TestUp_NoChecksOptsOut(t *testing.T) {
	a := processOnlyApp(t)
	recipe, root := convergeFixture(t, []model.LocaleID{"nb-NO"}, gate.Gate{"translated": gate.Threshold{Pct: 100}})
	require.NoError(t, os.WriteFile(filepath.Join(root, "src/locales/en/a.json"),
		[]byte(`{"greeting":"Hello %s, welcome."}`), 0o644))

	out, err := runUp(t, a, recipe, "--no-checks")
	require.NoError(t, err, out)
	assert.Contains(t, out, "Converged", out)
	assert.NotContains(t, out, "failing check(s)", out)
}

// TestComputeShipCoverage_ExclusionDemotes: the exclusion set demotes a
// translated unit to draft for gating — the unit still counts as produced
// (draft) but not toward the translated rung.
func TestComputeShipCoverage_ExclusionDemotes(t *testing.T) {
	a := processOnlyApp(t)
	recipe, root := convergeFixture(t, []model.LocaleID{"nb-NO"}, gate.Gate{"translated": gate.Threshold{Pct: 100}})

	// Materialize targets so every unit reads translated.
	out, err := runUp(t, a, recipe, "--no-checks")
	require.NoError(t, err, out)

	proj, err := project.Load(recipe)
	require.NoError(t, err)
	a.SourceLang = "en-US"
	units, err := a.UnitsFromProject(proj, root, "")
	require.NoError(t, err)
	require.NotEmpty(t, units)

	ctx := t.Context()
	cov, err := a.ComputeShipCoverage(ctx, proj, root, units, nil)
	require.NoError(t, err)
	require.Len(t, cov, 1)
	assert.Equal(t, 100, cov[0].Pct["translated"], "baseline: everything translated")

	// Exclude the `greeting` unit of a.json: translated drops below 100, the
	// unit still counts as draft (produced).
	excl := &CheckExclusions{Failing: map[string]bool{}, ByLocale: map[string]int{}}
	for _, u := range units {
		if filepath.Base(u.SourcePath) == "a.json" {
			excl.Failing[ExclusionKey(u.SourcePath, "greeting", u.Locale)] = true
			excl.ByLocale[u.Locale]++
		}
	}
	cov2, err := a.ComputeShipCoverage(ctx, proj, root, units, excl)
	require.NoError(t, err)
	require.Len(t, cov2, 1)
	assert.Less(t, cov2[0].Pct["translated"], 100, "excluded unit must not count as translated")
	assert.Equal(t, 100, cov2[0].Pct["draft"], "excluded unit still counts as produced (draft)")
	assert.False(t, cov2[0].Shippable, "the translated:100 gate is no longer met")
}
