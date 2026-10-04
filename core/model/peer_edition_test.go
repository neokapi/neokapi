package model_test

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/model"
)

// A same-language channel edition is a peer of the edition the block was read
// in: it is filed under its own key in Editions, every accessor reaches it by
// that key, and nothing done to it reaches the source.
func TestASameLanguageChannelEditionIsAPeer(t *testing.T) {
	short := model.EditionKey{Locale: "en", Channel: "short"}
	en := model.Variant("en")
	b := model.NewBlock("b1", "Read the guide")
	b.SourceLocale = "en"
	b.SetEditionStatus(model.EditionKey{}, model.Status(model.SourceStatusWritten))
	basis := model.EditionRevision(b, model.EditionKey{})
	b.SetEdition(short, model.Edition{
		Runs:    []model.Run{model.TextR("Read")},
		Status:  model.Status(model.TargetStatusTranslated),
		Derived: &model.Derivation{From: en, Rev: basis},
	})

	require.Contains(t, b.Editions, short, "the edition is filed under its own key")
	require.Contains(t, b.Editions, model.EditionKey{}, "the edition the block was read in sits under the zero key")
	assert.Equal(t, "Read the guide", model.RunsText(b.Editions[model.EditionKey{}].Runs))
	assert.NotContains(t, b.Editions, en, "no same-language edition without the channel exists")

	assert.Equal(t, []model.EditionKey{en, short}, b.EditionKeys())
	assert.Equal(t, en, b.Authoritative(model.AuthorityPolicy{}), "the first native edition is the source")
	assert.Equal(t, en, b.Authoritative(model.AuthorityPolicy{Locale: "en"}))
	assert.True(t, b.IsSourceEdition(en))
	assert.False(t, b.IsSourceEdition(short))
	assert.Equal(t, short, b.EditionKeyOf(short))

	e, ok := b.Edition(short)
	require.True(t, ok)
	assert.Equal(t, "Read", model.RunsText(e.Runs))
	require.NotNil(t, e.Derived)
	assert.Equal(t, model.Derivation{From: en, Rev: basis}, *e.Derived)

	// A status stamp and an edit on the channel edition leave the source as
	// it was.
	require.True(t, b.SetEditionStatus(short, model.Status(model.TargetStatusEstablished)))
	b.SetEdition(short, model.Edition{Runs: []model.Run{model.TextR("Read it")}})
	src, _ := b.Edition(en)
	assert.Equal(t, "Read the guide", model.RunsText(src.Runs))
	assert.Equal(t, model.Status(model.SourceStatusWritten), src.Status)
	assert.Equal(t, basis, model.EditionRevision(b, model.EditionKey{}), "the source's revision does not move")
	_, edited := b.SourceAsRead()
	assert.False(t, edited)

	// A copy holds the edition as its own.
	c := b.CopyEditions(slices.Clone[[]model.Run])
	c.SetEdition(short, model.Edition{Runs: []model.Run{model.TextR("Go")}})
	assert.Equal(t, "Read it", editionText(t, b, short))
	assert.Equal(t, "Go", editionText(t, c, short))

	require.True(t, b.RemoveEdition(short))
	assert.Equal(t, []model.EditionKey{en}, b.EditionKeys())
	assert.Equal(t, "Read the guide", b.SourceText())
}

// editionText is the text of edition k of b, which must hold it.
func editionText(t *testing.T, b *model.Block, k model.EditionKey) string {
	t.Helper()
	e, ok := b.Edition(k)
	require.True(t, ok, "%+v", k)
	return model.RunsText(e.Runs)
}

// Native lists the editions the document holds: the edition the block was
// read in first, always, then each edition a reader marked, while the block
// holds it.
func TestBlockNativeEditions(t *testing.T) {
	b := model.NewBlock("b1", "Hello")
	b.SourceLocale = "en"
	assert.Equal(t, []model.EditionKey{{Locale: "en"}}, b.NativeEditions(), "a block holds the edition it was read in natively")
	assert.Empty(t, b.Native)

	fr := model.Variant("fr")
	b.SetTargetText("fr", "Bonjour")
	b.MarkNative(model.EditionKey{Locale: "FR"})
	b.MarkNative(fr)
	b.MarkNative(model.Variant("en"))
	assert.Equal(t, []model.EditionKey{{}, fr}, b.Native, "the zero key first, each edition once, canonical")
	assert.Equal(t, []model.EditionKey{{Locale: "en"}, fr}, b.NativeEditions())
	assert.Equal(t, model.EditionKey{Locale: "en"}, b.Authoritative(model.AuthorityPolicy{}), "the engine's authority is the first native edition")

	c := b.CopyEditionSet()
	c.MarkNative(model.Variant("de"))
	assert.Equal(t, []model.EditionKey{{}, fr}, b.Native, "a copy marks natives of its own")

	require.True(t, b.RemoveEdition(fr))
	assert.Equal(t, []model.EditionKey{{}}, b.Native, "a removed edition is no longer native")
	assert.Equal(t, []model.EditionKey{{Locale: "en"}}, b.NativeEditions())
}

// A copy of the edition set holds the edition the block was read in as an
// entry of its own: a status stamp or an edit on either block's source leaves
// the other's as it was, as it did when the source sat in a field of its own.
func TestBlockCopyEditionSet_SourceIsItsOwn(t *testing.T) {
	b := editionBlock()
	c := b.CopyEditionSet()

	require.True(t, c.SetEditionStatus(model.EditionKey{}, model.Status(model.SourceStatusWritten)))
	c.EditSourceText("Hi")

	src, _ := b.Edition(model.EditionKey{})
	assert.Equal(t, "Hello", model.RunsText(src.Runs))
	assert.Equal(t, model.Status(model.SourceStatusEstablished), src.Status)
	assert.Equal(t, "Hi", c.SourceText())
	_, edited := b.SourceAsRead()
	assert.False(t, edited)
}

// A translation a reader filed under no language sits apart from the
// editions: the zero key keeps naming the edition the block was read in, so a
// write under the empty locale never reaches it.
func TestBlockATranslationUnderNoLanguageNeverWritesTheSource(t *testing.T) {
	b := model.NewBlock("b1", "Hello")
	b.SetTargetRuns("", []model.Run{model.TextR("zero")})
	b.SetTargetEdition(model.EditionKey{}, model.Edition{Runs: []model.Run{model.TextR("again")}, Status: model.Status(model.TargetStatusDraft)})
	b.StampTargetProvenance("", model.TargetStatusTranslated, model.Origin{Kind: model.OriginHuman})

	assert.Equal(t, "Hello", b.SourceText())
	src, _ := b.Edition(model.EditionKey{})
	assert.Empty(t, src.Status)
	assert.Equal(t, "again", b.TargetText(""))
	e, ok := b.TargetEdition("")
	require.True(t, ok)
	assert.Equal(t, model.Status(model.TargetStatusTranslated), e.Status)
	assert.Equal(t, []model.EditionKey{{}}, b.EditionKeys(), "it is not an edition the walks list")
	assert.Len(t, b.Editions, 1)
	assert.ElementsMatch(t, []model.LocaleID{""}, b.TargetLocales())
	assert.False(t, b.RemoveEdition(model.EditionKey{}), "the zero key names the source, which stays")
}
