package backend

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The terms store keys a term by its locale, so a concept added with "nb_NO"
// and searched with "nb-NO" has to be the same term rather than two.
func TestTerms_CanonicalizesLocaleArguments(t *testing.T) {
	app := newTestApp(t)
	handle := openTestTerms(t, app)

	require.NoError(t, app.AddConcept(handle, AddConceptRequest{
		Domain:     "e-commerce",
		Definition: "A temporary container for goods before purchase.",
		Terms: []TermDTO{
			{Text: "shopping cart", Locale: "en", Status: "preferred"},
			{Text: "handlekurv", Locale: "nb_NO", Status: "preferred"},
		},
	}))

	res := app.SearchTerms(handle, "shopping", "en", "nb-NO", 0, 10)
	require.Len(t, res.Concepts, 1)

	var locales []string
	for _, term := range res.Concepts[0].Terms {
		locales = append(locales, term.Locale)
	}
	assert.Contains(t, locales, "nb-NO", "nb_NO must be stored as nb-NO")
	assert.NotContains(t, locales, "nb_NO")

	// Either spelling reaches the same concept.
	for _, spelling := range []string{"nb-NO", "nb_NO", "NB-no"} {
		assert.Lenf(t, app.SearchTerms(handle, "shopping", "en", spelling, 0, 10).Concepts, 1,
			"searching target %q", spelling)
	}
}

func TestTerms_RefusesALocaleThatNamesNothing(t *testing.T) {
	app := newTestApp(t)
	handle := openTestTerms(t, app)

	err := app.AddConcept(handle, AddConceptRequest{
		Domain: "e-commerce",
		Terms:  []TermDTO{{Text: "cart", Locale: "not a locale", Status: "preferred"}},
	})
	require.Error(t, err, "a term in a language nothing names must not be stored")
	assert.Contains(t, err.Error(), "invalid locale")

	assert.Empty(t, app.SearchTerms(handle, "cart", "en", "not a locale", 0, 10).Concepts)
}
