package venue

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/model"
)

// The decisions component counts decisions, not production. A record that only
// says what was produced for a unit, the target and the source it was written
// for, is what a producer writes for every unit it drafts, so it leaves the
// component where it is: a client that read the component before a server run
// drafted a thousand units has missed no decision. A review state, a reviewed
// rung, a parked unit, an assignee and a note are each something a person or an
// agent decided, and each moves it.
func TestDecisionsComponentCountsDecisionsNotProduction(t *testing.T) {
	decided := []UnitDecision{decision("u1", "nb", "established")}
	component := DecisionsComponent(decided)
	require.NotEmpty(t, component)

	produced := []UnitDecision{
		{ItemName: "docs/intro.md", Unit: "u2", Variant: "nb", TargetHash: "t2", ContentHash: "s2", Updated: "2026-09-15T00:00:00Z"},
		{ItemName: "docs/intro.md", Unit: "u3", Variant: "nb", Status: "draft", TargetHash: "t3", ContentHash: "s3"},
		{ItemName: "docs/intro.md", Unit: "u4", Variant: "nb", Status: "translated", TargetHash: "t4", ContentHash: "s4"},
	}
	assert.Equal(t, component, DecisionsComponent(append(slices.Clone(decided), produced...)),
		"records of what was produced leave the component where it is")

	decisions := map[string]UnitDecision{
		"a rejection":         {ItemName: "docs/intro.md", Unit: "u5", Variant: "nb", ReviewState: "rejected", TargetHash: "t5"},
		"a parked unit":       {ItemName: "docs/intro.md", Unit: "u6", Variant: "nb", Parked: true},
		"an assignee":         {ItemName: "docs/intro.md", Unit: "u7", Variant: "nb", Assignee: "ben"},
		"a note":              {ItemName: "docs/intro.md", Unit: "u8", Variant: "nb", Note: "check the term"},
		"an established rung": {ItemName: "docs/intro.md", Unit: "u9", Variant: "nb", Status: "established"},
	}
	for name, rec := range decisions {
		t.Run(name, func(t *testing.T) {
			assert.NotEqual(t, component, DecisionsComponent(append(slices.Clone(decided), rec)))
		})
	}

	withdrawn := []UnitDecision{decided[0].AsBasis(model.TargetStatusTranslated)}
	assert.NotEqual(t, component, DecisionsComponent(withdrawn),
		"a decision withdrawn to its basis moves the component")
}
