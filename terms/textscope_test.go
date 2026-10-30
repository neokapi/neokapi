package terms_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/terms"
)

// "Run `leverage --now`, then leverage it." as the markdown reader holds it.
func codeSpanBlock() ([]model.Run, string) {
	runs := []model.Run{
		model.TextR("Run "),
		model.PcOpenR(model.PcOpenRun{ID: "1", Type: model.TypeInlineCode, Data: "`"}),
		model.TextR("leverage --now"),
		model.PcCloseR(model.PcCloseRun{ID: "1", Type: model.TypeInlineCode, Data: "`"}),
		model.TextR(", then leverage it."),
	}
	return runs, model.RunsText(runs)
}

// A concept scoped to prose locates no occurrence inside a code span the runs
// declare, from either backend, and a rule scoped to prose held beside the
// store answers the same. A concept with no scope finds both.
func TestLocate_TextScope(t *testing.T) {
	ctx := context.Background()
	runs, text := codeSpanBlock()
	inProse := strings.LastIndex(text, "leverage")

	scoped := terms.Concept{ID: "leverage", Terms: []terms.Term{
		{Text: "leverage", Locale: model.LocaleEnglish, Status: model.TermForbidden},
		{Text: "rely on", Locale: model.LocaleEnglish, Status: model.TermPreferred},
	}}
	scoped.SetTextScope(profile.ScopeProse)

	sqlite, err := terms.NewSQLiteStore(filepath.Join(t.TempDir(), "terms.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlite.Close() })
	memory := terms.NewInMemoryStore()
	for _, store := range []terms.Terminology{sqlite, memory} {
		require.NoError(t, store.AddConcept(ctx, scoped))
	}

	for name, req := range map[string]terms.LocateRequest{
		"the SQLite store":    {Text: text, Runs: runs, Store: sqlite, Locale: model.LocaleEnglish},
		"the in-memory store": {Text: text, Runs: runs, Store: memory, Locale: model.LocaleEnglish},
		"a rule":              {Text: text, Runs: runs, RuleSets: []profile.TermRuleSet{{Rules: []profile.TermRule{{Term: "leverage", Replacement: "rely on", Scope: profile.ScopeProse}}}}},
	} {
		t.Run(name, func(t *testing.T) {
			got, err := terms.Locate(ctx, req)
			require.NoError(t, err)
			require.Len(t, got, 1, "only the use in prose")
			assert.Equal(t, inProse, got[0].Start)
			if req.Store != nil {
				assert.Equal(t, "rely on", got[0].Replacement)
			}
		})
	}

	unscoped := terms.NewInMemoryStore()
	c := scoped
	c.Properties = nil
	require.NoError(t, unscoped.AddConcept(ctx, c))
	got, err := terms.Locate(ctx, terms.LocateRequest{Text: text, Runs: runs, Store: unscoped, Locale: model.LocaleEnglish})
	require.NoError(t, err)
	assert.Len(t, got, 2, "a concept with no scope finds the use in code too")
}

// A term written in a URL is no occurrence, in the store or in a rule.
func TestLocate_URLsHoldNoOccurrence(t *testing.T) {
	ctx := context.Background()
	store := terms.NewInMemoryStore()
	require.NoError(t, store.AddConcept(ctx, terms.Concept{ID: "use", Terms: []terms.Term{
		{Text: "utilize", Locale: model.LocaleEnglish, Status: model.TermForbidden},
		{Text: "use", Locale: model.LocaleEnglish, Status: model.TermPreferred},
	}}))
	text := "See https://example.com/utilize/setup, then utilize it."
	for name, req := range map[string]terms.LocateRequest{
		"the store": {Text: text, Store: store, Locale: model.LocaleEnglish},
		"a rule":    {Text: text, RuleSets: []profile.TermRuleSet{{Rules: []profile.TermRule{{Term: "utilize", Replacement: "use"}}}}},
	} {
		t.Run(name, func(t *testing.T) {
			got, err := terms.Locate(ctx, req)
			require.NoError(t, err)
			require.Len(t, got, 1)
			assert.Equal(t, strings.LastIndex(text, "utilize"), got[0].Start)
		})
	}
}

// Two word rules naming one replacement share a concept when they are marked
// alike. Marked otherwise, the second opens a concept of its own, so the
// advisory marking of one never silences the other, whichever comes first.
func TestUpsertDecision_RulesMarkedOtherwiseKeepTheirOwnConcept(t *testing.T) {
	utilize := terms.Decision{Text: "utilize", Locale: "en", Status: model.TermForbidden, Replacement: "use"}
	leverage := terms.Decision{Text: "leverage", Locale: "en", Status: model.TermForbidden, Replacement: "use", Advisory: true}

	for name, order := range map[string][]terms.Decision{
		"unmarked first": {utilize, leverage},
		"advisory first": {leverage, utilize},
	} {
		t.Run(name, func(t *testing.T) {
			var concepts []terms.Concept
			for _, d := range order {
				concepts, _, _ = terms.UpsertDecision(concepts, d)
			}
			require.Len(t, concepts, 2)
			fails := map[string]bool{}
			for _, r := range terms.SourceWordRules(concepts, "en") {
				fails[r.Term] = !r.Advisory
				assert.Equal(t, "use", r.Replacement, "%s names the replacement", r.Term)
			}
			assert.Equal(t, map[string]bool{"utilize": true, "leverage": false}, fails)

			// Deciding the same again changes nothing.
			for _, d := range order {
				var changed bool
				concepts, _, changed = terms.UpsertDecision(concepts, d)
				assert.False(t, changed)
			}
			assert.Len(t, concepts, 2)
		})
	}

	t.Run("marked alike", func(t *testing.T) {
		var concepts []terms.Concept
		concepts, _, _ = terms.UpsertDecision(concepts, utilize)
		concepts, _, _ = terms.UpsertDecision(concepts, terms.Decision{Text: "employ", Locale: "en", Status: model.TermForbidden, Replacement: "use"})
		require.Len(t, concepts, 1, "one concept per preferred term")
		assert.Len(t, concepts[0].Terms, 3)
	})

	t.Run("a different text scope", func(t *testing.T) {
		var concepts []terms.Concept
		concepts, _, _ = terms.UpsertDecision(concepts, utilize)
		concepts, _, _ = terms.UpsertDecision(concepts, terms.Decision{Text: "employ", Locale: "en", Status: model.TermForbidden, Replacement: "use", TextScope: profile.ScopeProse})
		require.Len(t, concepts, 2)
		scopes := map[string]string{}
		for _, r := range terms.SourceWordRules(concepts, "en") {
			scopes[r.Term] = r.Scope
		}
		assert.Equal(t, map[string]string{"utilize": "", "employ": profile.ScopeProse}, scopes)
	})

	t.Run("a concept with only the replacement takes the marking", func(t *testing.T) {
		concepts := []terms.Concept{{ID: "use", Terms: []terms.Term{{Text: "use", Locale: "en", Status: model.TermPreferred}}}}
		concepts, target, _ := terms.UpsertDecision(concepts, leverage)
		require.Len(t, concepts, 1)
		assert.Equal(t, 0, target)
		assert.True(t, concepts[0].Advisory)
	})
}
