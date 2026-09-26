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
		{"", model.SourceGateWritten, true},
		{"written", model.SourceGateWritten, true},
		{"established", model.SourceGateEstablished, true},
		{"none", model.SourceGateNone, true},
		{"checked", model.SourceGateWritten, false},
		{"bogus", model.SourceGateWritten, false},
	}
	for _, c := range cases {
		got, known := model.ResolveSourceGate(c.raw)
		assert.Equal(t, c.want, got, "gate for %q", c.raw)
		assert.Equal(t, c.known, known, "known for %q", c.raw)
	}
}

func TestSourceGateAdmits(t *testing.T) {
	written := model.SourceGateWritten
	assert.False(t, written.Admits(model.SourceStatusNew, false), "a source nothing has settled is held")
	assert.True(t, written.Admits(model.SourceStatusWritten, false))
	assert.False(t, written.Admits(model.SourceStatusWritten, true), "a failing source is held")
	assert.False(t, written.Admits(model.SourceStatusEstablished, true), "a failing source is held even when established")

	established := model.SourceGateEstablished
	assert.False(t, established.Admits(model.SourceStatusWritten, false), "written waits for a person")
	assert.True(t, established.Admits(model.SourceStatusEstablished, false))
	assert.False(t, established.Admits(model.SourceStatusEstablished, true))

	none := model.SourceGateNone
	assert.True(t, none.Admits(model.SourceStatusNew, true), "none admits everything")
}

func TestSourceGateAdmitsBlock(t *testing.T) {
	b := model.NewBlock("b", "Hello")
	b.SourceStatus = model.SourceStatusWritten
	assert.True(t, model.SourceGateWritten.AdmitsBlock(b))
	b.SetSourceFailing(true)
	assert.False(t, model.SourceGateWritten.AdmitsBlock(b))
	b.SetSourceFailing(false)
	assert.NotContains(t, b.Properties, model.PropSourceFailing)
}

func TestSourceStatusEffectiveRank(t *testing.T) {
	assert.Equal(t, model.SourceStatusWritten.Rank(), model.SourceStatusNew.EffectiveRank(),
		"new folds to the written baseline")
	assert.Equal(t, model.SourceStatusWritten.Rank(), model.SourceStatusWritten.EffectiveRank())
}
