package terms

import (
	"context"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The store normalizes the locale beside the text: a term written under a POSIX
// or odd-cased spelling is the term a canonical lookup finds, and the other way
// round, on every backend.

func localeStores(t *testing.T) map[string]Terminology {
	t.Helper()
	sq, err := NewSQLiteStore(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = sq.Close() })
	return map[string]Terminology{
		"inmemory": NewInMemoryStore(),
		"sqlite":   sq,
	}
}

func TestStore_NormalizesLocaleOnWriteAndLookup(t *testing.T) {
	ctx := context.Background()
	for name, tb := range localeStores(t) {
		t.Run(name, func(t *testing.T) {
			require.NoError(t, tb.AddConcept(ctx, Concept{
				ID:         "c-berth",
				Definition: "The place a vessel ties up.",
				Terms: []Term{
					{Text: "berth", Locale: "en_US", Status: model.TermPreferred},
					{Text: "kai", Locale: "NB-no", Status: model.TermPreferred},
				},
			}))

			// Stored canonical.
			got, ok, err := tb.GetConcept(ctx, "c-berth")
			require.NoError(t, err)
			require.True(t, ok)
			var locales []model.LocaleID
			for _, term := range got.Terms {
				locales = append(locales, term.Locale)
			}
			assert.ElementsMatch(t, []model.LocaleID{"en-US", "nb-NO"}, locales)

			// Looked up in canonical form, and in every other spelling.
			for _, loc := range []model.LocaleID{"en-US", "en_US", "EN-us"} {
				matches, err := tb.Lookup(ctx, "berth", LookupOptions{SourceLocale: loc, TargetLocale: "nb_NO"})
				require.NoError(t, err)
				require.Len(t, matches, 1, "lookup in %s", loc)
				assert.Equal(t, "c-berth", matches[0].Concept.ID)

				all, err := tb.LookupAll(ctx, "Reserve a berth today", LookupOptions{SourceLocale: loc})
				require.NoError(t, err)
				require.Len(t, all, 1, "lookup-all in %s", loc)
			}

			// Search filters by locale in canonical form too.
			concepts, total, err := tb.Search(ctx, "berth", "en_US", "nb_NO", 0, 10)
			require.NoError(t, err)
			assert.Equal(t, 1, total)
			assert.Len(t, concepts, 1)
		})
	}
}

func TestConcept_AccessorsMatchLocaleLeniently(t *testing.T) {
	c := Concept{Terms: []Term{
		{Text: "berth", Locale: "en-US", Status: model.TermPreferred},
		{Text: "kai", Locale: "nb-NO", Status: model.TermPreferred},
		{Text: "kaiplass", Locale: "nb-NO", Status: model.TermAdmitted},
	}}
	require.NotNil(t, c.SourceTerm("en_US"))
	assert.Equal(t, "berth", c.SourceTerm("EN-us").Text)
	assert.Len(t, c.TargetTerms("nb_NO"), 2)
	require.NotNil(t, c.PreferredTerm("NB_no"))
	assert.Equal(t, "kai", c.PreferredTerm("NB_no").Text)
	assert.Nil(t, c.SourceTerm("de"))
}

func TestNormalizedConcept(t *testing.T) {
	in := Concept{Terms: []Term{{Text: "a", Locale: "en_US"}, {Text: "b", Locale: "nb-NO"}}}
	out := NormalizedConcept(in)
	assert.Equal(t, model.LocaleID("en-US"), out.Terms[0].Locale)
	assert.Equal(t, model.LocaleID("en_US"), in.Terms[0].Locale, "the caller's terms are not rewritten under it")

	clean := Concept{Terms: []Term{{Text: "a", Locale: "en-US"}}}
	same := NormalizedConcept(clean)
	assert.Equal(t, &clean.Terms[0], &same.Terms[0], "canonical terms are shared, not copied")
}

func TestLookupLocales_Canonical(t *testing.T) {
	assert.Equal(t, []model.LocaleID{"en-GB", "en"}, LookupLocales("en_gb"))
	assert.Equal(t, []model.LocaleID{"nb"}, LookupLocales("NB"))
	assert.Nil(t, LookupLocales(""))
}
