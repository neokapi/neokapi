package model_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/model"
)

func TestBlockCopyEditionSet(t *testing.T) {
	b := editionBlock()
	c := b.CopyEditionSet()
	fr := model.Variant("fr")
	short := model.EditionKey{Locale: "en", Channel: "short"}

	require.True(t, b.RemoveEdition(fr))
	_, held := c.Edition(fr)
	assert.True(t, held, "removing an edition from the block leaves the copy's set alone")

	c.SetEdition(model.Variant("de"), model.Edition{Runs: []model.Run{model.TextR("Hallo")}})
	_, held = b.Edition(model.Variant("de"))
	assert.False(t, held, "adding an edition to the copy leaves the block's set alone")

	require.True(t, b.SetEditionStatus(short, model.Status(model.TargetStatusEstablished)))
	e, ok := c.Edition(short)
	require.True(t, ok)
	assert.Equal(t, model.Status(model.TargetStatusEstablished), e.Status, "an edition both hold is shared")

	assert.Equal(t, b.EditionKeys()[0], c.EditionKeys()[0])
	src, _ := c.Edition(c.Authoritative(model.AuthorityPolicy{}))
	assert.Equal(t, "Hello", model.RunsText(src.Runs))
	assert.Equal(t, model.Status(model.SourceStatusEstablished), src.Status)
}

// jsonCopyRuns copies a run sequence through its canonical JSON, so no run of
// the copy shares a pointer with the original.
func jsonCopyRuns(t *testing.T) func([]model.Run) []model.Run {
	return func(runs []model.Run) []model.Run {
		if runs == nil {
			return nil
		}
		var out []model.Run
		require.NoError(t, json.Unmarshal(model.CanonicalRunsJSON(runs), &out))
		return out
	}
}

func TestBlockCopyEditions(t *testing.T) {
	b := editionBlock()
	b.EditSourceText("Hello there")
	c := b.CopyEditions(jsonCopyRuns(t))
	fr := model.Variant("fr")

	want := map[string]string{}
	for k, e := range b.EachEdition {
		key, _ := k.MarshalText()
		want[string(key)] = string(model.CanonicalRunsJSON(e.Runs))
	}
	got := map[string]string{}
	for k, e := range c.EachEdition {
		key, _ := k.MarshalText()
		got[string(key)] = string(model.CanonicalRunsJSON(e.Runs))
	}
	assert.Equal(t, want, got, "the copy holds every edition the block holds")
	e, _ := c.Edition(fr)
	assert.Equal(t, model.Status(model.TargetStatusTranslated), e.Status)
	assert.Equal(t, model.OriginHuman, e.Origin.Kind)
	assert.InDelta(t, 0.8, e.Score, 0)

	frRuns, _ := c.Edition(fr)
	frRuns.Runs[0].Text.Text = "Salut"
	srcRuns := c.SourceRuns()
	srcRuns[0].Text.Text = "Goodbye"
	require.True(t, c.SetEditionStatus(fr, model.Status(model.TargetStatusEstablished)))
	c.SetTargetRuns("fr", []model.Run{model.TextR("Coucou")})

	orig, _ := b.Edition(fr)
	assert.Equal(t, "Bonjour", model.RunsText(orig.Runs), "the copy's runs are its own")
	assert.Equal(t, model.Status(model.TargetStatusTranslated), orig.Status, "the copy's editions are its own")
	assert.Equal(t, "Hello there", b.SourceText())

	asRead, edited := c.SourceAsRead()
	assert.Equal(t, "Hello", model.RunsText(asRead), "the copy keeps the source as read")
	assert.True(t, edited)
}

func TestBlockCopyEditions_HoldsItsOwnSourceAsRead(t *testing.T) {
	t.Run("an edited block", func(t *testing.T) {
		b := editionBlock()
		b.EditSourceText("Hello there")
		c := b.CopyEditions(jsonCopyRuns(t))

		asRead, edited := c.SourceAsRead()
		assert.True(t, edited, "the copy knows its source was edited")
		require.Len(t, asRead, 1)
		asRead[0].Text.Text = "Changed in place"

		orig, edited := b.SourceAsRead()
		assert.Equal(t, "Hello", model.RunsText(orig), "a change to the copy's source as read leaves the block's as it was")
		assert.True(t, edited)
		assert.Equal(t, "Hello there", b.SourceText())
	})

	t.Run("a block no edit has touched", func(t *testing.T) {
		b := editionBlock()
		c := b.CopyEditions(jsonCopyRuns(t))

		asRead, edited := c.SourceAsRead()
		assert.False(t, edited)
		require.Len(t, asRead, 1)
		asRead[0].Text.Text = "Changed in place"

		orig, edited := b.SourceAsRead()
		assert.Equal(t, "Hello", model.RunsText(orig), "a change to the copy's source leaves the block's as it was")
		assert.False(t, edited)

		c.EditSourceText("Hello again")
		orig, edited = b.SourceAsRead()
		assert.Equal(t, "Hello", model.RunsText(orig), "an edit to the copy keeps the block's source as read")
		assert.False(t, edited, "an edit to the copy leaves the block unedited")
	})
}

func TestBlockCopyEditions_KeepsANilTarget(t *testing.T) {
	b := model.NewBlock("b1", "Hello")
	b.Targets[model.Variant("fr")] = nil
	c := b.CopyEditions(func(runs []model.Run) []model.Run { return runs })
	held, ok := c.Targets[model.Variant("fr")]
	assert.True(t, ok)
	assert.Nil(t, held)

	empty := &model.Block{ID: "b2"}
	assert.Nil(t, empty.CopyEditions(func(runs []model.Run) []model.Run { return runs }).Targets, "a block with no target map gets none")
}
