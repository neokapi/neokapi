package profile

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// Tone rules report until a person marks them failing; word rules fail until
// they are marked advisory. These tables pin both defaults at every producer.

func TestPatternFailing(t *testing.T) {
	tests := []struct {
		name string
		pat  Pattern
		want bool
	}{
		{name: "an unmarked pattern reports", pat: Pattern{Regex: `x`}, want: false},
		{name: "fails: true fails", pat: Pattern{Regex: `x`, Fails: true}, want: true},
		{name: "advisory: true reports", pat: Pattern{Regex: `x`, Advisory: true}, want: false},
		{name: "advisory wins over fails", pat: Pattern{Regex: `x`, Fails: true, Advisory: true}, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.pat.Failing())
		})
	}
}

// TestPatternOutcomeAtEveryProducer holds each producer of a pattern finding to
// Pattern.Failing: a prohibited match, a prohibited match over a rate ceiling,
// and an absent required pattern.
func TestPatternOutcomeAtEveryProducer(t *testing.T) {
	marks := []struct {
		name     string
		fails    bool
		advisory bool
		want     bool
	}{
		{name: "unmarked", want: false},
		{name: "fails", fails: true, want: true},
		{name: "advisory", advisory: true, want: false},
		{name: "both", fails: true, advisory: true, want: false},
	}
	for _, m := range marks {
		t.Run(m.name, func(t *testing.T) {
			prohibited := profileWithPatterns(Pattern{Regex: `\bgonna\b`, Fails: m.fails, Advisory: m.advisory})
			hits := MatchPatterns(prohibited, "it is gonna work")
			require.Len(t, hits, 1)
			assert.Equal(t, m.want, hits[0].Fails, "prohibited pattern")

			rated := &VoiceProfile{Style: StyleRules{ProhibitedPatterns: []Pattern{{
				Regex: `\x{2014}`, Fails: m.fails, Advisory: m.advisory,
				Rate: &PatternRate{Max: 1, Per: 1000},
			}}}}
			over := MatchPatterns(rated, strings.Repeat("word ", 100)+"a — b — c")
			require.Len(t, over, 2, "over the ceiling every match is a hit")
			for _, h := range over {
				assert.Equal(t, m.want, h.Fails, "rate ceiling")
			}

			required := profileWithRequired(Pattern{Regex: `©`, Fails: m.fails, Advisory: m.advisory})
			findings := DocumentFindings(required, "no notice here")
			require.Len(t, findings, 1)
			assert.Equal(t, m.want, findings[0].Fails, "required pattern")
		})
	}
}

// TestTermRuleOutcomeIsUnchanged: a word rule fails unless it is advisory, and
// a suggested rule never fails.
func TestTermRuleOutcomeIsUnchanged(t *testing.T) {
	tests := []struct {
		name      string
		rule      TermRule
		suggested bool
		want      bool
	}{
		{name: "an unmarked term rule fails", rule: TermRule{Term: "utilize", Replacement: "use"}, want: true},
		{name: "an advisory term rule reports", rule: TermRule{Term: "utilize", Replacement: "use", Advisory: true}, want: false},
		{name: "a suggested term rule reports", rule: TermRule{Term: "utilize", Replacement: "use"}, suggested: true, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hits := MatchTermRulesInText([]TermRuleSet{{Rules: []TermRule{tt.rule}, Kind: VocabForbidden, Suggested: tt.suggested}}, "we utilize it")
			require.Len(t, hits, 1)
			assert.Equal(t, tt.want, hits[0].Fails)
		})
	}
}

// TestCommentLimitsReportUnlessMarked: comment limits are style measures, and
// what they find fails only under `fails: true`.
func TestCommentLimitsReportUnlessMarked(t *testing.T) {
	assert.False(t, (&CommentRules{}).Limits().Fails)
	assert.True(t, (&CommentRules{Fails: true}).Limits().Fails)
}

// TestConstraintPatternFails: a constraint holds for every audience, so its
// pattern fails whatever the style patterns default to.
func TestConstraintPatternFails(t *testing.T) {
	p := &VoiceProfile{Constraints: []Constraint{{
		ID: "no-guarantee", Version: 1, Source: "legal", Statement: "No guarantees",
		Kind: ConstraintProhibitedPattern, Regex: `(?i)\bguaranteed\b`,
	}}}
	findings := Findings(p, "Results guaranteed.", nil)
	require.Len(t, findings, 1)
	assert.True(t, findings[0].Fails)
}

func TestPatternFailsKeyReadsFromYAML(t *testing.T) {
	var style StyleRules
	require.NoError(t, yaml.Unmarshal([]byte(`
prohibited_patterns:
  - regex: '\bgonna\b'
    fails: true
  - regex: '\bwanna\b'
`), &style))
	require.Len(t, style.ProhibitedPatterns, 2)
	assert.True(t, style.ProhibitedPatterns[0].Failing())
	assert.False(t, style.ProhibitedPatterns[1].Failing())
}

func TestGuideNamesEachPatternOutcome(t *testing.T) {
	guide := RenderVoiceGuide(&VoiceProfile{Name: "v", Style: StyleRules{
		ProhibitedPatterns: []Pattern{
			{Regex: `\bgonna\b`, Description: "Casual contraction", Fails: true},
			{Regex: `\bwanna\b`, Description: "Casual wish"},
		},
		RequiredPatterns: []Pattern{{Regex: `©`, Description: "A copyright notice", Advisory: true}},
	}})
	assert.Contains(t, guide, "Casual contraction (such as gonna); fails a check")
	assert.Contains(t, guide, "Casual wish (such as wanna); reports")
	assert.Contains(t, guide, "A copyright notice; reports")
}
