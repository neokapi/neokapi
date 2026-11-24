package store

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/neokapi/neokapi/core/tool"
	"github.com/neokapi/neokapi/core/venue"
)

func TestCountRejection(t *testing.T) {
	rejected := func(revision, basis string) venue.UnitDecision {
		return venue.UnitDecision{ReviewState: venue.ReviewStateRejected, Revision: revision, Basis: basis, Note: "Too formal."}
	}
	approved := venue.UnitDecision{ReviewState: "approved", Revision: "t2", Basis: "s1"}

	tests := []struct {
		name      string
		old       *venue.UnitDecision
		tally     RejectionTally
		next      venue.UnitDecision
		wantBasis string
		wantCount int
	}{
		{"a first rejection starts the count", nil, RejectionTally{}, rejected("t1", "s1"), "s1", 1},
		{"a rejection of another draft of the same source adds one",
			new(rejected("t1", "s1")), RejectionTally{Basis: "s1", Count: 1}, rejected("t2", "s1"), "s1", 2},
		{"the same rejected draft with an edited note is the same rejection",
			new(rejected("t1", "s1")), RejectionTally{Basis: "s1", Count: 1},
			venue.UnitDecision{ReviewState: venue.ReviewStateRejected, Revision: "t1", Basis: "s1", Note: "Use tu."}, "s1", 1},
		{"a rejection after an approval of the same draft counts it",
			new(venue.UnitDecision{ReviewState: "approved", Revision: "t1", Basis: "s1"}), RejectionTally{Basis: "s1", Count: 1}, rejected("t1", "s1"), "s1", 2},
		{"a rejection of a draft of another source starts again",
			new(rejected("t1", "s1")), RejectionTally{Basis: "s1", Count: 2}, rejected("t3", "s2"), "s2", 1},
		{"an approval leaves the count as it is",
			new(rejected("t1", "s1")), RejectionTally{Basis: "s1", Count: 2}, approved, "s1", 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CountRejection(tt.old, tt.tally, tt.next)
			assert.Equal(t, tt.wantBasis, got.Basis)
			assert.Equal(t, tt.wantCount, got.Count)
		})
	}
}

func TestRejectionTally_Holds(t *testing.T) {
	assert.False(t, RejectionTally{Basis: "s1", Count: tool.MaxRejections - 1}.Holds("s1"))
	assert.True(t, RejectionTally{Basis: "s1", Count: tool.MaxRejections}.Holds("s1"))
	assert.False(t, RejectionTally{Basis: "s1", Count: tool.MaxRejections}.Holds("s2"), "a changed source is drafted again")
	assert.False(t, RejectionTally{Count: tool.MaxRejections}.Holds(""), "a count that names no source holds nothing")
}
