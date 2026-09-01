package host

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/check"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// sourceStatuses settles the project's source and returns each unit's rung, the
// way the report and the gate see it.
func sourceStatuses(t *testing.T, a *App, recipe, root string) []string {
	t.Helper()
	proj, err := project.Load(recipe)
	require.NoError(t, err)
	units, err := a.UnitsFromProject(proj, root, "")
	require.NoError(t, err)
	states, _, _, err := a.settleSourceStates(t.Context(), root, "en", model.SourceGateNone, units)
	require.NoError(t, err)
	return states
}

// `approved` was a rung the ladder could not reach: the settle derivation only
// ever stamps authored or checked, and check.NewSourceReadinessTool's
// "a clean re-check never undoes a human sign-off" branch was waiting on a
// sign-off no code path could record. A project asking for `source_gate:
// approved` therefore held its fan-out forever.
func TestApproveSourceUnit_ReachesApprovedAndSurvivesARecheck(t *testing.T) {
	a, _, recipe, root := newSourceSettleProject(t, "approved")

	before := sourceStatuses(t, a, recipe, root)
	require.Len(t, before, 2)
	for _, s := range before {
		assert.Equal(t, string(model.SourceStatusChecked), s,
			"a clean source settles to checked, and nothing can lift it further")
	}

	changed, err := a.ApproveSourceUnit(t.Context(), recipe, "en", SourceUnitRef{
		File: "src/en.json", Key: "greeting",
	})
	require.NoError(t, err)
	assert.True(t, changed)

	after := sourceStatuses(t, a, recipe, root)
	assert.Contains(t, after, string(model.SourceStatusApproved),
		"the approval survives a re-settle rather than being recomputed away")

	// Recording the same approval twice is not a change.
	again, err := a.ApproveSourceUnit(t.Context(), recipe, "en", SourceUnitRef{
		File: "src/en.json", Key: "greeting",
	})
	require.NoError(t, err)
	assert.False(t, again)
}

// An approval is about a specific sentence. Editing the source has to drop it,
// or the blessing outlives the wording it blessed — the same failure the target
// side's basis hash exists to prevent.
func TestApproveSourceUnit_DroppedWhenTheSourceIsEdited(t *testing.T) {
	a, _, recipe, root := newSourceSettleProject(t, "approved")

	_, err := a.ApproveSourceUnit(t.Context(), recipe, "en", SourceUnitRef{
		File: "src/en.json", Key: "greeting",
	})
	require.NoError(t, err)
	require.Contains(t, sourceStatuses(t, a, recipe, root), string(model.SourceStatusApproved))

	require.NoError(t, os.WriteFile(filepath.Join(root, "src", "en.json"),
		[]byte(`{"greeting":"Hello, world!","farewell":"Goodbye now"}`), 0o644))

	after := sourceStatuses(t, a, recipe, root)
	assert.NotContains(t, after, string(model.SourceStatusApproved),
		"an edited sentence carries no approval")
}

// The queue is what a person is asked to look at. Under an `approved` gate that
// is everything not yet signed off, and the item says whether the loop is
// already held on it.
func TestComputeSourceQueue_ListsWhatNeedsSignOff(t *testing.T) {
	a, _, recipe, root := newSourceSettleProject(t, "approved")
	proj, err := project.Load(recipe)
	require.NoError(t, err)
	units, err := a.UnitsFromProject(proj, root, "")
	require.NoError(t, err)

	queue, err := a.computeSourceQueue(t.Context(), proj, root, units)
	require.NoError(t, err)
	require.Len(t, queue, 2, "both units clear the checks but neither is signed off")
	for _, it := range queue {
		assert.Equal(t, "src/en.json", it.File)
		assert.True(t, it.Held, "an approved gate holds a merely-checked unit")
		assert.False(t, it.Approved)
		assert.Equal(t, string(model.SourceStatusChecked), it.Status)
	}

	_, err = a.ApproveSourceUnit(t.Context(), recipe, "en", SourceUnitRef{
		File: "src/en.json", Key: "greeting",
	})
	require.NoError(t, err)

	queue, err = a.computeSourceQueue(t.Context(), proj, root, units)
	require.NoError(t, err)
	require.Len(t, queue, 1, "the approved unit leaves the queue")
	assert.Equal(t, "farewell", queue[0].Key)
}

// With the default `checked` gate, a clean source needs nobody: the queue is
// empty rather than listing every unit for a signature the gate never asks for.
func TestComputeSourceQueue_EmptyUnderTheCheckedGate(t *testing.T) {
	a, _, recipe, root := newSourceSettleProject(t, "checked")
	proj, err := project.Load(recipe)
	require.NoError(t, err)
	units, err := a.UnitsFromProject(proj, root, "")
	require.NoError(t, err)

	queue, err := a.computeSourceQueue(t.Context(), proj, root, units)
	require.NoError(t, err)
	assert.Empty(t, queue)
}

// The report and the run have to agree about a unit. Before the seeder they did
// not: settleSourceStates honoured a committed approval while the in-flow
// source gate re-derived readiness from the checks alone, so `kapi status`
// called a unit approved and the run beside it held that same unit below an
// `approved` gate, with nothing on either surface to say why.
func TestSourceStateSeeder_MakesTheInFlowGateAgreeWithTheReport(t *testing.T) {
	a, _, recipe, root := newSourceSettleProject(t, "approved")

	// No approvals: no seeder, and nothing to pay for.
	seed, err := a.SourceStateSeeder(t.Context(), root, "en")
	require.NoError(t, err)
	assert.Nil(t, seed, "a project with no approvals installs no hook")

	_, err = a.ApproveSourceUnit(t.Context(), recipe, "en", SourceUnitRef{
		File: "src/en.json", Key: "greeting",
	})
	require.NoError(t, err)

	seed, err = a.SourceStateSeeder(t.Context(), root, "en")
	require.NoError(t, err)
	require.NotNil(t, seed)

	proj, err := project.Load(recipe)
	require.NoError(t, err)
	units, err := a.UnitsFromProject(proj, root, "")
	require.NoError(t, err)

	blocks, err := a.readSource(t.Context(), units[0])
	require.NoError(t, err)

	var approved, other int
	for _, b := range blocks {
		if !b.Translatable {
			continue
		}
		seed(units[0].SourcePath, b)
		// The gate settles after the seed, exactly as the in-flow stage does.
		check.SettleSourceStatus(t.Context(), b)
		if b.SourceStatus == model.SourceStatusApproved {
			approved++
		} else {
			other++
		}
	}
	assert.Equal(t, 1, approved, "the approved unit clears an approved gate in-flow")
	assert.Equal(t, 1, other, "the unapproved one still does not")
}
