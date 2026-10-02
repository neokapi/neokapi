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
