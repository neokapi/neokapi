//go:build integration

package terms_test

import (
	"context"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/terms"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPostgresTerms_FormsRoundTripAndMatch mirrors the framework backends: a
// term's declared forms persist, read back normalized, and are what LookupAll
// finds the term under.
func TestPostgresTerms_FormsRoundTripAndMatch(t *testing.T) {
	tb := openTestPostgresTerms(t)
	ctx := context.Background()

	require.NoError(t, tb.AddConcept(ctx, terms.Concept{
		ID: "alert",
		Terms: []terms.Term{
			{Text: "alert", Locale: model.LocaleEnglish, Status: model.TermPreferred},
			{Text: "varsel", Locale: "nb", Status: model.TermPreferred, Forms: []string{"varsler", " varsler ", "varsel"}},
		},
	}))

	c, ok, err := tb.GetConcept(ctx, "alert")
	require.NoError(t, err)
	require.True(t, ok)
	nb := c.TargetTerms("nb")
	require.Len(t, nb, 1)
	assert.Equal(t, []string{"varsler"}, nb[0].Forms)
	assert.Nil(t, c.TargetTerms(model.LocaleEnglish)[0].Forms)

	all, err := tb.Concepts(ctx)
	require.NoError(t, err)
	require.Len(t, all, 1)
	assert.Equal(t, []string{"varsler"}, all[0].TargetTerms("nb")[0].Forms)

	matches, err := tb.LookupAll(ctx, "Lese varsler", terms.LookupOptions{SourceLocale: "nb"})
	require.NoError(t, err)
	require.Len(t, matches, 1)
	assert.Equal(t, "varsel", matches[0].Term.Text)
	assert.Equal(t, []string{"varsler"}, matches[0].Term.Forms)
}
