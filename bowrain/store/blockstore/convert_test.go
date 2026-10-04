package blockstore

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/kbf"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/venue"
)

// The projection keys every edition by its full key, so a language's own
// edition and its tone and channel editions each keep their content.
func TestToKBF_ProjectsEveryEditionUnderItsKey(t *testing.T) {
	text := func(s string) []model.Run { return []model.Run{{Text: &model.TextRun{Text: s}}} }
	b := model.NewBlock("row-1", "Save")
	b.SetEdition(model.Variant("fr"), model.Edition{Runs: text("Enregistrer")})
	b.SetEdition(model.EditionKey{Locale: "fr", Tone: "formal"}, model.Edition{Runs: text("Veuillez enregistrer")})
	b.SetEdition(model.EditionKey{Locale: "fr", Channel: "short"}, model.Edition{Runs: text("Enreg.")})
	b.SetEdition(model.Variant("de"), model.Edition{Runs: text("Speichern")})
	sb := &venue.StoredBlock{Block: b, SourceID: "save", ItemName: "ui.json", ContentHash: "h1"}

	want := map[string]string{
		"de":               "Speichern",
		"fr":               "Enregistrer",
		"fr;channel=short": "Enreg.",
		"fr;tone=formal":   "Veuillez enregistrer",
	}
	for range 32 {
		k := toKBF(sb)
		require.NotNil(t, k)
		assert.Equal(t, "save", k.ID)
		assert.Equal(t, "Save", model.RunsText(k.SourceRuns()))
		assert.Equal(t, want, editionTexts(k))
	}
}

// A same-language channel edition keeps its own key through the projection
// and back, and no plain edition in the source language appears.
func TestToKBF_CarriesASameLanguageChannelEdition(t *testing.T) {
	text := func(s string) []model.Run { return []model.Run{{Text: &model.TextRun{Text: s}}} }
	b := model.NewBlock("row-1", "Save")
	b.SourceLocale = "en"
	short := model.EditionKey{Locale: "en", Channel: "short"}
	b.SetEdition(short, model.Edition{Runs: text("Sv.")})
	b.SetEdition(model.Variant("fr"), model.Edition{Runs: text("Enregistrer")})
	sb := &venue.StoredBlock{Block: b, SourceID: "save", ItemName: "ui.json"}

	k := toKBF(sb)
	require.NotNil(t, k)
	assert.Equal(t, "Save", model.RunsText(k.SourceRuns()))
	assert.Equal(t, map[string]string{"en;channel=short": "Sv.", "fr": "Enregistrer"}, editionTexts(k))

	back := fromKBF(k)
	back.SourceLocale = "en"
	assert.Equal(t, []model.EditionKey{{Locale: "en"}, short, {Locale: "fr"}}, back.EditionKeys(),
		"the channel edition comes back under its key, and no same-language edition appears")
}

// editionTexts is the text of every edition of a projection but the source,
// by key.
func editionTexts(b *kbf.Block) map[string]string {
	out := map[string]string{}
	for _, key := range b.TargetKeys() {
		out[key] = model.RunsText(b.Editions[key].Runs)
	}
	return out
}
