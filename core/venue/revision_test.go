package venue_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/state"
	"github.com/neokapi/neokapi/core/venue"
	"github.com/neokapi/neokapi/core/venue/venuetest"
)

// linkBlock is a block whose source holds a link to href, as a reader that
// declared language reads it.
func linkBlock(href string, language model.LocaleID) *model.Block {
	b := &model.Block{ID: "b1", Name: "intro", Translatable: true, SourceLocale: language}
	b.SetSourceRuns([]model.Run{
		model.TextR("Read "),
		{PcOpen: &model.PcOpenRun{ID: "1", Type: "link", Data: `<a href="` + href + `">`}},
		model.TextR("the guide"),
		{PcClose: &model.PcCloseRun{ID: "1", Type: "link", Data: "</a>"}},
	})
	return b
}

// projectRead files a copy of b under the project's source language, as a
// project read does, so its basis is the one a project read records.
func projectRead(b *model.Block, source model.LocaleID) *model.Block {
	cp := *b
	cp.SetSourceRuns(b.SourceRuns())
	cp.SourceLocale = source
	return &cp
}

// The venue's revision of a source is a basis every checkout reader of the
// same content accepts as current, whatever language the format's reader
// declared, and the one a project read records.
func TestSourceRevision_IsABasisEveryCheckoutReaderAccepts(t *testing.T) {
	for _, declared := range []model.LocaleID{"", "en", "en-GB"} {
		b := linkBlock("/v1/guide", declared)
		rev := venue.SourceRevision(b, "en-US")

		assert.Equal(t, state.ReadSource(projectRead(b, "en-US"), "en-US").Basis, rev, "reader declared %q", declared)
		assert.True(t, state.ReadSource(b, "en-US").HoldsBasis(rev),
			"a checkout whose reader declared %q reads the venue's basis as current", declared)
	}
}

// A checkout records a basis under the key its reader filed the source by,
// and a push sends it as the venue takes it: the venue's revision of the
// same source. A basis of other content, and an empty one, travel as they
// are.
func TestBasis_IsTheVenuesRevisionOfTheSameSource(t *testing.T) {
	for _, declared := range []model.LocaleID{"", "en", "en-GB"} {
		b := linkBlock("/v1/guide", declared)
		want := venue.SourceRevision(b, "en-US")
		for _, taken := range []string{
			model.EditionRevision(b, model.EditionKey{}),             // the change service, keeping the reader's language
			state.ReadSource(projectRead(b, "en-US"), "en-US").Basis, // a project read
			model.RunsRevision(model.EditionKey{}, b.SourceRuns()),   // a read under no language
		} {
			assert.Equal(t, want, venue.Basis(b, "en-US", taken), "reader declared %q", declared)
		}

		moved := venue.SourceRevision(linkBlock("/v2/guide", declared), "en-US")
		assert.Equal(t, moved, venue.Basis(b, "en-US", moved), "a basis of another source stays stale")
		assert.Empty(t, venue.Basis(b, "en-US", ""))
	}
	assert.Equal(t, "r:x", venue.Basis(nil, "en-US", "r:x"), "with no source to read, the basis travels as it is")
}

// An inline code moves the revision; the text hash cannot see it.
func TestSourceRevision_MovesWithAnInlineCode(t *testing.T) {
	before, after := linkBlock("/v1/guide", ""), linkBlock("/v2/guide", "")
	require.Equal(t, before.SourceText(), after.SourceText())

	assert.NotEqual(t, venue.SourceRevision(before, "en"), venue.SourceRevision(after, "en"))
	assert.NotEqual(t, venue.RecordHash(before, "en"), venue.RecordHash(after, "en"),
		"a push transfers a change to an inline code alone")
	assert.Equal(t, venue.RecordHash(before, "en"), venue.RecordHash(linkBlock("/v1/guide", "en-GB"), "en"),
		"the language a reader declared does not move the transfer hash")
}

// A block holding a translation under the source language itself (a
// bilingual file whose two languages are one) has one source revision on the
// venue with that translation or without it. The checkout knows that source by
// the zero key, and a push sends its basis as the venue's revision.
func TestSourceRevision_SameLanguageTranslation(t *testing.T) {
	plain := linkBlock("/v1/guide", "en")
	bilingual := linkBlock("/v1/guide", "en")
	bilingual.SetTargetEdition(model.Variant("en"), model.Edition{Runs: []model.Run{model.TextR("Read the guide")}})

	assert.Equal(t, venue.SourceRevision(plain, "en"), venue.SourceRevision(bilingual, "en"),
		"the translations a write carries do not move the source revision")
	assert.Equal(t, venue.RecordHash(plain, "en"), venue.RecordHash(bilingual, "en"))

	checkout := model.EditionRevision(bilingual, model.EditionKey{})
	assert.Equal(t, model.RunsRevision(model.EditionKey{}, bilingual.SourceRuns()), checkout,
		"the checkout takes the source under the zero key")
	assert.Equal(t, venue.SourceRevision(bilingual, "en"), venue.Basis(bilingual, "en", checkout))
	assert.True(t, state.ReadSource(bilingual, "en").HoldsBasis(venue.SourceRevision(bilingual, "en")),
		"and reads the venue's basis as current")
}

// The push carries a block to the venue as a proto and the venue takes its
// revision of the block it decodes. Every run kind survives the trip, so the
// revision the venue stamps is the one the push took.
func TestSourceRevision_SurvivesThePushWire(t *testing.T) {
	b := venuetest.KitchenSinkBlock()
	back, err := venue.ProtoToBlock(venue.BlockToProto(b, "kitchen.json"))
	require.NoError(t, err)

	assert.Equal(t, venue.SourceRevision(b, "en"), venue.SourceRevision(back, "en"))
	assert.Equal(t, venue.RecordHash(b, "en"), venue.RecordHash(back, "en"))
}
