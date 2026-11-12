package rulesview

import (
	"testing"

	"github.com/stretchr/testify/assert"

	coreprofile "github.com/neokapi/neokapi/core/profile"
)

func TestRuleTally(t *testing.T) {
	voice := &coreprofile.VoiceProfile{
		Name: "plain",
		Style: coreprofile.StyleRules{
			ProhibitedPatterns: []coreprofile.Pattern{{Regex: `\bvery\b`}, {Regex: " "}},
			RequiredPatterns:   []coreprofile.Pattern{{Regex: `support line`}},
		},
		Constraints: []coreprofile.Constraint{
			{ID: "no-guarantee", Version: 1, Kind: coreprofile.ConstraintProhibitedPattern, Regex: `guarantee`},
			{ID: "be-kind", Version: 1, Kind: coreprofile.ConstraintGuidance},
		},
	}

	var tally RuleTally
	assert.Zero(t, tally.Len())

	// One place: two word rules, a note, and the voice's three pattern rules
	// (two patterns with an expression and one prohibited-pattern constraint).
	tally.AddWordRules([]WordRule{
		{Say: "sign in", Not: []string{"Login", "log-in"}, Locale: "en"},
		{Not: []string{"cheap"}},
	})
	tally.AddNote("op-1")
	tally.AddVoice(voice)
	assert.Equal(t, 6, tally.Len())
	assert.Equal(t, 3, coreprofile.PatternRuleCount(voice)-1,
		"the tally counts the patterns PatternRuleCount counts, less the one with no expression")

	// A second place holding the same rules, spelled with other case and
	// order, adds nothing.
	tally.AddWordRule(" Sign in ", []string{"log-in", "login"}, "en")
	tally.AddNote("op-1")
	tally.AddVoice(voice)
	assert.Equal(t, 6, tally.Len())

	// The same wording in another language is another rule.
	tally.AddWordRule("sign in", []string{"login", "log-in"}, "fr")
	assert.Equal(t, 7, tally.Len())
}
