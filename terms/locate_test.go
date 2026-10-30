package terms_test

import (
	"context"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/terms"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Neither a caller's rules nor the bound store see a placeholder's name as an
// occurrence, and an occurrence beside one indexes the caller's text.
func TestLocate_PlaceholderNamesAreNotOccurrences(t *testing.T) {
	ctx := context.Background()
	store := terms.NewInMemoryStore()
	require.NoError(t, store.AddConcept(ctx, terms.Concept{
		ID: "vessel",
		Terms: []terms.Term{
			{Text: "vessel", Locale: model.LocaleEnglish, Status: model.TermPreferred},
			{Text: "fartøy", Locale: "nb", Status: model.TermPreferred},
		},
	}))
	rules := []profile.TermRuleSet{{Rules: []profile.TermRule{{Term: "vessel", Replacement: "fartøy"}}}}

	locate := func(text string) []terms.Occurrence {
		t.Helper()
		got, err := terms.Locate(ctx, terms.LocateRequest{
			Text:     text,
			RuleSets: rules,
			Store:    store,
			Locale:   model.LocaleEnglish,
		})
		require.NoError(t, err)
		return got
	}

	assert.Empty(t, locate("{vessel} is alongside until {until}."))

	text := "The {vessel} name and the vessel itself"
	got := locate(text)
	require.Len(t, got, 2, "one occurrence from the rule, one from the store")
	for _, o := range got {
		assert.Equal(t, "vessel", o.Text, "%s occurrence", o.Source)
		assert.Equal(t, strings.LastIndex(text, "vessel"), o.Start, "%s occurrence", o.Source)
	}
}

// A store term whose note only names its replacement reads once in the
// finding: the replacement is the suggestion, and the note adds nothing.
func TestLocate_ReplacementNoteReadsOnce(t *testing.T) {
	ctx := context.Background()
	store := terms.NewInMemoryStore()
	require.NoError(t, store.AddConcept(ctx, terms.Concept{
		ID: "utilise",
		Terms: []terms.Term{{
			Text: "utilise", Locale: model.LocaleEnglish, Status: model.TermForbidden,
			Note: terms.ReplacementNote("use"),
		}},
	}))
	got, err := terms.Locate(ctx, terms.LocateRequest{
		Text: "Utilise the import.", Store: store, Locale: model.LocaleEnglish,
	})
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "use", got[0].Replacement)
	assert.Empty(t, got[0].Note)

	findings := profile.HitsToFindings([]profile.VocabHit{got[0].Hit()}, "Utilise the import.", nil)
	require.Len(t, findings, 1)
	assert.Equal(t, `Forbidden term "utilise" found`, findings[0].Message)
	assert.Equal(t, `Use "use" instead`, findings[0].Suggestion)

	assert.Equal(t, "say what the reader does", terms.UsageNote("say what the reader does"))
}

// A term written against a character reference is an occurrence for the rules
// and for the store alike, and each indexes the block's text, where the
// reference has no width, and anchors in the text run that holds the word.
func TestLocate_CharacterReferencesEndAWord(t *testing.T) {
	ctx := context.Background()
	store := terms.NewInMemoryStore()
	require.NoError(t, store.AddConcept(ctx, terms.Concept{
		ID:    "vessel",
		Terms: []terms.Term{{Text: "vessel", Locale: model.LocaleEnglish, Status: model.TermPreferred}},
	}))
	rules := []profile.TermRuleSet{{Rules: []profile.TermRule{{Term: "vessel", Replacement: "ship"}}}}
	ref := func(id, data string) model.Run {
		return model.PhR(model.PlaceholderRun{ID: id, Type: "code:entity", Data: data})
	}
	cases := []struct {
		name string
		runs []model.Run
		at   int
	}{
		{"before", []model.Run{model.TextR("the vessel"), ref("1", "&amp;"), model.TextR("crew")}, 4},
		{"after", []model.Run{model.TextR("the"), ref("1", "&nbsp;"), model.TextR("vessel sails")}, 3},
		{"between", []model.Run{model.TextR("a"), ref("1", "&mdash;"), model.TextR("vessel"), ref("2", "&#160;"), model.TextR("b")}, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			text := model.RunsText(tc.runs)
			got, err := terms.Locate(ctx, terms.LocateRequest{Text: text, Runs: tc.runs, RuleSets: rules, Store: store, Locale: model.LocaleEnglish})
			require.NoError(t, err)
			require.Len(t, got, 2, "one occurrence from the rule, one from the store")
			for _, o := range got {
				assert.Equal(t, "vessel", o.Text, "%s occurrence", o.Source)
				assert.Equal(t, tc.at, o.Start, "%s occurrence", o.Source)
				assert.Equal(t, model.RangeAnchorForBytes(tc.runs, tc.at, tc.at+len("vessel")), o.Anchor, "%s occurrence", o.Source)
			}
		})
	}
}

// An occurrence whose last letter a character reference spells is quoted as
// the page shows it and carries no anchor, from the rules and from the store.
func TestLocate_AReferenceAtAnEdgeIsSpelledWithNoAnchor(t *testing.T) {
	ctx := context.Background()
	store := terms.NewInMemoryStore()
	require.NoError(t, store.AddConcept(ctx, terms.Concept{
		ID:    "cafe",
		Terms: []terms.Term{{Text: "café", Locale: model.LocaleEnglish, Status: model.TermPreferred}},
	}))
	rules := []profile.TermRuleSet{{Rules: []profile.TermRule{{Term: "café", Replacement: "coffee shop"}}}}
	runs := []model.Run{model.TextR("the caf"), model.PhR(model.PlaceholderRun{ID: "1", Type: "code:entity", Data: "&eacute;"}), model.TextR(" menu")}
	got, err := terms.Locate(ctx, terms.LocateRequest{Text: model.RunsText(runs), Runs: runs, RuleSets: rules, Store: store, Locale: model.LocaleEnglish})
	require.NoError(t, err)
	require.Len(t, got, 2, "one occurrence from the rule, one from the store")
	for _, o := range got {
		assert.True(t, o.Spelled, "%s occurrence", o.Source)
		assert.Equal(t, "café", o.Text, "%s occurrence", o.Source)
		assert.Equal(t, model.Anchor{}, o.Anchor, "%s occurrence", o.Source)
		assert.Equal(t, "café", o.Hit().Spelled, "%s occurrence", o.Source)
	}
}
