package insights_test

import (
	"testing"

	"github.com/neokapi/neokapi/bowrain/core/insights"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The funnel exists to answer one question — is the context graph actually
// driving what ships, or merely sitting alongside it — so these tests are
// mostly about the shapes that answer it WRONG if the arithmetic is careless.

func stepOf(f insights.Funnel, s insights.Stage) insights.Step {
	for _, st := range f.Steps {
		if st.Stage == s {
			return st
		}
	}
	return insights.Step{}
}

// TestFunnelSurfacesTheDropNotJustTheStages: a graph can look healthy at every
// stage and still be doing nothing, which is the failure a single coverage
// number hides.
func TestFunnelSurfacesTheDropNotJustTheStages(t *testing.T) {
	f := insights.Compute(insights.Counts{
		Indexed: 1000, Governed: 900, Checked: 400,
		ProducedUnderContext: 100, TargetsTotal: 1000,
	})

	assert.Equal(t, 500, stepOf(f, insights.StageChecked).DroppedFromPrevious,
		"900 governed and 400 checked is a 500-block gap — the number worth reading")
	assert.Equal(t, 100, stepOf(f, insights.StageGoverned).DroppedFromPrevious)
	// 90% governed reads healthy; 10% produced-under-context is the truth.
	assert.Equal(t, 90, stepOf(f, insights.StageGoverned).ShareOfTotal)
	assert.Equal(t, 10, f.Headline)
}

// TestTargetsAreNotComparedToBlocks: the last stage counts targets and the rest
// count blocks. Sharing a denominator would produce a confident wrong number —
// a block with twelve locales would look like twelve blocks.
func TestTargetsAreNotComparedToBlocks(t *testing.T) {
	f := insights.Compute(insights.Counts{
		Indexed: 100, Governed: 100, Checked: 100,
		ProducedUnderContext: 600, TargetsTotal: 1200,
	})

	produced := stepOf(f, insights.StageProducedUnderContext)
	assert.Equal(t, "targets", produced.Basis)
	assert.Equal(t, "blocks", stepOf(f, insights.StageChecked).Basis)
	// 600/1200, NOT 600/100.
	assert.Equal(t, 50, produced.ShareOfTotal)
	// And no drop is computed across a basis change, because the subtraction
	// would be meaningless.
	assert.Equal(t, 0, produced.DroppedFromPrevious,
		"a drop across different populations is not a drop")
}

// TestEmptyProjectDoesNotDivideByZero: a fresh workspace is the most common
// state before the dogfood is running, and it must read as 0%, not crash and
// not report a vacuous 100%.
func TestEmptyProjectReadsAsZero(t *testing.T) {
	f := insights.Compute(insights.Counts{})
	assert.Equal(t, 0, f.Headline)
	for _, s := range f.Steps {
		assert.Equal(t, 0, s.ShareOfTotal, "stage %s", s.Stage)
	}
	assert.Contains(t, f.Narrative, "No content indexed")
}

// TestNarrativeNamesTheLargestGap: "35% checked" prompts "get it to 100";
// naming what the gap MEANS prompts the right question instead.
func TestNarrativeNamesTheLargestGap(t *testing.T) {
	t.Run("ungoverned dominates", func(t *testing.T) {
		f := insights.Compute(insights.Counts{
			Indexed: 1000, Governed: 200, Checked: 190,
			ProducedUnderContext: 180, TargetsTotal: 200,
		})
		assert.Contains(t, f.Narrative, "resolve to no profile")
	})

	t.Run("unchecked dominates", func(t *testing.T) {
		f := insights.Compute(insights.Counts{
			Indexed: 1000, Governed: 990, Checked: 100,
			ProducedUnderContext: 95, TargetsTotal: 100,
		})
		assert.Contains(t, f.Narrative, "never been scored")
	})

	t.Run("unstamped dominates", func(t *testing.T) {
		f := insights.Compute(insights.Counts{
			Indexed: 100, Governed: 100, Checked: 100,
			ProducedUnderContext: 10, TargetsTotal: 1000,
		})
		assert.Contains(t, f.Narrative, "no profile stamp")
	})

	t.Run("nothing to say", func(t *testing.T) {
		f := insights.Compute(insights.Counts{
			Indexed: 50, Governed: 50, Checked: 50,
			ProducedUnderContext: 50, TargetsTotal: 50,
		})
		assert.Contains(t, f.Narrative, "governed, checked, and producing")
	})
}

// TestInconsistentCountsRenderRatherThanFail: governed > indexed means the
// queries disagree. That is a bug, and an operator learns more from a funnel
// showing an impossible shape than from one that refuses to render.
func TestInconsistentCountsRenderRatherThanFail(t *testing.T) {
	f := insights.Compute(insights.Counts{
		Indexed: 10, Governed: 50, Checked: 5,
		ProducedUnderContext: 0, TargetsTotal: 10,
	})
	require.Len(t, f.Steps, 4)
	assert.Equal(t, 500, stepOf(f, insights.StageGoverned).ShareOfTotal,
		"an impossible 500% is visible; a refusal to render is not")
	assert.Equal(t, 0, stepOf(f, insights.StageGoverned).DroppedFromPrevious,
		"a negative drop clamps to zero rather than reading as a gain")
}

// TestPercentRounding: 1 in 3 is 33%, 2 in 3 is 67% — half-up, so the numbers
// on a dashboard add up the way a reader expects.
func TestPercentRounding(t *testing.T) {
	for _, tc := range []struct{ n, total, want int }{
		{1, 3, 33}, {2, 3, 67}, {1, 2, 50}, {1, 1000, 0}, {999, 1000, 100}, {0, 10, 0},
	} {
		f := insights.Compute(insights.Counts{Indexed: tc.total, Governed: tc.n})
		assert.Equalf(t, tc.want, stepOf(f, insights.StageGoverned).ShareOfTotal,
			"%d/%d", tc.n, tc.total)
	}
}
