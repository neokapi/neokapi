package model_test

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/model"
)

// A derived edition records its basis on itself, and its standing is read from
// the content: current while the source has that revision, stale once the
// source moves, current again when the source returns to it.
func TestDerivationStandingIsReadFromTheContent(t *testing.T) {
	b := model.NewBlock("b1", "Read the guide")
	b.SourceLocale = "en"
	en := model.Variant("en")
	fr := model.Variant("fr")
	b.SetTargetRuns("fr", []model.Run{model.TextR("Lisez le guide")})

	assert.Equal(t, model.StandingUnknown, b.DerivationStanding(fr), "nothing records what fr was made from")
	_, ok := b.Derivation(fr)
	assert.False(t, ok)

	basis := model.EditionRevision(b, en)
	require.True(t, b.SetDerivation(fr, &model.Derivation{From: en, Rev: basis}))
	d, ok := b.Derivation(fr)
	require.True(t, ok)
	assert.Equal(t, model.Derivation{From: en, Rev: basis}, d)
	assert.Equal(t, model.StandingCurrent, b.DerivationStanding(fr))

	b.EditSourceText("Read the whole guide")
	assert.Equal(t, model.StandingStale, b.DerivationStanding(fr), "the source moved under the translation")

	b.EditSourceText("Read the guide")
	assert.Equal(t, model.StandingCurrent, b.DerivationStanding(fr), "the source is back at the basis")

	// A change to a code's data moves the revision where the text does not.
	withBreak := func(data string) []model.Run {
		return []model.Run{model.TextR("Read the guide"), model.PhR(model.PlaceholderRun{ID: "1", Type: "lb", Data: data})}
	}
	b.EditSourceRuns(withBreak("<br/>"))
	require.True(t, b.SetDerivation(fr, &model.Derivation{From: en, Rev: model.EditionRevision(b, en)}))
	text := b.SourceText()
	b.EditSourceRuns(withBreak("<br>"))
	assert.Equal(t, text, b.SourceText(), "the text is the same")
	assert.Equal(t, model.StandingStale, b.DerivationStanding(fr))

	require.True(t, b.SetDerivation(fr, nil))
	assert.Equal(t, model.StandingUnknown, b.DerivationStanding(fr), "a cleared derivation records nothing")
	assert.False(t, b.SetDerivation(model.Variant("de"), &model.Derivation{From: en, Rev: basis}), "a block that holds no de is left as it was")
	_, held := b.Edition(model.Variant("de"))
	assert.False(t, held)
}

// Every key that reaches an edition reads the same derivation, and a
// derivation that names a key spelled another way is kept canonical.
func TestDerivationIsFiledCanonically(t *testing.T) {
	b := model.NewBlock("b1", "Hello")
	b.SourceLocale = "en-US"
	b.SetTargetRuns("nb-NO", []model.Run{model.TextR("Hei")})
	basis := model.EditionRevision(b, model.EditionKey{})
	require.True(t, b.SetDerivation(model.EditionKey{Locale: "nb_NO"}, &model.Derivation{From: model.EditionKey{Locale: "en_US"}, Rev: basis}))

	d, ok := b.Derivation(model.Variant("nb-NO"))
	require.True(t, ok)
	assert.Equal(t, model.EditionKey{Locale: "en-US"}, d.From)
	assert.Equal(t, model.StandingCurrent, b.DerivationStanding(model.Variant("nb-NO")))
	assert.Equal(t, model.StandingCurrent, b.BasisStanding(model.Derivation{From: model.EditionKey{}, Rev: basis}),
		"the zero key and the source language name one edition, with one revision")
}

// A derivation with no revision says nothing about its basis.
func TestBasisStandingOfADerivationWithNoRevision(t *testing.T) {
	b := model.NewBlock("b1", "Hello")
	assert.Equal(t, model.StandingUnknown, b.BasisStanding(model.Derivation{From: model.EditionKey{}}))
	assert.Equal(t, "unknown", model.StandingUnknown.String())
	assert.Equal(t, "current", model.StandingCurrent.String())
	assert.Equal(t, "stale", model.StandingStale.String())
}

// A copy of a block holds the derivation as its own.
func TestDerivationIsCopiedWithTheEditions(t *testing.T) {
	b := model.NewBlock("b1", "Hello")
	b.SourceLocale = "en"
	b.SetTargetRuns("fr", []model.Run{model.TextR("Bonjour")})
	basis := model.EditionRevision(b, model.EditionKey{})
	b.SetDerivation(model.Variant("fr"), &model.Derivation{From: model.Variant("en"), Rev: basis})

	c := b.CopyEditions(slices.Clone[[]model.Run])
	c.SetDerivation(model.Variant("fr"), &model.Derivation{From: model.Variant("en"), Rev: "r:0000000000000000"})
	d, _ := b.Derivation(model.Variant("fr"))
	assert.Equal(t, basis, d.Rev, "writing the copy's derivation leaves the original's")
}

// The derivation gate holds an edition derived from the source until the
// source reaches the level, and reads the ladder of the edition it is made
// from.
func TestTranslateAfterAdmitsDerivation(t *testing.T) {
	b := model.NewBlock("b", "Hello")
	b.SourceLocale = "en"
	short := model.EditionKey{Locale: "en", Channel: "short"}

	written := model.TranslateAfterWritten
	established := model.TranslateAfterEstablished
	assert.False(t, written.AdmitsDerivation(b, model.EditionKey{}), "a source nothing has settled holds its derivations")
	assert.True(t, model.TranslateAfterNone.AdmitsDerivation(b, model.EditionKey{}))

	b.SetEditionStatus(model.EditionKey{}, model.Status(model.SourceStatusWritten))
	assert.True(t, written.AdmitsDerivation(b, model.Variant("en")), "the source language names the source")
	assert.False(t, established.AdmitsDerivation(b, model.EditionKey{}), "established waits for a person")
	b.SetSourceFailing(true)
	assert.False(t, written.AdmitsDerivation(b, model.EditionKey{}), "failing checks hold it")
	b.SetSourceFailing(false)

	// An edition made from a channel edition waits on that edition's ladder.
	assert.False(t, written.AdmitsDerivation(b, short), "a block that holds no such edition derives nothing from it")
	b.SetEdition(short, model.Edition{Runs: []model.Run{model.TextR("Hi")}, Status: model.Status(model.TargetStatusDraft)})
	assert.False(t, written.AdmitsDerivation(b, short), "a draft is not yet written")
	b.SetEditionStatus(short, model.Status(model.TargetStatusTranslated))
	assert.True(t, written.AdmitsDerivation(b, short))
	assert.False(t, established.AdmitsDerivation(b, short))
	b.SetEditionStatus(short, model.Status(model.TargetStatusEstablished))
	assert.True(t, established.AdmitsDerivation(b, short))

	assert.Equal(t, written.AdmitsDerivation(b, model.EditionKey{}), written.AdmitsBlock(b),
		"AdmitsBlock is the gate on the edition the block was read in")
}
