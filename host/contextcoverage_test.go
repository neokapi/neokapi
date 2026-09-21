package host_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/host"
	"github.com/neokapi/neokapi/memory"
	"github.com/neokapi/neokapi/terms"
)

// What an answer with nothing in it says.
//
// A project that has recorded nothing is the ordinary first hour. The answer
// for a location in one used to be four lines of caveats, which an assistant
// read as "there is nothing here" and carried on without the project's context
// for the rest of the session. These hold the answer to saying, plainly, that
// it is empty and what is worth noticing while the work is done.

func TestContextPointCoverageCountsWhatStandsBehindTheAnswer(t *testing.T) {
	proj := pointRecipe(t, "")
	req := host.ContextPointRequest{Path: "docs/guide.md"}

	tests := []struct {
		name string
		src  host.ContextPointSources
		want host.ContextCoverage
	}{
		{
			name: "no voice and no terms is empty",
			src:  host.ContextPointSources{},
			want: host.CoverageEmpty,
		},
		{
			name: "a voice and no terms is thin",
			src:  host.ContextPointSources{Voice: pointVoice()},
			want: host.CoverageThin,
		},
		{
			name: "terms and no voice is thin",
			src:  host.ContextPointSources{Concepts: pointConcepts()},
			want: host.CoverageThin,
		},
		{
			name: "both is covered",
			src:  host.ContextPointSources{Voice: pointVoice(), Concepts: pointConcepts()},
			want: host.CoverageCovered,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			res := resolveAt(t, proj, req, tc.src)
			assert.Equal(t, tc.want, res.Coverage)
		})
	}
}

func TestContextPointTeachesWhenItIsEmpty(t *testing.T) {
	proj := pointRecipe(t, "")
	res := resolveAt(t, proj, host.ContextPointRequest{Path: "docs/guide.md"}, host.ContextPointSources{})
	require.Equal(t, host.CoverageEmpty, res.Coverage)

	notes := strings.Join(res.Notes, "\n")
	assert.Contains(t, notes, "records nothing for this location",
		"an empty answer says so rather than leaving a caller to infer it")
	assert.Contains(t, notes, "who the text addresses",
		"and says what is worth noticing, so the next answer is better than this one")

	// The teaching leads, behind nothing but a freshness note.
	assert.Contains(t, res.Notes[0], "records nothing for this location")

	// It states no rule. A project that holds none must not be handed one.
	assert.Empty(t, res.Terms)
	assert.Nil(t, res.Voice)
	assert.NotContains(t, notes, "must", "the note describes what to notice and prescribes nothing")

	text := renderAnswer(t, res)
	assert.Contains(t, text, "records nothing for this location",
		"the text rendering carries the same statement as the JSON")
}

func TestContextPointSaysNothingExtraWhenItIsCovered(t *testing.T) {
	proj := pointRecipe(t, "")
	res := resolveAt(t, proj, host.ContextPointRequest{Path: "docs/guide.md"},
		host.ContextPointSources{Voice: pointVoice(), Concepts: pointConcepts()})

	require.Equal(t, host.CoverageCovered, res.Coverage)
	for _, note := range res.Notes {
		assert.NotContains(t, note, "who the text addresses",
			"an answer with context behind it does not lecture the caller about collecting some")
	}
}

func TestContextSearchCoverageCountsWhatItFound(t *testing.T) {
	tests := []struct {
		name string
		src  host.ContextSearchSources
		want host.ContextCoverage
	}{
		{name: "nothing found is empty", src: host.ContextSearchSources{}, want: host.CoverageEmpty},
		{
			name: "terms alone is thin",
			src: host.ContextSearchSources{
				Terms: fakeTerms{concepts: []terms.Concept{renameConcept()}},
			},
			want: host.CoverageThin,
		},
		{
			name: "terms and prior wording is covered",
			src: host.ContextSearchSources{
				Terms: fakeTerms{concepts: []terms.Concept{renameConcept()}},
				Memory: fakeMemory{entries: []memory.Entry{{
					ID:          "m1",
					HintSrcLang: model.LocaleEnglish,
					Variants: map[model.LocaleID][]model.Run{
						model.LocaleEnglish: {{Text: &model.TextRun{Text: "Drag a gadget onto the dashboard."}}},
					},
				}}},
			},
			want: host.CoverageCovered,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			res, err := host.SearchContext(t.Context(), tc.src, host.ContextSearchRequest{Query: "widget"})
			require.NoError(t, err)
			assert.Equal(t, tc.want, res.Coverage)
		})
	}
}

func TestContextSearchTeachesWhenItFindsNothing(t *testing.T) {
	res, err := host.SearchContext(t.Context(), host.ContextSearchSources{},
		host.ContextSearchRequest{Query: "widget"})
	require.NoError(t, err)

	require.Equal(t, host.CoverageEmpty, res.Coverage)
	notes := strings.Join(res.Notes, "\n")
	assert.Contains(t, notes, `records nothing about "widget"`,
		"the note names what was asked, so a caller holding two answers can tell them apart")
	assert.Contains(t, notes, "who the text addresses")
	assert.Empty(t, res.Terms)
	assert.Empty(t, res.Precedent)
}
