package tools

import (
	"testing"

	"github.com/neokapi/neokapi/core/check"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/core/tool"
	"github.com/neokapi/neokapi/terms"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func runVoiceVocab(t *testing.T, tl *VoiceVocabCheckTool, text string) []check.Finding {
	t.Helper()
	b := &model.Block{ID: "b", Translatable: true, Source: []model.Run{{Text: &model.TextRun{Text: text}}}}
	require.NoError(t, tl.Annotate(tool.NewBlockView(b)))
	if ann, ok := model.AnnoAs[*profile.VoiceAnnotation](b, "voice"); ok {
		return ann.Findings
	}
	return nil
}

// TestCorrectionBecomesEnforcedCheck is the closed loop end to end: a term the
// terms store does not yet forbid is not flagged; after a correction-derived
// rule is promoted into the store, the very same text is flagged with the
// team's replacement. A correction made once is enforced from then on.
func TestCorrectionBecomesEnforcedCheck(t *testing.T) {
	ctx := t.Context()
	store := terms.NewInMemoryStore(terms.WithMaxConcepts(0))
	tl := NewVoiceVocabCheckTool(nil, store).InSourceLocale("en")
	const text = "Please utilize the new API."

	// Before promotion: nothing to flag.
	assert.Empty(t, runVoiceVocab(t, tl, text), "term is not forbidden yet")

	// A team corrected "utilize" to "use" repeatedly; that becomes a rule.
	changed, err := terms.PromoteRule(ctx, store, "en", profile.SuggestedRule{
		Term: "utilize", Replacement: "use", CorrectionCount: 3, Dimension: profile.DimensionVocabulary,
	})
	require.NoError(t, err)
	require.True(t, changed)

	// After promotion: the same content now fails the check, with the fix.
	after := runVoiceVocab(t, tl, text)
	require.Len(t, after, 1)
	assert.True(t, after[0].Fails)
	assert.Equal(t, "utilize", after[0].OriginalText)
	assert.Contains(t, after[0].Suggestion, "use")

	// Demoting the rule takes the check away again.
	changed, err = terms.DemoteRule(ctx, store, "en", "utilize")
	require.NoError(t, err)
	require.True(t, changed)
	assert.Empty(t, runVoiceVocab(t, tl, text), "a demoted term is no longer flagged")
}

// TestPromotedRuleOnACarriedListIsEnforced: a rule promoted into a voice
// file's carried terms is enforced the same way.
func TestPromotedRuleOnACarriedListIsEnforced(t *testing.T) {
	rules, changed := profile.ApplySuggestedRule(nil, profile.SuggestedRule{Term: "utilize", Replacement: "use", CorrectionCount: 2})
	require.True(t, changed)
	p := (&profile.VoiceProfile{}).Carry("voice file", rules)

	after := runVoiceVocab(t, NewVoiceVocabCheckTool(p, nil), "Please utilize the new API.")
	require.Len(t, after, 1)
	assert.True(t, after[0].Fails)
	assert.Contains(t, after[0].Suggestion, "use")
}
