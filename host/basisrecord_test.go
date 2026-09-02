package host

import (
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/state"
	"github.com/stretchr/testify/assert"
)

// The basis grading, over the two classes of record that share the index: a
// person's decision, and the loop's record of a target it wrote. They are graded
// against the source by the same comparison, which is the point — drift is drift
// whoever produced the translation — and they differ in what a rewritten
// TRANSLATION does to them.

// gradeBlock builds the block a grading is asked about.
func gradeBlock(source, target string) *model.Block {
	b := model.NewBlock("u1", source)
	b.Name = "u1"
	b.Translatable = true
	if target != "" {
		b.SetTargetText("nb", target)
	}
	return b
}

func TestReviewedIndex_GradeBasis(t *testing.T) {
	const (
		scope     = "d-doc"
		source    = "Apple"
		rewritten = "Apricot"
		target    = "Eple"
		edited    = "Eplet"
	)
	decision := state.Decision{ReviewState: "approved"}

	tests := []struct {
		name string
		rec  state.UnitState
		// the content in front of the reader
		source, target string
		wantBasis      basisVerdict
		wantApplies    bool
	}{
		{
			name:      "no record at all is basisNone, whatever is on disk",
			source:    rewritten,
			target:    target,
			wantBasis: basisNone,
		},
		{
			name: "a decision whose source moved is stale",
			rec: state.UnitState{
				Status: model.TargetStatusReviewed, Decision: decision,
				TargetHash: state.TargetHash(target), ContentHash: state.SourceHash(source),
			},
			source:    rewritten,
			target:    target,
			wantBasis: basisStale,
		},
		{
			name: "a decision on the source the project holds still applies",
			rec: state.UnitState{
				Status: model.TargetStatusReviewed, Decision: decision,
				TargetHash: state.TargetHash(target), ContentHash: state.SourceHash(source),
			},
			source:      source,
			target:      target,
			wantBasis:   basisCurrent,
			wantApplies: true,
		},
		{
			name: "a decision written before the basis was tracked is unknown, never stale",
			rec: state.UnitState{
				Status: model.TargetStatusReviewed, Decision: decision,
				TargetHash: state.TargetHash(target),
			},
			source:      rewritten,
			target:      target,
			wantBasis:   basisUnknown,
			wantApplies: true,
		},
		{
			// The whole point of the loop's own record: the same rewrite, under
			// a translation nobody has decided, reads the same way.
			name: "a basis the loop recorded, source moved, is stale",
			rec: state.UnitState{
				Status:     model.TargetStatusTranslated,
				TargetHash: state.TargetHash(target), ContentHash: state.SourceHash(source),
			},
			source:    rewritten,
			target:    target,
			wantBasis: basisStale,
		},
		{
			name: "a basis the loop recorded, source unchanged, is current and decides nothing",
			rec: state.UnitState{
				Status:     model.TargetStatusTranslated,
				TargetHash: state.TargetHash(target), ContentHash: state.SourceHash(source),
			},
			source:    source,
			target:    target,
			wantBasis: basisCurrent,
		},
		{
			// A person took the translation over. Their wording renders whatever
			// they had in front of them, which the record cannot name, so the
			// loop stops speaking for the unit rather than re-drafting over it.
			name: "a basis the loop recorded, translation rewritten by hand, is unknown",
			rec: state.UnitState{
				Status:     model.TargetStatusTranslated,
				TargetHash: state.TargetHash(target), ContentHash: state.SourceHash(source),
			},
			source:    rewritten,
			target:    edited,
			wantBasis: basisUnknown,
		},
		{
			// A decision reads its rewritten translation differently: it is
			// still a decision ABOUT a source, and coverage says the source
			// moved so the scope does not ship.
			name: "a decision whose translation was rewritten still grades its source",
			rec: state.UnitState{
				Status: model.TargetStatusReviewed, Decision: decision,
				TargetHash: state.TargetHash(target), ContentHash: state.SourceHash(source),
			},
			source:    rewritten,
			target:    edited,
			wantBasis: basisStale,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			idx := reviewedIndex{byUnit: map[string]reviewedEntry{}, aiReviews: map[string]aiReviewEntry{}}
			if tc.rec.TargetHash != "" || tc.rec.ContentHash != "" {
				rec := tc.rec
				rec.Unit, rec.Scope, rec.Variant = "u1", scope, model.Variant("nb")
				loadStateInto(idx, rec)
			}
			b := gradeBlock(tc.source, tc.target)
			e, basis, applies := idx.grade(scope, b, "nb")
			assert.Equal(t, tc.wantBasis, basis)
			assert.Equal(t, tc.wantApplies, applies)
			if !tc.wantApplies {
				assert.False(t, approvesTarget(e, applies), "nothing applies, so nothing approves")
			}
		})
	}
}

// loadStateInto mirrors what loadReviewedCorrections does for one record, so the
// table above exercises the classification the store feeds rather than a
// hand-built entry.
func loadStateInto(idx reviewedIndex, u state.UnitState) {
	key := reviewUnitKey(u.Scope, u.Unit, string(u.Variant.Locale))
	switch u.Status {
	case model.TargetStatusReviewed, model.TargetStatusSignedOff, model.TargetStatusDraft:
		idx.byUnit[key] = reviewedEntry{
			status: u.Status, targetHash: u.TargetHash,
			contentHash: u.ContentHash, by: u.Decision.By, decided: true,
		}
	default:
		if u.TargetHash != "" && u.Decision.ReviewState == "" {
			idx.byUnit[key] = reviewedEntry{
				status: u.Status, targetHash: u.TargetHash, contentHash: u.ContentHash,
			}
		}
	}
}

// A loop-written basis never moves a unit on the ladder. It carries a status so
// the record says what the target is, and `apply` has to ignore it: reading it
// would report every translation the loop produced at whatever rung the record
// happened to name.
func TestReviewedIndex_ApplyIgnoresARecordedBasis(t *testing.T) {
	idx := reviewedIndex{byUnit: map[string]reviewedEntry{}, aiReviews: map[string]aiReviewEntry{}}
	loadStateInto(idx, state.UnitState{
		Unit: "u1", Scope: "d-doc", Variant: model.Variant("nb"),
		Status:     model.TargetStatusTranslated,
		TargetHash: state.TargetHash("Eple"), ContentHash: state.SourceHash("Apple"),
	})
	b := gradeBlock("Apple", "Eple")

	st, aiDecided, basis := idx.apply(string(model.TargetStatusTranslated), "d-doc", b, "nb")
	assert.Equal(t, string(model.TargetStatusTranslated), st)
	assert.False(t, aiDecided)
	assert.Equal(t, basisCurrent, basis)
	assert.False(t, idx.decided("d-doc", b, "nb"), "the loop deciding its own output is what this forbids")
}
