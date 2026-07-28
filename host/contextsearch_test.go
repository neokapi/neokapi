package host_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/neokapi/neokapi/core/graph"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/host"
	"github.com/neokapi/neokapi/memory"
	"github.com/neokapi/neokapi/terms"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The retrieval surface's contract (AD-037): one question, answered from every
// bound store, grouped by kind, and honest about what it could not reach.

// fakeTerms is a terms.Terminology that returns a fixed concept set, so these
// tests exercise the projection rather than a store implementation.
type fakeTerms struct {
	terms.Terminology
	concepts []terms.Concept
	err      error
}

func (f fakeTerms) Search(context.Context, string, model.LocaleID, model.LocaleID, int, int) ([]terms.Concept, int, error) {
	if f.err != nil {
		return nil, 0, f.err
	}
	return f.concepts, len(f.concepts), nil
}

type fakeMemory struct {
	memory.Store
	entries []memory.Entry
	err     error
}

func (f fakeMemory) SearchEntries(context.Context, memory.SearchParams) ([]memory.Entry, int, error) {
	if f.err != nil {
		return nil, 0, f.err
	}
	return f.entries, len(f.entries), nil
}

// renameConcept is the case the whole model exists to serve: a term was
// retired, another is preferred, and the retired one stays recognisable for a
// while rather than forever.
func renameConcept() terms.Concept {
	until := time.Date(2027, 3, 1, 0, 0, 0, 0, time.UTC)
	return terms.Concept{
		ID:         "c-widget",
		Domain:     "product",
		Definition: "The unit users place on a dashboard.",
		Terms: []terms.Term{
			{Text: "gadget", Locale: model.LocaleEnglish, Status: model.TermPreferred},
			{
				Text:   "widget",
				Locale: model.LocaleEnglish,
				Status: model.TermDeprecated,
				// The old name stays recognisable until a date, which is the
				// whole point of validity: "recognise it until March" is a
				// different instruction from "never use it".
				Validity: &graph.Validity{ValidTo: &until},
			},
		},
	}
}

func TestSearchContextAnswersTheRenameQuestion(t *testing.T) {
	src := host.ContextSearchSources{
		Terms: fakeTerms{concepts: []terms.Concept{renameConcept()}},
		Scope: host.ScopeProject,
	}

	res, err := host.SearchContext(context.Background(), src, host.ContextSearchRequest{
		Query:  "widget",
		Locale: model.LocaleEnglish,
	})
	require.NoError(t, err)

	var deprecated *host.ContextTermHit
	for i := range res.Terms {
		if res.Terms[i].Term == "widget" {
			deprecated = &res.Terms[i]
		}
	}
	require.NotNil(t, deprecated, "the queried term must come back")

	// "is it discouraged" answered without knowing the status vocabulary…
	assert.True(t, deprecated.Discouraged)
	// …and "what do I say instead" answered in the same result, not a second call.
	assert.Equal(t, "gadget", deprecated.Replacement)
}

func TestSearchContextGroupsRatherThanMerges(t *testing.T) {
	src := host.ContextSearchSources{
		Terms: fakeTerms{concepts: []terms.Concept{renameConcept()}},
		Memory: fakeMemory{entries: []memory.Entry{{
			ID:          "m1",
			HintSrcLang: model.LocaleEnglish,
			Variants: map[model.LocaleID][]model.Run{
				model.LocaleEnglish: {{Text: &model.TextRun{Text: "Drag a gadget onto the dashboard."}}},
			},
		}}},
		Scope: host.ScopeProject,
	}

	res, err := host.SearchContext(context.Background(), src, host.ContextSearchRequest{Query: "gadget"})
	require.NoError(t, err)

	// Two kinds, each in its own group. A term match and a memory match score on
	// different things, so there is deliberately no single ranked list to assert.
	assert.NotEmpty(t, res.Terms, "terminology group")
	assert.NotEmpty(t, res.Precedent, "precedent group")
	assert.Equal(t, "Drag a gadget onto the dashboard.", res.Precedent[0].Text)
}

// TestSearchContextSaysWhatItCouldNotReach is the guard against the defect
// AD-037 records: a partial answer that reads as a whole one. An unbound store
// must be visible in the result, or "nothing found" is ambiguous between "no
// answer" and "nowhere to look".
func TestSearchContextSaysWhatItCouldNotReach(t *testing.T) {
	res, err := host.SearchContext(context.Background(),
		host.ContextSearchSources{Scope: host.ScopeProject},
		host.ContextSearchRequest{Query: "widget"})
	require.NoError(t, err)

	assert.Empty(t, res.Terms)
	assert.Empty(t, res.Precedent)
	require.NotEmpty(t, res.Notes, "an empty answer must say why it is empty")

	joined := notesText(res)
	assert.Contains(t, joined, "no terms store is bound")
	assert.Contains(t, joined, "no content memory is bound")
}

// TestSearchContextDegradesOnStoreError: half an answer plus a statement of
// what broke beats an error mid-task — but the failure must never be silent.
func TestSearchContextDegradesOnStoreError(t *testing.T) {
	src := host.ContextSearchSources{
		Terms: fakeTerms{err: assert.AnError},
		Memory: fakeMemory{entries: []memory.Entry{{
			ID:          "m1",
			HintSrcLang: model.LocaleEnglish,
			Variants: map[model.LocaleID][]model.Run{
				model.LocaleEnglish: {{Text: &model.TextRun{Text: "prior wording"}}},
			},
		}}},
		Scope: host.ScopeProject,
	}

	res, err := host.SearchContext(context.Background(), src, host.ContextSearchRequest{Query: "x"})
	require.NoError(t, err, "one broken store must not fail the whole call")
	assert.NotEmpty(t, res.Precedent, "the store that worked still answers")

	joined := notesText(res)
	assert.Contains(t, joined, "terms store could not be searched")
}

// TestSearchContextReportsScope: reach, not capability. A project answer must
// declare that it is a project answer, so an empty local result is not read as
// "the organization has no view on this".
func TestSearchContextReportsScope(t *testing.T) {
	res, err := host.SearchContext(context.Background(),
		host.ContextSearchSources{Scope: host.ScopeProject},
		host.ContextSearchRequest{Query: "widget"})
	require.NoError(t, err)
	assert.Equal(t, host.ScopeProject, res.Scope)

	joined := notesText(res)
	assert.Contains(t, joined, "project scope")
}

// TestSearchContextPrecedentSkipsOtherLanguages: offering another language's
// text as precedent to someone authoring in English would be actively
// misleading, so an entry with no variant in the wanted language is dropped.
func TestSearchContextPrecedentSkipsOtherLanguages(t *testing.T) {
	src := host.ContextSearchSources{
		Memory: fakeMemory{entries: []memory.Entry{{
			ID:          "m-fr",
			HintSrcLang: model.LocaleFrench,
			Variants: map[model.LocaleID][]model.Run{
				model.LocaleFrench: {{Text: &model.TextRun{Text: "Faites glisser un gadget."}}},
			},
		}}},
		Scope: host.ScopeProject,
	}

	res, err := host.SearchContext(context.Background(), src, host.ContextSearchRequest{
		Query:  "gadget",
		Locale: model.LocaleEnglish,
	})
	require.NoError(t, err)
	assert.Empty(t, res.Precedent, "a French-only entry is not English precedent")
}

func TestSearchContextRejectsEmptyQuery(t *testing.T) {
	_, err := host.SearchContext(context.Background(),
		host.ContextSearchSources{Scope: host.ScopeProject},
		host.ContextSearchRequest{})
	require.Error(t, err)
}

// notesText joins a result's notes for substring assertions. A helper rather
// than a loop per test: the linter is right that repeated string concatenation
// in a loop is wasteful, and three copies of it was three chances to diverge.
func notesText(res *host.ContextSearchResult) string {
	return strings.Join(res.Notes, "\n")
}
