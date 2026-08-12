package host

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/ref"
)

// TestGovernanceLineRendersTheSecondAxis holds the governance line to what a
// composite ref can actually support. Content counts and resolves itself;
// governance is an identity, so the line may say a component moved and may
// never say how far — there is no distance between two unequal hashes.
func TestGovernanceLineRendersTheSecondAxis(t *testing.T) {
	tests := []struct {
		name       string
		governance *StatusGovernance
		contains   []string
		absent     []string
	}{
		{
			name:       "an unconnected project prints no governance line at all",
			governance: nil,
			absent:     []string{"governance"},
		},
		{
			name: "a project that has never reached the venue says so",
			governance: &StatusGovernance{
				Observed: ref.Ref{},
			},
			contains: []string{"governance", "never observed"},
			absent:   []string{"in sync", "moved"},
		},
		{
			name: "an unreachable venue is not checked, not in sync",
			governance: &StatusGovernance{
				Observed:   ref.Ref{Context: "ctx", Terms: "trm", Decisions: "dec"},
				ObservedAt: "2026-08-11T09:00:00Z",
			},
			contains: []string{"not checked", "could not be reached", "2026-08-11T09:00:00Z"},
			absent:   []string{"in sync", "moved"},
		},
		{
			name: "every component equal reads as in sync",
			governance: &StatusGovernance{
				Observed:   ref.Ref{Content: 4, Context: "ctx", Terms: "trm", Decisions: "dec"},
				Venue:      &ref.Ref{Content: 9, Context: "ctx", Terms: "trm", Decisions: "dec"},
				ObservedAt: "2026-08-11T09:00:00Z",
			},
			contains: []string{"context in sync", "terms in sync", "decisions in sync"},
			// The position differs by five, and the governance line says
			// nothing about it: content is the other axis.
			absent: []string{"moved", "behind", "ahead", "4", "9"},
		},
		{
			name: "a moved component is named, and only that one",
			governance: &StatusGovernance{
				Observed:   ref.Ref{Context: "ctx", Terms: "trm", Decisions: "dec"},
				Venue:      &ref.Ref{Context: "ctx", Terms: "trm-2", Decisions: "dec"},
				ObservedAt: "2026-08-11T09:00:00Z",
			},
			contains: []string{"context in sync", "terms moved", "decisions in sync", "kapi pull"},
		},
		{
			name: "a component neither side claims is unobserved, never moved",
			governance: &StatusGovernance{
				Observed:   ref.Ref{Context: "ctx"},
				Venue:      &ref.Ref{Context: "ctx", Terms: "trm"},
				ObservedAt: "2026-08-11T09:00:00Z",
			},
			contains: []string{"context in sync", "terms not observed", "decisions not observed"},
			absent:   []string{"moved"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			writeGovernanceLine(&buf, tt.governance)
			for _, want := range tt.contains {
				assert.Contains(t, buf.String(), want)
			}
			for _, notWant := range tt.absent {
				assert.NotContains(t, buf.String(), notWant)
			}
		})
	}
}

// TestStatusRendersTwoAxes is the shape the report promises: one line for
// position, one for identity, aligned under labels a reader scans down.
func TestStatusRendersTwoAxes(t *testing.T) {
	var buf bytes.Buffer
	writeServerLine(&buf, StatusServerSection{
		ServerURL:   "https://app.bowrain.cloud",
		PendingPush: 12,
		PendingPull: 0,
		Governance: &StatusGovernance{
			Observed:   ref.Ref{Context: "ctx", Terms: "trm", Decisions: "dec"},
			Venue:      &ref.Ref{Context: "ctx-2", Terms: "trm", Decisions: "dec"},
			ObservedAt: "2026-08-11T09:00:00Z",
		},
		Terminology: &StatusTerminology{PulledAt: "2026-08-11T09:00:00Z", Concepts: 12, Relations: 3},
	})

	assert.Equal(t, "\n"+
		"content     https://app.bowrain.cloud · 12 to push · 0 to pull\n"+
		"governance  context moved · terms in sync · decisions in sync\n"+
		"            run `kapi pull` to take down the governance that moved\n"+
		"terms       synced 2026-08-11T09:00:00Z · 12 concepts · 3 relations\n",
		buf.String())
}

// TestGovernanceDivergenceIsCoresComparison pins where the verdict comes from.
// The plugin ships two refs and no opinion; the comparison is core/ref's, made
// once. A second implementation would answer differently the day one of them
// learned a component the other had not.
func TestGovernanceDivergenceIsCoresComparison(t *testing.T) {
	g := &StatusGovernance{
		Observed: ref.Ref{Context: "ctx", Terms: "trm"},
		Venue:    &ref.Ref{Context: "ctx-2", Terms: "trm"},
	}
	div, compared := g.Divergence()
	require.True(t, compared)
	assert.Equal(t, []ref.Component{ref.ComponentContext}, div.Moved())

	_, compared = (&StatusGovernance{Observed: ref.Ref{Context: "ctx"}}).Divergence()
	assert.False(t, compared, "no venue ref means no comparison, not an empty one")

	var absent *StatusGovernance
	_, compared = absent.Divergence()
	assert.False(t, compared)
}
