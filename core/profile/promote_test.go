package profile

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestApplySuggestedRule_AddsRule(t *testing.T) {
	rules, changed := ApplySuggestedRule(nil, SuggestedRule{
		Term: "utilize", Replacement: "use", CorrectionCount: 3, Dimension: DimensionVocabulary,
	})
	assert.True(t, changed)
	require.Len(t, rules, 1)
	assert.Equal(t, "utilize", rules[0].Term)
	assert.Equal(t, "use", rules[0].Replacement)
	assert.Contains(t, rules[0].Note, "3 corrections")
}

func TestApplySuggestedRule_Idempotent(t *testing.T) {
	rules, _ := ApplySuggestedRule(nil, SuggestedRule{Term: "utilize", Replacement: "use", CorrectionCount: 3})

	// Same rule again: no change, no duplicate.
	again, changed := ApplySuggestedRule(rules, SuggestedRule{Term: "utilize", Replacement: "use", CorrectionCount: 3})
	assert.False(t, changed)
	require.Len(t, again, 1)

	// A newer correction with a different replacement updates in place.
	updated, changed := ApplySuggestedRule(rules, SuggestedRule{Term: "Utilize", Replacement: "employ", CorrectionCount: 5})
	assert.True(t, changed)
	require.Len(t, updated, 1, "case-insensitive: still one rule")
	assert.Equal(t, "employ", updated[0].Replacement)
	assert.Equal(t, "use", rules[0].Replacement, "the list passed in is left as it was")
}

func TestApplySuggestedRule_CarriesConceptID(t *testing.T) {
	rules, changed := ApplySuggestedRule(nil, SuggestedRule{
		Term: "utilize", Replacement: "use", CorrectionCount: 3, ConceptID: "concept-use",
	})
	assert.True(t, changed)
	require.Len(t, rules, 1)
	assert.Equal(t, "concept-use", rules[0].ConceptID)

	// A later concept-backed re-promotion attaches the concept to a standalone rule.
	standalone, _ := ApplySuggestedRule(nil, SuggestedRule{Term: "leverage", Replacement: "use", CorrectionCount: 2})
	require.Empty(t, standalone[0].ConceptID)
	attached, changed := ApplySuggestedRule(standalone, SuggestedRule{Term: "Leverage", Replacement: "use", CorrectionCount: 2, ConceptID: "c"})
	assert.True(t, changed)
	assert.Equal(t, "c", attached[0].ConceptID)
}

func TestApplySuggestedRule_EmptyTermNoop(t *testing.T) {
	rules, changed := ApplySuggestedRule(nil, SuggestedRule{})
	assert.False(t, changed)
	assert.Empty(t, rules)
}

func TestRemoveRule(t *testing.T) {
	rules := []TermRule{{Term: "utilize", Replacement: "use"}, {Term: "leverage", Replacement: "use"}}
	kept, changed := RemoveRule(rules, "Utilize")
	assert.True(t, changed)
	require.Len(t, kept, 1)
	assert.Equal(t, "leverage", kept[0].Term)

	same, changed := RemoveRule(kept, "synergy")
	assert.False(t, changed)
	assert.Len(t, same, 1)
}
