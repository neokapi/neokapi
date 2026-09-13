package terms_test

import (
	"context"
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
