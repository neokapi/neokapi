//go:build integration

package terms_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/terms"
)

// TestPostgresTerms_KeepsAdvisory mirrors the framework store: an advisory
// concept reads back with its flag from every read path, and the word rules it
// imposes on source content report without failing.
func TestPostgresTerms_KeepsAdvisory(t *testing.T) {
	tb := openTestPostgresTerms(t)
	ctx := context.Background()

	require.NoError(t, tb.AddConcept(ctx, terms.Concept{
		ID:       "use",
		Advisory: true,
		Terms: []terms.Term{
			{Text: "use", Locale: model.LocaleEnglish, Status: model.TermPreferred},
			{Text: "utilize", Locale: model.LocaleEnglish, Status: model.TermForbidden},
		},
	}))
	require.NoError(t, tb.AddConcept(ctx, terms.Concept{
		ID: "sign-in",
		Terms: []terms.Term{
			{Text: "sign in", Locale: model.LocaleEnglish, Status: model.TermPreferred},
			{Text: "login", Locale: model.LocaleEnglish, Status: model.TermForbidden},
		},
	}))

	c, ok, err := tb.GetConcept(ctx, "use")
	require.NoError(t, err)
	require.True(t, ok)
	assert.True(t, c.Advisory, "GetConcept keeps the flag")

	all, err := tb.Concepts(ctx)
	require.NoError(t, err)
	flags := map[string]bool{}
	for _, concept := range all {
		flags[concept.ID] = concept.Advisory
	}
	assert.Equal(t, map[string]bool{"use": true, "sign-in": false}, flags, "Concepts keeps the flag")

	matches, err := tb.LookupAll(ctx, "We utilize it", terms.LookupOptions{SourceLocale: model.LocaleEnglish})
	require.NoError(t, err)
	require.NotEmpty(t, matches)
	assert.True(t, matches[0].Concept.Advisory, "a lookup hydrates the flag")

	rules := terms.SourceWordRules(all, model.LocaleEnglish)
	advisory := map[string]bool{}
	for _, r := range rules {
		advisory[r.Term] = r.Advisory
	}
	assert.Equal(t, map[string]bool{"utilize": true, "login": false}, advisory)
}
