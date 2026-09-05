package review

import (
	"testing"

	"github.com/neokapi/neokapi/core/profile"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLeadTermRulesKeepsWhatThePromptCarried(t *testing.T) {
	// More rules than the cap, with the one bearing on the text buried at the
	// end so a plain truncation would drop it.
	rules := make([]profile.TermRule, 0, TermRuleLimit+5)
	for i := range TermRuleLimit + 4 {
		rules = append(rules, profile.TermRule{Term: filler(i), Replacement: "x"})
	}
	rules = append(rules, profile.TermRule{Term: "utilize", Replacement: "use"})

	got := LeadTermRules(rules, "The platform utilize your data")
	require.Len(t, got, TermRuleLimit)
	assert.Equal(t, "utilize", got[0].Term, "the rule the prompt would have scoped to the text leads")

	t.Run("a list within the cap is left alone", func(t *testing.T) {
		short := rules[:3]
		assert.Equal(t, short, LeadTermRules(short, "anything"))
	})
}

// filler names a rule that bears on nothing the test text says.
func filler(i int) string {
	return "filler" + string(rune('a'+i%26)) + string(rune('a'+i/26))
}
