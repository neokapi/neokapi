package model_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/model"
)

func TestParseEditionKey(t *testing.T) {
	tests := []struct {
		in      string
		want    model.EditionKey
		wantErr string
	}{
		{in: "", want: model.EditionKey{}},
		{in: "fr", want: model.EditionKey{Locale: "fr"}},
		{in: "nb_NO", want: model.EditionKey{Locale: "nb-NO"}},
		{in: "en;channel=short", want: model.EditionKey{Locale: "en", Channel: "short"}},
		{in: "fr;channel=web;tone=formal", want: model.EditionKey{Locale: "fr", Tone: "formal", Channel: "web"}},
		{in: ";tone=formal", wantErr: "names no language"},
		{in: "xx-YY", wantErr: "invalid locale"},
		{in: "fr;tone", wantErr: "not name=value"},
		{in: "fr;tone=", wantErr: "not name=value"},
		{in: "fr;tone=a;tone=b", wantErr: "tone twice"},
		{in: "fr;product=app", wantErr: "unknown dimension"},
	}
	for _, tc := range tests {
		t.Run(tc.in, func(t *testing.T) {
			got, err := model.ParseEditionKey(tc.in)
			if tc.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func editionBlock() *model.Block {
	b := model.NewBlock("b1", "Hello")
	b.SourceLocale = "en"
	b.SourceStatus = model.SourceStatusEstablished
	b.SetTarget("fr", &model.Target{Runs: []model.Run{model.TextR("Bonjour")}, Status: model.TargetStatusTranslated, Origin: model.Origin{Kind: model.OriginHuman}, Score: 0.8})
	b.SetTargetVariant(model.EditionKey{Locale: "en", Channel: "short"}, &model.Target{Runs: []model.Run{model.TextR("Hi")}})
	return b
}

func TestBlockEdition_RoutesEveryKeyToOneEdition(t *testing.T) {
	b := editionBlock()
	tests := []struct {
		name string
		key  model.EditionKey
		text string
		ok   bool
	}{
		{"the zero key is the document's own edition", model.EditionKey{}, "Hello", true},
		{"the source language is the edition Source holds", model.EditionKey{Locale: "en"}, "Hello", true},
		{"another spelling of the source language", model.EditionKey{Locale: "EN"}, "Hello", true},
		{"a derived edition", model.EditionKey{Locale: "fr"}, "Bonjour", true},
		{"a same-language channel edition is its own edition", model.EditionKey{Locale: "en", Channel: "short"}, "Hi", true},
		{"an edition the block does not hold", model.EditionKey{Locale: "de"}, "", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			e, ok := b.Edition(tc.key)
			assert.Equal(t, tc.ok, ok)
			assert.Equal(t, tc.text, model.RunsText(e.Runs))
		})
	}
	src, _ := b.Edition(model.EditionKey{})
	assert.Equal(t, model.Status(model.SourceStatusEstablished), src.Status)
	fr, _ := b.Edition(model.Variant("fr"))
	assert.Equal(t, model.Edition{Runs: b.Targets[model.Variant("fr")].Runs, Status: "translated", Origin: model.Origin{Kind: model.OriginHuman}, Score: 0.8}, fr)
}

func TestBlockSetEdition(t *testing.T) {
	t.Run("the source edition is an edit that keeps the read content", func(t *testing.T) {
		b := editionBlock()
		b.SetEdition(model.EditionKey{Locale: "en"}, model.Edition{Runs: []model.Run{model.TextR("Hi there")}, Status: "written"})
		assert.Equal(t, "Hi there", b.SourceText())
		assert.Equal(t, model.SourceStatusWritten, b.SourceStatus)
		read, edited := b.SourceAsRead()
		assert.True(t, edited)
		assert.Equal(t, "Hello", model.RunsText(read))
	})
	t.Run("an existing derived edition is updated in place", func(t *testing.T) {
		b := editionBlock()
		held := b.Target("fr")
		b.SetEdition(model.Variant("fr"), model.Edition{Runs: []model.Run{model.TextR("Salut")}, Status: "draft"})
		assert.Same(t, held, b.Target("fr"))
		assert.Equal(t, "Salut", model.RunsText(held.Runs))
		assert.Equal(t, model.TargetStatusDraft, held.Status)
	})
	t.Run("a new edition is filed under its canonical key", func(t *testing.T) {
		b := editionBlock()
		b.SetEdition(model.EditionKey{Locale: "nb_NO"}, model.Edition{Runs: []model.Run{model.TextR("Hei")}})
		assert.Equal(t, "Hei", b.TargetText("nb-NO"))
		_, raw := b.Targets[model.EditionKey{Locale: "nb_NO"}]
		assert.False(t, raw, "no target is filed under the spelling a caller used")
	})
	t.Run("the source origin follows the edition", func(t *testing.T) {
		b := editionBlock()
		b.SetEdition(model.EditionKey{}, model.Edition{Runs: b.Source, Origin: model.Origin{Kind: model.OriginOCR}})
		o, ok := b.SourceOrigin()
		require.True(t, ok)
		assert.Equal(t, model.OriginOCR, o.Kind)
		b.SetEdition(model.EditionKey{}, model.Edition{Runs: b.Source})
		_, ok = b.SourceOrigin()
		assert.False(t, ok)
	})
}

func TestBlockRemoveEditionAndEditions(t *testing.T) {
	b := editionBlock()
	assert.Equal(t, []model.EditionKey{{Locale: "en"}, {Locale: "en", Channel: "short"}, {Locale: "fr"}}, b.Editions())
	assert.False(t, b.RemoveEdition(model.EditionKey{Locale: "en"}), "the edition the block was read in stays")
	assert.False(t, b.RemoveEdition(model.Variant("de")))
	assert.True(t, b.RemoveEdition(model.EditionKey{Locale: "FR"}))
	assert.Equal(t, []model.EditionKey{{Locale: "en"}, {Locale: "en", Channel: "short"}}, b.Editions())
}

// A bilingual file whose two languages are one (an XLIFF file from en-US to
// en-US, a PO catalogue in its source language) holds a target under the key
// of the source language. That key then reaches the target, and the zero key
// alone reaches the edition the block was read in, so neither hides the
// other and a write to one never lands in the other.
func TestBlockEdition_ASameLanguageTargetKeepsItsKey(t *testing.T) {
	b := model.NewBlock("b1", "colour source")
	b.SourceLocale = "en-US"
	b.SourceStatus = model.SourceStatusWritten
	b.SetTarget("en-US", &model.Target{Runs: []model.Run{model.TextR("colour target")}, Status: model.TargetStatusEstablished})

	assert.Equal(t, []model.EditionKey{{}, {Locale: "en-US"}}, b.Editions())
	tgt, ok := b.Edition(model.Variant("en-US"))
	require.True(t, ok)
	assert.Equal(t, "colour target", model.RunsText(tgt.Runs))
	src, ok := b.Edition(model.EditionKey{})
	require.True(t, ok)
	assert.Equal(t, "colour source", model.RunsText(src.Runs))
	assert.False(t, b.IsSourceEdition(model.Variant("en-US")))
	assert.Equal(t, model.EditionKey{}, b.Authoritative(model.AuthorityPolicy{}))
	assert.Equal(t, model.EditionKey{}, b.Authoritative(model.AuthorityPolicy{Locale: "en-US"}), "the source language names the edition the block was read in")
	assert.NotEqual(t, model.EditionRevision(b, model.EditionKey{}), model.EditionRevision(b, model.Variant("en-US")))

	b.SetEdition(model.Variant("en-US"), model.Edition{Runs: []model.Run{model.TextR("color target")}, Status: "draft"})
	assert.Equal(t, "color target", b.TargetText("en-US"))
	assert.Equal(t, "colour source", b.SourceText())
	assert.Equal(t, model.SourceStatusWritten, b.SourceStatus)

	assert.True(t, b.RemoveEdition(model.Variant("en-US")))
	assert.Equal(t, "colour source", b.SourceText())
	assert.Equal(t, []model.EditionKey{{Locale: "en-US"}}, b.Editions(), "with no such target the source language reaches the source again")
}

func TestBlockAuthoritative(t *testing.T) {
	b := editionBlock()
	assert.Equal(t, model.EditionKey{Locale: "en"}, b.Authoritative(model.AuthorityPolicy{}))
	assert.Equal(t, model.EditionKey{Locale: "en"}, b.Authoritative(model.AuthorityPolicy{Locale: "en"}))
	assert.Equal(t, model.EditionKey{Locale: "fr"}, b.Authoritative(model.AuthorityPolicy{Locale: "fr"}), "a recipe can name a held edition")
	assert.Equal(t, model.EditionKey{Locale: "en"}, b.Authoritative(model.AuthorityPolicy{Locale: "de"}), "an edition the block lacks names nothing")
}
