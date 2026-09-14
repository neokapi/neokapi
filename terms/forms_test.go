package terms_test

import (
	"context"
	"slices"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/terms"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTermSurfaces(t *testing.T) {
	term := terms.Term{Text: "varsel", Forms: []string{" varsler ", "", "Varsel", "varslene", "varsler"}}
	assert.Equal(t, []string{"varsel", "varsler", "varslene"}, term.Surfaces())
	assert.Equal(t, []string{"varsler", "varslene"}, terms.NormalizeForms(term.Text, term.Forms))
	assert.Nil(t, terms.NormalizeForms("varsel", []string{"Varsel", " "}))
	assert.Nil(t, terms.NormalizeForms("varsel", nil))
}

func TestFormsColumnRoundTrip(t *testing.T) {
	assert.Equal(t, "[]", terms.FormsColumn(nil))
	got, err := terms.FormsFromColumn(terms.FormsColumn([]string{"Liegeplätze", "Liegeplatzes"}))
	require.NoError(t, err)
	assert.Equal(t, []string{"Liegeplätze", "Liegeplatzes"}, got)

	for _, empty := range []string{"", "[]"} {
		got, err := terms.FormsFromColumn(empty)
		require.NoError(t, err)
		assert.Nil(t, got)
	}
	_, err = terms.FormsFromColumn("{not json")
	assert.Error(t, err)
}

// alertConcepts is a Norwegian rendering whose plural drops a vowel, so no
// spelling of the term contains the form, and an English source term whose
// declared plural is the only way "Alerts" is a use of it.
func alertConcepts() []terms.Concept {
	return []terms.Concept{
		{
			ID: "alert",
			Terms: []terms.Term{
				{Text: "alert", Locale: model.LocaleEnglish, Status: model.TermPreferred, Forms: []string{"alerts"}},
				{Text: "varsel", Locale: "nb", Status: model.TermPreferred, Forms: []string{"varsler", "varselet", "varslene"}},
			},
		},
		{
			ID: "alert-rule",
			Terms: []terms.Term{
				{Text: "alert rule", Locale: model.LocaleEnglish, Status: model.TermPreferred, Forms: []string{"alert rules"}},
			},
		},
	}
}

func forEachBackend(t *testing.T, fn func(t *testing.T, tb terms.Terminology)) {
	t.Run("in-memory", func(t *testing.T) {
		fn(t, terms.NewInMemoryStore())
	})
	t.Run("sqlite", func(t *testing.T) {
		tb, err := terms.NewSQLiteStore(":memory:")
		require.NoError(t, err)
		defer tb.Close()
		fn(t, tb)
	})
}

func TestLookupAll_MatchesDeclaredForms(t *testing.T) {
	forEachBackend(t, func(t *testing.T, tb terms.Terminology) {
		ctx := context.Background()
		for _, c := range alertConcepts() {
			require.NoError(t, tb.AddConcept(ctx, c))
		}

		text := "Alerts fire when two alert rules match."
		matches := mustLookupAll(t, tb, text, terms.LookupOptions{SourceLocale: model.LocaleEnglish})
		type hit struct{ concept, term, surface string }
		var got []hit
		for _, m := range matches {
			got = append(got, hit{m.Concept.ID, m.Term.Text, text[m.Position.Start:m.Position.End]})
		}
		// The form reports the declared term, and "alert rules" is one use of the
		// longer concept rather than also a use of "alert" inside it.
		assert.Equal(t, []hit{
			{"alert", "alert", "Alerts"},
			{"alert-rule", "alert rule", "alert rules"},
		}, got)

		nb := mustLookupAll(t, tb, "Lese varsler", terms.LookupOptions{SourceLocale: "nb"})
		require.Len(t, nb, 1)
		assert.Equal(t, "varsel", nb[0].Term.Text)
		assert.Equal(t, []string{"varsler", "varselet", "varslene"}, nb[0].Term.Forms)
	})
}

func TestLookupAll_FormsKeepWordBoundaries(t *testing.T) {
	forEachBackend(t, func(t *testing.T, tb terms.Terminology) {
		require.NoError(t, tb.AddConcept(context.Background(), terms.Concept{
			ID:    "use",
			Terms: []terms.Term{{Text: "use", Locale: model.LocaleEnglish, Forms: []string{"uses", "used"}}},
		}))
		assert.Empty(t, mustLookupAll(t, tb, "Every user is unused.", terms.LookupOptions{SourceLocale: model.LocaleEnglish}))
		assert.Len(t, mustLookupAll(t, tb, "It uses what it used.", terms.LookupOptions{SourceLocale: model.LocaleEnglish}), 2)
	})
}

func TestConceptForms_PersistAndNormalize(t *testing.T) {
	forEachBackend(t, func(t *testing.T, tb terms.Terminology) {
		ctx := context.Background()
		require.NoError(t, tb.AddConcept(ctx, terms.Concept{
			ID: "berth",
			Terms: []terms.Term{
				{Text: "Liegeplatz", Locale: "de", Status: model.TermPreferred, Forms: []string{"Liegeplätze", " liegeplatz ", "", "Liegeplätze"}},
				{Text: "berth", Locale: model.LocaleEnglish, Status: model.TermPreferred},
			},
		}))

		c, ok := mustGetConcept(t, tb, "berth")
		require.True(t, ok)
		de := c.TargetTerms("de")
		require.Len(t, de, 1)
		assert.Equal(t, []string{"Liegeplätze"}, de[0].Forms, "blanks, repeats and the term itself are not stored as forms")
		en := c.TargetTerms(model.LocaleEnglish)
		require.Len(t, en, 1)
		assert.Nil(t, en[0].Forms, "a term with no forms reads back with none")

		all, err := tb.Concepts(ctx)
		require.NoError(t, err)
		require.Len(t, all, 1)
		assert.Equal(t, []string{"Liegeplätze"}, all[0].TargetTerms("de")[0].Forms)
	})
}

func TestConceptForms_CallerSliceUntouched(t *testing.T) {
	forms := []string{"varsler", " varsler "}
	in := terms.Concept{ID: "c", Terms: []terms.Term{{Text: "varsel", Locale: "nb", Forms: forms}}}
	out := terms.NormalizedConcept(in)
	assert.Equal(t, []string{"varsler"}, out.Terms[0].Forms)
	assert.Equal(t, []string{"varsler", " varsler "}, in.Terms[0].Forms, "the caller's forms are not rewritten under it")
}

func TestLocate_StoreOccurrenceUnderDeclaredForm(t *testing.T) {
	tb := terms.NewInMemoryStore()
	for _, c := range alertConcepts() {
		require.NoError(t, tb.AddConcept(context.Background(), c))
	}
	occ, err := terms.Locate(context.Background(), terms.LocateRequest{
		Text:   "Two alerts",
		Store:  tb,
		Locale: model.LocaleEnglish,
	})
	require.NoError(t, err)
	require.Len(t, occ, 1)
	assert.Equal(t, "alerts", occ[0].Text)
	assert.Equal(t, "alert", occ[0].Term)
	assert.Equal(t, "alert", occ[0].ConceptID)
}

// lookupModes asks the exact and normalized tiers, which is what `kapi terms
// lookup` asks without --fuzzy. The fuzzy tier is left out so a near miss
// cannot be found by edit distance instead.
var lookupModes = []model.MatchStrategy{model.MatchStrategyExact, model.MatchStrategyNormalized}

// lookupConcepts adds the alert concepts, a term that declares forms, and an
// English term that declares none.
func lookupConcepts(t *testing.T, tb terms.Terminology) {
	t.Helper()
	ctx := context.Background()
	for _, c := range alertConcepts() {
		require.NoError(t, tb.AddConcept(ctx, c))
	}
	require.NoError(t, tb.AddConcept(ctx, terms.Concept{
		ID:    "use",
		Terms: []terms.Term{{Text: "use", Locale: model.LocaleEnglish, Forms: []string{"uses", "used"}}},
	}))
	require.NoError(t, tb.AddConcept(ctx, terms.Concept{
		ID:    "save",
		Terms: []terms.Term{{Text: "save", Locale: model.LocaleEnglish}},
	}))
}

// A term typed as one of its declared forms finds the term, as an exact match
// that reports the term rather than the form.
func TestLookup_FindsATermUnderADeclaredForm(t *testing.T) {
	forEachBackend(t, func(t *testing.T, tb terms.Terminology) {
		lookupConcepts(t, tb)
		for _, tc := range []struct {
			query   string
			locale  model.LocaleID
			concept string
			term    string
		}{
			{"alerts", model.LocaleEnglish, "alert", "alert"},
			{"Alerts", model.LocaleEnglish, "alert", "alert"},
			{"alert rules", model.LocaleEnglish, "alert-rule", "alert rule"},
			{"used", model.LocaleEnglish, "use", "use"},
			{"varsler", "nb", "alert", "varsel"},
			{"varslene", "nb", "alert", "varsel"},
		} {
			t.Run(tc.query, func(t *testing.T) {
				matches, err := tb.Lookup(context.Background(), tc.query, terms.LookupOptions{SourceLocale: tc.locale, MatchModes: lookupModes})
				require.NoError(t, err)
				require.Len(t, matches, 1)
				assert.Equal(t, tc.concept, matches[0].Concept.ID)
				assert.Equal(t, tc.term, matches[0].Term.Text)
				assert.Equal(t, model.MatchStrategyExact, matches[0].MatchType)
				assert.InDelta(t, 1.0, matches[0].Score, 1e-9)
			})
		}
	})
}

func TestLookup_DesignationStillMatches(t *testing.T) {
	forEachBackend(t, func(t *testing.T, tb terms.Terminology) {
		lookupConcepts(t, tb)
		for _, tc := range []struct {
			query   string
			locale  model.LocaleID
			concept string
		}{
			{"alert", model.LocaleEnglish, "alert"},
			{"alert rule", model.LocaleEnglish, "alert-rule"},
			{"save", model.LocaleEnglish, "save"},
			{"varsel", "nb", "alert"},
		} {
			t.Run(tc.query, func(t *testing.T) {
				matches, err := tb.Lookup(context.Background(), tc.query, terms.LookupOptions{SourceLocale: tc.locale, MatchModes: lookupModes})
				require.NoError(t, err)
				require.Len(t, matches, 1)
				assert.Equal(t, tc.concept, matches[0].Concept.ID)
				assert.Equal(t, model.MatchStrategyExact, matches[0].MatchType)
			})
		}
	})
}

// A spelling a term does not declare is not a use of it: a word that contains
// the term, an inflection the term does not list, a passage that holds a form
// among other words, and an English term with no declared forms asked for by
// an inflection.
func TestLookup_NearMissIsNotAForm(t *testing.T) {
	forEachBackend(t, func(t *testing.T, tb terms.Terminology) {
		lookupConcepts(t, tb)
		for _, tc := range []struct {
			query  string
			locale model.LocaleID
		}{
			{"user", model.LocaleEnglish},
			{"unused", model.LocaleEnglish},
			{"alerting", model.LocaleEnglish},
			{"alerts fire", model.LocaleEnglish},
			{"saves", model.LocaleEnglish},
			{"varslet", "nb"},
		} {
			t.Run(tc.query, func(t *testing.T) {
				matches, err := tb.Lookup(context.Background(), tc.query, terms.LookupOptions{SourceLocale: tc.locale, MatchModes: lookupModes})
				require.NoError(t, err)
				assert.Empty(t, matches)
			})
		}
	})
}

// Lookup and LookupAll read a term by one rule. For a query that is a single
// term, the terms Lookup finds exactly are the terms LookupAll finds spanning
// the whole query.
func TestLookup_AgreesWithLookupAll(t *testing.T) {
	forEachBackend(t, func(t *testing.T, tb terms.Terminology) {
		lookupConcepts(t, tb)
		found := 0
		for _, tc := range []struct {
			query  string
			locale model.LocaleID
		}{
			{"alert", model.LocaleEnglish},
			{"Alerts", model.LocaleEnglish},
			{"alert rule", model.LocaleEnglish},
			{"alert rules", model.LocaleEnglish},
			{"alerting", model.LocaleEnglish},
			{"uses", model.LocaleEnglish},
			{"user", model.LocaleEnglish},
			{"save", model.LocaleEnglish},
			{"saves", model.LocaleEnglish},
			{"varsler", "nb"},
			{"varslet", "nb"},
		} {
			t.Run(tc.query, func(t *testing.T) {
				opts := terms.LookupOptions{SourceLocale: tc.locale, MatchModes: []model.MatchStrategy{model.MatchStrategyExact}}
				looked, err := tb.Lookup(context.Background(), tc.query, opts)
				require.NoError(t, err)
				var fromLookup []string
				for _, m := range looked {
					fromLookup = append(fromLookup, m.Concept.ID+"/"+m.Term.Text)
				}
				var fromLookupAll []string
				for _, m := range mustLookupAll(t, tb, tc.query, opts) {
					if m.Position.Start == 0 && m.Position.End == len(tc.query) {
						fromLookupAll = append(fromLookupAll, m.Concept.ID+"/"+m.Term.Text)
					}
				}
				slices.Sort(fromLookup)
				slices.Sort(fromLookupAll)
				assert.Equal(t, fromLookupAll, fromLookup)
				found += len(fromLookup)
			})
		}
		assert.Positive(t, found, "the agreement must be over queries that find something")
	})
}
