package voicescope

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/terms"
)

func TestWordRules_ReadsOneLanguageOrEvery(t *testing.T) {
	ctx := t.Context()
	tb := terms.NewInMemoryStore()
	require.NoError(t, tb.AddConcept(ctx, terms.Concept{ID: "use", Terms: []terms.Term{
		{Text: "use", Locale: "en", Status: model.TermPreferred},
		{Text: "utilize", Locale: "en", Status: model.TermForbidden},
		{Text: "utiliser", Locale: "fr", Status: model.TermForbidden},
	}}))
	require.NoError(t, tb.AddConcept(ctx, terms.Concept{ID: "globex", Terms: []terms.Term{
		{Text: "Globex", Locale: "en", Status: model.TermForbidden, CompetitorTerm: true},
	}}))

	en, err := WordRules(ctx, tb, "en-US")
	require.NoError(t, err)
	require.Len(t, en, 2)
	assert.Equal(t, "utilize", en[0].Term)
	assert.Equal(t, "use", en[0].Replacement)

	all, err := WordRules(ctx, tb, "")
	require.NoError(t, err)
	assert.Len(t, all, 3, "every language, each rule once")

	preferred, forbidden, competitor := WordRuleCounts(all)
	assert.Equal(t, 1, preferred)
	assert.Equal(t, 2, forbidden)
	assert.Equal(t, 1, competitor)

	none, err := WordRules(ctx, nil, "en")
	require.NoError(t, err)
	assert.Empty(t, none)
	assert.Empty(t, WordRuleSets(none))
}
