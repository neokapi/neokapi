package jobs

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/venue"
)

// The governor's maps hold one entry per block and language. A language's own
// edition fills its entry, whichever order the editions are visited in, and a
// tone or channel variant fills it only when the block holds no such edition.
func TestPushGovernor_IndexesALanguageByItsOwnEdition(t *testing.T) {
	text := func(s string) []model.Run { return []model.Run{model.TextR(s)} }
	withVariants := func(fr string) *model.Block {
		b := model.NewBlock("row-1", "Save")
		b.SourceLocale = "en"
		b.SetEdition(model.Variant("fr"), model.Edition{Runs: text(fr), Status: model.Status(model.TargetStatusEstablished)})
		b.SetEdition(model.EditionKey{Locale: "fr", Tone: "formal"}, model.Edition{Runs: text("Veuillez enregistrer"), Status: model.Status(model.TargetStatusDraft)})
		b.SetEdition(model.EditionKey{Locale: "fr", Channel: "short"}, model.Edition{Runs: text("Enreg."), Status: model.Status(model.TargetStatusTranslated)})
		b.SetEdition(model.EditionKey{Locale: "de", Tone: "formal"}, model.Edition{Runs: text("Speichern Sie"), Status: model.Status(model.TargetStatusTranslated)})
		return b
	}
	row := &venue.StoredBlock{Block: withVariants("Enregistrer")}
	pushed := withVariants("Enregistrer maintenant")
	fr := platstore.TargetRef{BlockID: "row-1", Locale: "fr"}
	de := platstore.TargetRef{BlockID: "row-1", Locale: "de"}

	// Map order varies between runs, so an order-dependent choice shows.
	for range 32 {
		g, err := newPushGovernor(t.Context(), nil, "p1", "main", "ws1", "en", "actor", nil, nil)
		require.NoError(t, err)

		g.indexRows("ui.json", []*venue.StoredBlock{row})
		assert.Equal(t, model.TargetStatusEstablished, g.priorStatus[fr])
		assert.Equal(t, languageRevision("fr", text("Enregistrer")), g.priorRevision[fr])
		assert.Equal(t, model.TargetStatusTranslated, g.priorStatus[de], "a language held only as a variant")
		assert.Equal(t, languageRevision("de", text("Speichern Sie")), g.priorRevision[de])

		g.indexPushedTargets([]stagedGroup{{ItemName: "ui.json", Blocks: []*model.Block{pushed}}})
		assert.Equal(t, languageRevision("fr", text("Enregistrer maintenant")), g.pushedRevision[fr])
		assert.Equal(t, languageRevision("de", text("Speichern Sie")), g.pushedRevision[de])
	}
}
