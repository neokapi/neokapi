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
