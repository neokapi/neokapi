package model_test

import (
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
)

func TestResolveTranslateAfter(t *testing.T) {
	cases := []struct {
		raw   string
		want  model.TranslateAfterLevel
		known bool
	}{
		{"", model.TranslateAfterWritten, true},
		{"written", model.TranslateAfterWritten, true},
		{"established", model.TranslateAfterEstablished, true},
		{"none", model.TranslateAfterNone, true},
		{"checked", model.TranslateAfterWritten, false},
		{"bogus", model.TranslateAfterWritten, false},
	}
	for _, c := range cases {
		got, known := model.ResolveTranslateAfter(c.raw)
		assert.Equal(t, c.want, got, "level for %q", c.raw)
		assert.Equal(t, c.known, known, "known for %q", c.raw)
	}
}

func TestTranslateAfterAdmits(t *testing.T) {
	written := model.TranslateAfterWritten
	assert.False(t, written.Admits(model.SourceStatusNew, false), "a source nothing has settled is held")
	assert.True(t, written.Admits(model.SourceStatusWritten, false))
	assert.False(t, written.Admits(model.SourceStatusWritten, true), "a failing source is held")
	assert.False(t, written.Admits(model.SourceStatusEstablished, true), "a failing source is held even when established")

	established := model.TranslateAfterEstablished
	assert.False(t, established.Admits(model.SourceStatusWritten, false), "written waits for a person")
	assert.True(t, established.Admits(model.SourceStatusEstablished, false))
	assert.False(t, established.Admits(model.SourceStatusEstablished, true))

	none := model.TranslateAfterNone
	assert.True(t, none.Admits(model.SourceStatusNew, true), "none admits everything")
}

func TestTranslateAfterAdmitsBlock(t *testing.T) {
	b := model.NewBlock("b", "Hello")
	b.SourceStatus = model.SourceStatusWritten
	assert.True(t, model.TranslateAfterWritten.AdmitsBlock(b))
	b.SetSourceFailing(true)
	assert.False(t, model.TranslateAfterWritten.AdmitsBlock(b))
	b.SetSourceFailing(false)
	assert.NotContains(t, b.Properties, model.PropSourceFailing)
}

func TestSourceStatusEffectiveRank(t *testing.T) {
	assert.Equal(t, model.SourceStatusWritten.Rank(), model.SourceStatusNew.EffectiveRank(),
		"new folds to the written baseline")
	assert.Equal(t, model.SourceStatusWritten.Rank(), model.SourceStatusWritten.EffectiveRank())
}
