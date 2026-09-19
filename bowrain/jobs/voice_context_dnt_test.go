package jobs

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/model"
	coreprofile "github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/terms"
)

// A do-not-translate concept names a term and no replacement, so the server's
// derivation dropped it: recycle filled a content-memory match that translated
// a product name, the drafter was never told to keep it, and the ship gate
// then failed the target. The gate derives its rules from the framework
// derivation, which keeps such a rule, so the two disagreed about the same
// concept.
func TestTermRulesFromConcepts_KeepsDoNotTranslate(t *testing.T) {
	tb := terms.NewInMemoryStore()
	require.NoError(t, tb.AddConcept(t.Context(), terms.Concept{
		ID:             "c-kapi",
		DoNotTranslate: true,
		Terms:          []terms.Term{{Text: "kapi", Locale: "en", Status: model.TermPreferred}},
	}))

	rules, err := TermRulesFromConcepts(t.Context(), tb, "proj-1", "en", "fr")
	require.NoError(t, err)
	require.Len(t, rules, 1, "the concept governs this pair even with no fr term")
	assert.Equal(t, "kapi", rules[0].Term)
	assert.True(t, rules[0].DoNotTranslate)
	assert.Empty(t, rules[0].Replacement, "a do-not-translate rule names no replacement")
	assert.Equal(t, "c-kapi", rules[0].ConceptID)
}

// A do-not-translate concept answers for a language its store has never heard
// of: the claim is that the term is the same string everywhere.
func TestTermRulesFromConcepts_DoNotTranslateGovernsEveryLanguage(t *testing.T) {
	tb := terms.NewInMemoryStore()
	require.NoError(t, tb.AddConcept(t.Context(), terms.Concept{
		ID:             "c-kapi",
		DoNotTranslate: true,
		Terms:          []terms.Term{{Text: "kapi", Locale: "en", Status: model.TermPreferred}},
	}))

	for _, target := range []model.LocaleID{"fr", "ja", "qps"} {
		rules, err := TermRulesFromConcepts(t.Context(), tb, "proj-1", "en", target)
		require.NoError(t, err)
		require.Len(t, rules, 1, "target %s", target)
		assert.True(t, rules[0].DoNotTranslate, "target %s", target)
	}
}

// Two concepts can claim the same source term, one saying keep it and one
// naming a rendering. They are different demands, and the ship gate holds a
// target to both, so the server's derivation keeps both rather than letting one
// displace the other in a map keyed by term alone.
func TestTermRulesFromConcepts_KeepsBothClaimsOnOneTerm(t *testing.T) {
	tb := terms.NewInMemoryStore()
	require.NoError(t, tb.AddConcept(t.Context(), terms.Concept{
		ID:             "c-keep",
		DoNotTranslate: true,
		Terms:          []terms.Term{{Text: "sync", Locale: "en", Status: model.TermPreferred}},
	}))
	require.NoError(t, tb.AddConcept(t.Context(), terms.Concept{
		ID: "c-render",
		Terms: []terms.Term{
			{Text: "sync", Locale: "en", Status: model.TermPreferred},
			{Text: "synchronisation", Locale: "fr", Status: model.TermPreferred},
		},
	}))

	rules, err := TermRulesFromConcepts(t.Context(), tb, "proj-1", "en", "fr")
	require.NoError(t, err)
	require.Len(t, rules, 2, "a keep-verbatim claim and a rendering are separate demands")

	var kept, rendered bool
	for _, r := range rules {
		assert.Equal(t, "sync", r.Term)
		if r.DoNotTranslate {
			kept = true
		} else {
			rendered = true
			assert.Equal(t, "synchronisation", r.Replacement)
		}
	}
	assert.True(t, kept, "the do-not-translate claim survives")
	assert.True(t, rendered, "the rendering survives")
}

// The behaviours the derivation already had are unchanged by admitting
// do-not-translate rules: another project's concept stays out, and a concept
// that neither names a rendering nor claims the term is still nothing to
// enforce.
func TestTermRulesFromConcepts_DoNotTranslateRespectsProjectScope(t *testing.T) {
	tb := terms.NewInMemoryStore()
	require.NoError(t, tb.AddConcept(t.Context(), terms.Concept{
		ID:             "c-other",
		ProjectID:      "proj-OTHER",
		DoNotTranslate: true,
		Terms:          []terms.Term{{Text: "Bowrain", Locale: "en", Status: model.TermPreferred}},
	}))
	require.NoError(t, tb.AddConcept(t.Context(), terms.Concept{
		ID:             "c-mine",
		ProjectID:      "proj-1",
		DoNotTranslate: true,
		Terms:          []terms.Term{{Text: "kapi", Locale: "en", Status: model.TermPreferred}},
	}))
	require.NoError(t, tb.AddConcept(t.Context(), terms.Concept{
		ID:    "c-bare",
		Terms: []terms.Term{{Text: "widget", Locale: "en", Status: model.TermPreferred}},
	}))

	rules, err := TermRulesFromConcepts(t.Context(), tb, "proj-1", "en", "fr")
	require.NoError(t, err)

	got := make([]string, 0, len(rules))
	for _, r := range rules {
		got = append(got, r.Term)
	}
	assert.Equal(t, []string{"kapi"}, got,
		"another project's concept stays out, and a concept with no rendering and no claim enforces nothing")
}

// Rules stay ordered by term, so one terms store yields one prompt and one
// fingerprint however the concepts were read.
func TestTermRulesFromConcepts_OrdersDoNotTranslateByTerm(t *testing.T) {
	tb := terms.NewInMemoryStore()
	for _, term := range []string{"neokapi", "Bowrain", "kapi"} {
		require.NoError(t, tb.AddConcept(t.Context(), terms.Concept{
			ID:             "c-" + term,
			DoNotTranslate: true,
			Terms:          []terms.Term{{Text: term, Locale: "en", Status: model.TermPreferred}},
		}))
	}

	rules, err := TermRulesFromConcepts(t.Context(), tb, "proj-1", "en", "fr")
	require.NoError(t, err)

	got := make([]string, 0, len(rules))
	for _, r := range rules {
		got = append(got, r.Term)
	}
	assert.Equal(t, []string{"Bowrain", "kapi", "neokapi"}, got)
}

// The prompt map and the keep-verbatim list are the two halves of one
// projection, and the server's rules feed both the way the CLI's do.
func TestTermRulesFromConcepts_FeedsBothProjections(t *testing.T) {
	tb := terms.NewInMemoryStore()
	require.NoError(t, tb.AddConcept(t.Context(), terms.Concept{
		ID:             "c-kapi",
		DoNotTranslate: true,
		Terms:          []terms.Term{{Text: "kapi", Locale: "en", Status: model.TermPreferred}},
	}))
	require.NoError(t, tb.AddConcept(t.Context(), terms.Concept{
		ID: "c-save",
		Terms: []terms.Term{
			{Text: "save", Locale: "en", Status: model.TermPreferred},
			{Text: "enregistrer", Locale: "fr", Status: model.TermPreferred},
		},
	}))

	rules, err := TermRulesFromConcepts(t.Context(), tb, "proj-1", "en", "fr")
	require.NoError(t, err)

	assert.Equal(t, map[string]string{"save": "enregistrer"}, coreprofile.TermRuleMap(rules))
	assert.Equal(t, []string{"kapi"}, coreprofile.DoNotTranslateTerms(rules))
}
