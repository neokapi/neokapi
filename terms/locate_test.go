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
