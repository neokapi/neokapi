package brand

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPromoteAndSave_VersionsAndEnforces(t *testing.T) {
	p := &VoiceProfile{ID: "p1", Version: 1}
	store := &mockBrandStore{profiles: map[string]*VoiceProfile{"p1": p}}
	ctx := context.Background()
	rule := SuggestedRule{Term: "utilize", Replacement: "use", CorrectionCount: 3}

	got, changed, err := PromoteAndSave(ctx, store, "p1", rule)
	require.NoError(t, err)
	assert.True(t, changed)
	assert.Equal(t, 2, got.Version, "version bumped")
	require.Len(t, got.Vocabulary.ForbiddenTerms, 1)
	assert.Contains(t, got.VersionNote, "utilize")

	// Idempotent: promoting the same rule again neither changes nor re-versions.
	_, changed2, err := PromoteAndSave(ctx, store, "p1", rule)
	require.NoError(t, err)
	assert.False(t, changed2)
	assert.Equal(t, 2, got.Version)

	// Unknown profile errors.
	_, _, err = PromoteAndSave(ctx, store, "missing", rule)
	assert.Error(t, err)
}

func TestApplySuggestedRule_AddsForbiddenTerm(t *testing.T) {
	p := &VoiceProfile{}
	changed := ApplySuggestedRule(p, SuggestedRule{
		Term: "utilize", Replacement: "use", CorrectionCount: 3, Dimension: DimensionVocabulary,
	})
	assert.True(t, changed)
	require.Len(t, p.Vocabulary.ForbiddenTerms, 1)
	assert.Equal(t, "utilize", p.Vocabulary.ForbiddenTerms[0].Term)
	assert.Equal(t, "use", p.Vocabulary.ForbiddenTerms[0].Replacement)
	assert.Contains(t, p.Vocabulary.ForbiddenTerms[0].Note, "3 corrections")
}

func TestApplySuggestedRule_Idempotent(t *testing.T) {
	p := &VoiceProfile{}
	ApplySuggestedRule(p, SuggestedRule{Term: "utilize", Replacement: "use", CorrectionCount: 3})

	// Same rule again → no change, no duplicate.
	again := ApplySuggestedRule(p, SuggestedRule{Term: "utilize", Replacement: "use", CorrectionCount: 3})
	assert.False(t, again)
	require.Len(t, p.Vocabulary.ForbiddenTerms, 1)

	// A newer correction with a different replacement updates in place.
	updated := ApplySuggestedRule(p, SuggestedRule{Term: "Utilize", Replacement: "employ", CorrectionCount: 5})
	assert.True(t, updated)
	require.Len(t, p.Vocabulary.ForbiddenTerms, 1, "case-insensitive: still one rule")
	assert.Equal(t, "employ", p.Vocabulary.ForbiddenTerms[0].Replacement)
}

func TestApplySuggestedRule_EmptyTermNoop(t *testing.T) {
	p := &VoiceProfile{}
	assert.False(t, ApplySuggestedRule(p, SuggestedRule{}))
	assert.Empty(t, p.Vocabulary.ForbiddenTerms)
}

func TestPromoteRules_CountsChanges(t *testing.T) {
	p := &VoiceProfile{}
	n := PromoteRules(p, []SuggestedRule{
		{Term: "utilize", Replacement: "use", CorrectionCount: 3},
		{Term: "leverage", Replacement: "use", CorrectionCount: 4},
		{Term: "utilize", Replacement: "use", CorrectionCount: 3}, // dup → no change
	})
	assert.Equal(t, 2, n)
	assert.Len(t, p.Vocabulary.ForbiddenTerms, 2)
}
