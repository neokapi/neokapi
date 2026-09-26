package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/gate"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/schema"
	"github.com/neokapi/neokapi/core/tool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestUp_ChecksInLoop_FailingPlaceholderParks: a produced unit that drops a
// printf placeholder fails the loop's checks and holds the locale out of
// shipping — parked, with the failing count surfaced. The unit still counts as
// translated: the finding withholds the verdict, not the percentage. Fixing the
// source lets the next up converge.
func TestUp_ChecksInLoop_FailingPlaceholderParks(t *testing.T) {
	a := processOnlyApp(t)
	recipe, root := convergeFixture(t, []model.LocaleID{"nb-NO"}, gate.Gate{"translated": gate.Threshold{Pct: 100}})
	src := filepath.Join(root, "src/locales/en/a.json")
	// The flow writes each target without the `%s` its source carries: a
	// placeholder integrity failure the loop's checks must catch.
	dropPlaceholderInFlow(t, a, recipe)
	require.NoError(t, os.WriteFile(src, []byte(`{"greeting":"Hello %s, welcome."}`), 0o644))

	out, err := runUp(t, a, recipe)
	require.NoError(t, err, "failing checks park the locale — never a build failure")
	assert.Contains(t, out, "parked (needs human)", out)
	assert.Contains(t, out, "1 unit(s) failing checks", out)
	assert.Contains(t, out, "Not yet up to date", out)

	// The pass DID produce the target file — the unit is produced and failing
	// guardrails, which are two facts, not one.
	_, statErr := os.Stat(filepath.Join(root, "src/locales/nb-NO", "a.json"))
	require.NoError(t, statErr)

	// The one answer, from the surface a delivery step reads: it agrees with the
	// table above rather than offering the locale the run just parked (#2024).
	status, err := runCLI(t, NewStatusCmd(a), "--project", recipe)
	require.NoError(t, err, status)
	assert.Contains(t, status, "blocked: checks", status)
	assert.Contains(t, status, "1 unit(s) fail the project's bound checks", status)

	// Fix the source (no placeholder): the guardrail passes and up converges.
	require.NoError(t, os.WriteFile(src, []byte(`{"greeting":"Hello friend, welcome."}`), 0o644))
	a2 := processOnlyApp(t)
	registerPlaceholderDropper(a2)
	out2, err := runUp(t, a2, recipe)
	require.NoError(t, err, out2)
	assert.Contains(t, out2, "Up to date: every gated scope is shippable", out2)
	assert.NotContains(t, out2, "failing checks", out2)
}

// TestUp_NoChecksOptsOut: --no-checks skips the loop checks, so the same
// placeholder-dropping output counts as translated and the gate is met.
func TestUp_NoChecksOptsOut(t *testing.T) {
	a := processOnlyApp(t)
	recipe, root := convergeFixture(t, []model.LocaleID{"nb-NO"}, gate.Gate{"translated": gate.Threshold{Pct: 100}})
	dropPlaceholderInFlow(t, a, recipe)
	require.NoError(t, os.WriteFile(filepath.Join(root, "src/locales/en/a.json"),
		[]byte(`{"greeting":"Hello %s, welcome."}`), 0o644))

	out, err := runUp(t, a, recipe, "--no-checks")
	require.NoError(t, err, out)
	assert.Contains(t, out, "Up to date", out)
	assert.NotContains(t, out, "failing checks", out)
}

// placeholderDropper names the producer the loop-check tests run in place of
// the pseudo pass.
const placeholderDropper = "drop-placeholder"

// dropPlaceholderInFlow makes the fixture's pseudo flow run the placeholder
// dropper, so a unit reaches the loop's checks without a placeholder its
// source carries.
func dropPlaceholderInFlow(t *testing.T, a *App, recipe string) {
	t.Helper()
	registerPlaceholderDropper(a)
	proj, err := project.Load(recipe)
	require.NoError(t, err)
	spec := proj.Flows["pseudo"]
	require.NotNil(t, spec)
	spec.Steps = []flow.FlowStep{{Tool: placeholderDropper}}
	require.NoError(t, project.Save(recipe, proj))
}

// registerPlaceholderDropper registers a producer that writes each target as
// its source, prefixed with the locale and with every `%s` removed.
func registerPlaceholderDropper(a *App) {
	produce := func(locale model.LocaleID) tool.Tool {
		base := &tool.BaseTool{ToolName: placeholderDropper, ToolDescription: "writes targets without %s"}
		base.Produce = func(v tool.VariantView) error {
			if !v.Translatable() {
				return nil
			}
			v.SetTargetText(locale, string(locale)+": "+strings.ReplaceAll(v.SourceText(), "%s", ""))
			return nil
		}
		return base
	}
	a.ToolReg.RegisterWithSchema(placeholderDropper, func() tool.Tool { return produce("") }, &schema.ComponentSchema{
		ID:    placeholderDropper,
		Title: "Drop Placeholder",
		ToolMeta: &schema.ToolMeta{
			ID:          placeholderDropper,
			DisplayName: "Drop Placeholder",
			Requires:    []string{schema.RequiresTargetLanguage},
			Cardinality: schema.Bilingual,
			Produces:    []schema.IOPort{schema.Port(schema.PortTarget, model.SideTarget)},
		},
	})
	a.ToolReg.SetConfigFactory(placeholderDropper, func(_ map[string]any, targetLang string) (tool.Tool, error) {
		return produce(model.LocaleID(targetLang)), nil
	})
}

// TestComputeShipCoverage_FindingsWithholdTheVerdictNotThePercentages: a unit
// in the check-findings set keeps the rung it holds — it IS translated — and
// the finding is what takes the scope out of shippable and verified.
//
// The two halves are one property. Demoting the percentage instead put the
// number under a caller's control: a surface that ran the checks published 95%
// while one that did not published 100%, for one locale, over one tree (#2024).
func TestComputeShipCoverage_FindingsWithholdTheVerdictNotThePercentages(t *testing.T) {
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
	assert.True(t, cov[0].Shippable, "baseline: the gate is met")

	// Fail the `greeting` unit of a.json.
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
	assert.Equal(t, 100, cov2[0].Pct["translated"],
		"the unit is translated — a percentage that said otherwise would be a false statement about the content")
	assert.Equal(t, 1, cov2[0].FailingChecks, "the finding is reported on its own axis")
	assert.False(t, cov2[0].Shippable, "and it is what withholds the verdict")
	assert.NotEqual(t, "established", string(cov2[0].ShipState))
}
