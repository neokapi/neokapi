package tools

import (
	"testing"

	"github.com/neokapi/neokapi/core/brand"
	"github.com/neokapi/neokapi/core/check"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/tool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func runBrandVocab(t *testing.T, p *brand.VoiceProfile, text string) []check.Finding {
	t.Helper()
	b := &model.Block{ID: "b", Translatable: true, Source: []model.Run{{Text: &model.TextRun{Text: text}}}}
	tl := NewBrandVocabCheckTool(p, nil)
	require.NoError(t, tl.Annotate(tool.NewBlockView(b)))
	if ann, ok := model.AnnoAs[*brand.BrandVoiceAnnotation](b, "brand-voice"); ok {
		return ann.Findings
	}
	return nil
}

// TestCorrectionBecomesEnforcedCheck is the closed loop end to end: a term that
// a profile does not yet forbid is not flagged; after a correction-derived rule
// is promoted into the profile, the very same text is flagged with the team's
// replacement — a correction made once, enforced forever.
func TestCorrectionBecomesEnforcedCheck(t *testing.T) {
	p := &brand.VoiceProfile{}
	const text = "Please utilize the new API."

	// Before promotion: nothing to flag.
	assert.Empty(t, runBrandVocab(t, p, text), "term is not forbidden yet")

	// A team corrected "utilize" → "use" repeatedly; that becomes a rule.
	brand.ApplySuggestedRule(p, brand.SuggestedRule{
		Term: "utilize", Replacement: "use", CorrectionCount: 3, Dimension: brand.DimensionVocabulary,
	})

	// After promotion: the same content now fails the check, with the fix.
	after := runBrandVocab(t, p, text)
	require.Len(t, after, 1)
	assert.Equal(t, check.SeverityMajor, after[0].Severity)
	assert.Equal(t, "utilize", after[0].OriginalText)
	assert.Contains(t, after[0].Suggestion, "use")
}
