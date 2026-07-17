package model_test

import (
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
)

func TestResolveSourceGate(t *testing.T) {
	cases := []struct {
		raw   string
		want  model.SourceGateLevel
		known bool
	}{
		{"", model.SourceGateChecked, true},          // unset → default
		{"checked", model.SourceGateChecked, true},   // explicit default
		{"approved", model.SourceGateApproved, true}, // human sign-off
		{"authored", model.SourceGateAuthored, true}, // presence baseline
		{"none", model.SourceGateNone, true},         // opt-out
		{"bogus", model.SourceGateChecked, false},    // typo → default, flagged
	}
	for _, c := range cases {
		got, known := model.ResolveSourceGate(c.raw)
		assert.Equal(t, c.want, got, "gate for %q", c.raw)
		assert.Equal(t, c.known, known, "known for %q", c.raw)
	}
}

func TestSourceGateAdmits(t *testing.T) {
	// checked gate: authored/new is held; checked/approved clears.
	checked := model.SourceGateChecked
	assert.False(t, checked.Admits(model.SourceStatusNew), "new source is below checked")
	assert.False(t, checked.Admits(model.SourceStatusAuthored), "authored is below checked")
	assert.True(t, checked.Admits(model.SourceStatusChecked), "checked clears checked")
	assert.True(t, checked.Admits(model.SourceStatusApproved), "approved clears checked")

	// approved gate: only a human sign-off clears.
	approved := model.SourceGateApproved
	assert.False(t, approved.Admits(model.SourceStatusChecked), "checked is below approved")
	assert.True(t, approved.Admits(model.SourceStatusApproved), "approved clears approved")

	// authored gate: any present source clears (new folds to authored).
	authored := model.SourceGateAuthored
	assert.True(t, authored.Admits(model.SourceStatusNew), "new folds to authored baseline")
	assert.True(t, authored.Admits(model.SourceStatusAuthored))

	// none gate: everything clears — the opt-out.
	none := model.SourceGateNone
	assert.True(t, none.Admits(model.SourceStatusNew), "none admits everything")
	assert.Equal(t, -1, none.RequiredRank(), "none has no required rank")
}

func TestSourceStatusEffectiveRank(t *testing.T) {
	assert.Equal(t, model.SourceStatusAuthored.Rank(), model.SourceStatusNew.EffectiveRank(),
		"new folds to the authored baseline")
	assert.Equal(t, model.SourceStatusChecked.Rank(), model.SourceStatusChecked.EffectiveRank())
}
