package check

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSourceChecksNameTheRulesTheirCanariesExpect holds the rule list each
// checker publishes to the canaries it is probed with: a category a canary
// expects is one the checker reports, so the list a dossier is bound to is a
// list the checker is held to on every run.
func TestSourceChecksNameTheRulesTheirCanariesExpect(t *testing.T) {
	canaries := map[string][]Canary{
		ContentLintID:   HygieneCanaries(),
		SourceLengthID:  LengthCanaries(80, 12),
		CommentStyleID:  append(append(CommentSentenceCanaries(CommentLimits{SentenceWords: 20}), CommentLengthCanaries(CommentLimits{CommentWords: 40, DocWords: 60, PackageDocWords: 80})...), CommentDensityCanary(CommentLimits{DensityRatio: 1})),
		SourcePatternID: nil,
	}
	patternCanaries, _ := PatternCanaries([]PatternRule{
		{Name: "forbidden-1", Pattern: `TODO`, MustNotMatch: true},
		{Name: "required-1", Pattern: `\bsee\b`, MustMatch: true},
	})
	canaries[SourcePatternID] = patternCanaries

	seen := map[string]bool{}
	for _, c := range SourceChecks() {
		assert.False(t, seen[c.ID], "checker ids are unique: %s", c.ID)
		seen[c.ID] = true
		require.NotEmpty(t, c.Family, "%s names a family", c.ID)
		require.NotEmpty(t, c.Categories, "%s reports at least one rule", c.ID)
		for _, rule := range c.RuleIDs() {
			assert.Equal(t, RuleID(c.Family, rule[len(c.Family)+1:]), rule)
		}
		probes, ok := canaries[c.ID]
		require.True(t, ok, "%s has canaries this test knows", c.ID)
		require.NotEmpty(t, probes, "%s is probed", c.ID)
		for _, canary := range probes {
			if canary.Expect == "" {
				continue
			}
			assert.Contains(t, c.Categories, canary.Expect, "%s expects a category it publishes", c.ID)
		}
	}
	assert.Len(t, SourceCheckIDs(), len(SourceChecks()))
}
