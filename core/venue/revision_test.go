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
// project read does, so its basis is the one a checkout records.
func projectRead(b *model.Block, source model.LocaleID) *model.Block {
	cp := *b
	cp.SetSourceRuns(b.SourceRuns())
	cp.SourceLocale = source
	return &cp
}

// The venue's revision of a source is the basis a checkout records for the
// same content: a project read files the block under the project's language,
// and so does the venue, whatever language the format's reader declared.
func TestSourceRevision_IsTheBasisAProjectReadRecords(t *testing.T) {
	for _, declared := range []model.LocaleID{"", "en", "en-GB"} {
		b := linkBlock("/v1/guide", declared)
		basis := state.ReadSource(projectRead(b, "en-US"), "en-US").Basis

		assert.Equal(t, basis, venue.SourceRevision(b, "en-US"), "reader declared %q", declared)
	}

	stored := linkBlock("/v1/guide", "")
	assert.True(t, state.ReadSource(projectRead(stored, "en-US"), "en-US").HoldsBasis(venue.SourceRevision(stored, "en-US")),
		"a basis a venue records is one a checkout holding the content reads as current")
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

// A block holding a translation under the source language itself knows its
// source by the zero key, and the venue takes the revision as a checkout does.
func TestSourceRevision_SameLanguageTranslation(t *testing.T) {
	b := linkBlock("/v1/guide", "")
	b.SetTargetEdition(model.Variant("en"), model.Edition{Runs: []model.Run{model.TextR("Read the guide")}})

	assert.Equal(t, state.ReadSource(projectRead(b, "en"), "en").Basis, venue.SourceRevision(b, "en"))
	assert.Equal(t, model.RunsRevision(model.EditionKey{}, b.SourceRuns()), venue.SourceRevision(b, "en"))
}

// The push carries a block to the venue as a proto and the venue takes its
// revision of the block it decodes. Every run kind survives the trip, so the
// revision the venue stamps is the one the checkout took.
func TestSourceRevision_SurvivesThePushWire(t *testing.T) {
	b := venuetest.KitchenSinkBlock()
	back, err := venue.ProtoToBlock(venue.BlockToProto(b, "kitchen.json"))
	require.NoError(t, err)

	assert.Equal(t, venue.SourceRevision(b, "en"), venue.SourceRevision(back, "en"))
	assert.Equal(t, venue.RecordHash(b, "en"), venue.RecordHash(back, "en"))
}
