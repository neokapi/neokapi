package blockstore

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/venue"
)

func TestToKBF_ProjectsEditionsByLanguage(t *testing.T) {
	text := func(s string) []model.Run { return []model.Run{{Text: &model.TextRun{Text: s}}} }
	b := model.NewBlock("row-1", "Save")
	b.SetEdition(model.Variant("fr"), model.Edition{Runs: text("Enregistrer")})
	b.SetEdition(model.EditionKey{Locale: "fr", Tone: "formal"}, model.Edition{Runs: text("Veuillez enregistrer")})
	b.SetEdition(model.EditionKey{Locale: "fr", Channel: "short"}, model.Edition{Runs: text("Enreg.")})
	b.SetEdition(model.Variant("de"), model.Edition{Runs: text("Speichern")})
	sb := &venue.StoredBlock{Block: b, SourceID: "save", ItemName: "ui.json", ContentHash: "h1"}

	tests := []struct {
		name   string
		locale string
		want   string
	}{
		{"a language holds its own edition over a tone or channel variant", "fr", "Enregistrer"},
		{"a language with one edition", "de", "Speichern"},
	}
	// The variants of one language share a slot, and the projection is read
	// many times so that an order-dependent choice shows.
	for range 32 {
		k := toKBF(sb)
		require.NotNil(t, k)
		assert.Equal(t, "save", k.ID)
		assert.Equal(t, "Save", model.RunsText(k.Source))
		assert.Len(t, k.Targets, 2)
		for _, tc := range tests {
			assert.Equal(t, tc.want, model.RunsText(k.Targets[tc.locale]), tc.name)
		}
	}
}

// A same-language channel edition has no slot in the projection: the source
// language's slot belongs to the source, and a target written there would
// come back as a same-language edition with no channel.
func TestToKBF_LeavesASameLanguageChannelEditionOut(t *testing.T) {
	text := func(s string) []model.Run { return []model.Run{{Text: &model.TextRun{Text: s}}} }
	b := model.NewBlock("row-1", "Save")
	b.SourceLocale = "en"
	b.SetEdition(model.EditionKey{Locale: "en", Channel: "short"}, model.Edition{Runs: text("Sv.")})
	b.SetEdition(model.Variant("fr"), model.Edition{Runs: text("Enregistrer")})
	sb := &venue.StoredBlock{Block: b, SourceID: "save", ItemName: "ui.json"}

	for range 32 {
		k := toKBF(sb)
		require.NotNil(t, k)
		assert.Equal(t, "Save", model.RunsText(k.Source))
		assert.Equal(t, map[string]string{"fr": "Enregistrer"}, targetTexts(k.Targets))
	}

	back := fromKBF(toKBF(sb))
	back.SourceLocale = "en"
	assert.Equal(t, []model.EditionKey{{Locale: "en"}, {Locale: "fr"}}, back.EditionKeys(),
		"no same-language edition comes back")
}

// targetTexts is a projection's targets as text, by locale.
func targetTexts[K ~string](targets map[K][]model.Run) map[string]string {
	out := map[string]string{}
	for k, runs := range targets {
		out[string(k)] = model.RunsText(runs)
	}
	return out
}
