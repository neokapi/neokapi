package state_test

import (
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/state"
	"github.com/stretchr/testify/assert"
)

func approved(unit, locale, targetHash string) state.UnitState {
	return state.UnitState{
		Unit:       unit,
		Variant:    model.Variant(model.LocaleID(locale)),
		Status:     model.TargetStatusReviewed,
		TargetHash: targetHash,
		Decision:   state.Decision{ReviewState: "approved", By: "alice", At: "2026-06-29T00:00:00Z"},
		Updated:    "2026-06-29T00:00:00Z",
	}
}

// TestUnitState_StaleOnTranslationChange verifies the targetHash link: an
// approval no longer applies once the translation it blessed changes.
func TestUnitState_StaleOnTranslationChange(t *testing.T) {
	u := approved("h1", "fr-FR", "sha256:aaa")
	assert.True(t, u.Reviewed("sha256:aaa"), "reviewed for the translation it blessed")
	assert.False(t, u.Reviewed("sha256:bbb"), "a changed translation invalidates the approval")
	assert.True(t, u.Stale("sha256:bbb"))
	assert.False(t, u.Stale("sha256:aaa"))
}

// TestUnitState_SourceStale covers the basis: the other half of what a decision
// blesses. An approval bound only to the translation survived its source being
// rewritten, so a reviewer's blessing of "refreshed every ten minutes" stayed on
// a unit whose source now said five.
func TestUnitState_SourceStale(t *testing.T) {
	tests := []struct {
		name    string
		basis   string
		current string
		want    bool
	}{
		{"same source", "sha256:src-a", "sha256:src-a", false},
		{"source rewritten", "sha256:src-a", "sha256:src-b", true},
		{"no basis recorded", "", "sha256:src-a", false},
		{"nothing to compare against", "sha256:src-a", "", false},
		{"neither known", "", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u := approved("h1", "nb", "sha256:tgt")
			u.ContentHash = tt.basis
			assert.Equal(t, tt.want, u.SourceStale(tt.current))
		})
	}
}

// TestUnitState_FreshNeedsBothHalves: a decision is about a pairing, so either
// half moving retires it.
func TestUnitState_FreshNeedsBothHalves(t *testing.T) {
	u := approved("h1", "nb", "sha256:tgt")
	u.ContentHash = "sha256:src"

	assert.True(t, u.Fresh("sha256:tgt", "sha256:src"))
	assert.False(t, u.Fresh("sha256:other", "sha256:src"), "the translation moved")
	assert.False(t, u.Fresh("sha256:tgt", "sha256:other"), "the source moved")
}

// TestSourceHash_IsTheIdentityHash: the basis and the identity signal
// core/reconcile matches on are one number, so a decision recorded on one path
// is comparable with a source read on another. Two compositions here would be
// two answers to "is this the same wording".
func TestSourceHash_IsTheIdentityHash(t *testing.T) {
	assert.Equal(t, model.ComputeContentHash("Refreshed every ten minutes."),
		state.SourceHash("Refreshed every ten minutes."))
	assert.NotEqual(t, state.SourceHash("Refreshed every ten minutes."),
		state.SourceHash("Refreshed every five minutes."))
}
