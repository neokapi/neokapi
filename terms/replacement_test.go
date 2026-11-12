package terms_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/terms"
)

// A kept decision "cart, not basket" joins the concept that declares cart.
// When that concept prefers "shopping cart", the word the person kept is
// still what to say instead of basket.
func TestReplacementPrefersTheKeptWord(t *testing.T) {
	concepts := []terms.Concept{{ID: "cart", Terms: []terms.Term{
		{Text: "shopping cart", Locale: "en", Status: model.TermPreferred},
		{Text: "cart", Locale: "en", Status: model.TermAdmitted},
	}}}
	concepts, target, _, changed := terms.UpsertDecision(concepts, terms.Decision{
		Text: "basket", Locale: "en", Status: model.TermForbidden, Replacement: "cart",
	})
	require.True(t, changed)
	require.Equal(t, 0, target, "basket joins the concept that declares cart")

	c := concepts[target]
	basket := c.Terms[terms.TermIndex(&c, "basket", "en")]
	assert.Equal(t, "cart", terms.Replacement(c, basket))

	rules := terms.SourceWordRules(concepts, "en")
	require.Len(t, rules, 1)
	assert.Equal(t, "basket", rules[0].Term)
	assert.Equal(t, "cart", rules[0].Replacement)
}

// A discouraged term that names nothing of its own takes the concept's
// preferred term; one with neither has no replacement.
func TestReplacementFallsBackToThePreferredTerm(t *testing.T) {
	c := terms.Concept{ID: "leverage", Terms: []terms.Term{
		{Text: "leverage", Locale: "en", Status: model.TermForbidden},
	}}
	assert.Empty(t, terms.Replacement(c, c.Terms[0]))

	c.Terms = append(c.Terms, terms.Term{Text: "rely on", Locale: "en", Status: model.TermPreferred})
	assert.Equal(t, "rely on", terms.Replacement(c, c.Terms[0]))

	c.Terms[0].Note = terms.ReplacementNote("use")
	assert.Equal(t, "use", terms.Replacement(c, c.Terms[0]), "the term's own note comes first")
}
