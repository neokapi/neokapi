package profile

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// BlockRuleCount counts the rules Findings applies to one block, so a caller
// can tell a block held to no rule from a block that passed its rules.
func TestBlockRuleCountIsTheRulesFindingsAppliesToABlock(t *testing.T) {
	assert.Zero(t, BlockRuleCount(nil))
	assert.Zero(t, BlockRuleCount(&VoiceProfile{Name: "empty"}))

	required := &VoiceProfile{Name: "doc", Style: StyleRules{RequiredPatterns: []Pattern{{Regex: `©`}}}}
	assert.Zero(t, BlockRuleCount(required), "a required pattern applies to a document, not a block")

	forbidden := &VoiceProfile{Name: "f", Vocabulary: VocabularyRules{ForbiddenTerms: []TermRule{{Term: "cheap"}}}}
	competitor := &VoiceProfile{Name: "c", Vocabulary: VocabularyRules{CompetitorTerms: []TermRule{{Term: "Acme"}}}}
	prohibited := &VoiceProfile{Name: "p", Style: StyleRules{ProhibitedPatterns: []Pattern{{Regex: `!!`}}}}
	for _, c := range []struct {
		profile *VoiceProfile
		text    string
	}{
		{forbidden, "a cheap offer"},
		{competitor, "better than Acme"},
		{prohibited, "Save now!!"},
	} {
		assert.Equal(t, 1, BlockRuleCount(c.profile), c.profile.Name)
		assert.NotEmpty(t, Findings(c.profile, c.text, nil),
			"%s: a counted rule is one Findings applies to a block", c.profile.Name)
	}
}
