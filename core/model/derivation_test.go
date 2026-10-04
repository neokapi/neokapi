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

// Readers of one document disagree on the key the edition a block was read in
// is filed under, and a revision of it depends on that key. Every revision of
// the same content, under any key a read of the same document gives it, is
// among the block's source revisions; a revision of other content is none of
// them.
func TestSourceRevisionsCoverEveryKeyAReaderGives(t *testing.T) {
	runs := []model.Run{model.TextR("Read the guide")}
	readAs := func(locale, declared model.LocaleID) *model.Block {
		b := model.NewRunsBlock("b1", runs)
		b.SourceLocale = locale
		if declared != "" {
			b.Properties = map[string]string{model.PropReadSourceLocale: string(declared)}
		}
		return b
	}
	const project = "en-US"
	documents := map[string][]*model.Block{
		// A reader that declares no language: a plain read leaves the block
		// with none, and a project read, or the change service, files it
		// under the project's language.
		"no language declared": {readAs("", ""), readAs(project, "")},
		// A reader that declares its own: a plain read and the change
		// service keep it, and a project read files the block under the
		// project's language and records the reader's.
		"a language declared": {readAs("en", ""), readAs(project, "en")},
	}
	for name, reads := range documents {
		for _, writer := range reads {
			rev := model.EditionRevision(writer, model.EditionKey{})
			for _, reader := range reads {
				assert.Contains(t, reader.SourceRevisions(project), rev,
					"%s: taken under %q, read under %q", name, writer.SourceLocale, reader.SourceLocale)
			}
		}
	}
	under := readAs(project, "")
	assert.Equal(t, model.EditionRevision(under, model.EditionKey{}), under.SourceRevisions(project)[0],
		"the revision under the key the block files it by comes first")

	moved := readAs(project, "")
	moved.SetSourceRuns([]model.Run{model.TextR("Read the whole guide")})
	for _, rev := range moved.SourceRevisions(project) {
		for _, reads := range documents {
			for _, reader := range reads {
				assert.NotContains(t, reader.SourceRevisions(project), rev, "other content has other revisions under every key")
			}
		}
	}
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

// A derivation made where the edition the block was read in is filed under
// the language its reader declared stands on the same content filed under
// another key, as a project read files it under the project's language, and
// reads stale once that content moves. A derivation from an edition the block
// holds of its own is graded against that edition alone.
func TestBasisStandingMatchesTheSourceUnderEveryKey(t *testing.T) {
	read := func(locale model.LocaleID) *model.Block {
		b := model.NewBlock("b1", "Read the guide")
		b.SourceLocale = locale
		b.SetTargetRuns("fr", []model.Run{model.TextR("Lisez le guide")})
		return b
	}
	declared := read("en")
	from := declared.Authoritative(model.AuthorityPolicy{})
	d := model.Derivation{From: from, Rev: model.EditionRevision(declared, from)}
	require.Equal(t, model.Variant("en"), from)

	project := read("en-US")
	assert.Equal(t, model.StandingCurrent, project.BasisStanding(d), "the same content under the project's language")
	project.Properties = map[string]string{model.PropReadSourceLocale: "en"}
	assert.Equal(t, model.StandingCurrent, project.BasisStanding(d), "and with the reader's language recorded")
	assert.Equal(t, model.StandingCurrent, read("").BasisStanding(d), "and under no language")

	project.EditSourceText("Read the whole guide")
	assert.Equal(t, model.StandingStale, project.BasisStanding(d), "the content moved")

	// Taken under the project's language and read where the reader's own is
	// kept, and taken under no language and read under one.
	underProject := read("en-US")
	fromProject := underProject.Authoritative(model.AuthorityPolicy{})
	assert.Equal(t, model.StandingCurrent, read("en").BasisStanding(
		model.Derivation{From: fromProject, Rev: model.EditionRevision(underProject, fromProject)}))
	unfiled := read("")
	assert.Equal(t, model.StandingCurrent, read("en-US").BasisStanding(
		model.Derivation{From: model.EditionKey{}, Rev: model.EditionRevision(unfiled, model.EditionKey{})}))

	// A block that holds an en edition of its own grades a derivation from en
	// against that edition, whatever its source holds.
	held := read("en-US")
	held.SetTargetRuns("en", []model.Run{model.TextR("Read the manual")})
	assert.Equal(t, model.StandingStale, held.BasisStanding(d))
	held.SetTargetRuns("en", []model.Run{model.TextR("Read the guide")})
	assert.Equal(t, model.StandingCurrent, held.BasisStanding(d), "the en edition holds the content")

	// A derivation from a tone or channel edition the block does not hold is stale.
	short := model.EditionKey{Locale: "en", Channel: "short"}
	assert.Equal(t, model.StandingStale, project.BasisStanding(model.Derivation{From: short, Rev: d.Rev}))
}
