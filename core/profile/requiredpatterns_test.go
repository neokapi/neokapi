package profile

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A required pattern was validated, merged, copied and counted as a rule, and
// matched against content nowhere — so a profile card reported a rule total that
// included a list no gate read. These tests hold the two halves together: what
// the count says, and what the gates apply.
//
// The scope is the document, not the block. "This text must contain X" is a
// claim about the page — the call to action, the trademark line, the safety
// notice — and matching it per block would flag every paragraph of every file.

func profileWithRequired(pats ...Pattern) *VoiceProfile {
	return &VoiceProfile{Name: "t", Style: StyleRules{RequiredPatterns: pats}}
}

func TestUnmetRequiredPatterns(t *testing.T) {
	tests := []struct {
		name    string
		profile *VoiceProfile
		text    string
		want    []string // the regex of each rule expected to be reported unmet
	}{
		{
			name:    "nil profile declares nothing",
			profile: nil,
			text:    "anything",
		},
		{
			name:    "a satisfied rule is not reported",
			profile: profileWithRequired(Pattern{Regex: `\bkapi\b`}),
			text:    "Run kapi up to converge.",
		},
		{
			name:    "an absent rule is reported",
			profile: profileWithRequired(Pattern{Regex: `\bkapi\b`}),
			text:    "Run the loop to converge.",
			want:    []string{`\bkapi\b`},
		},
		{
			name: "each rule is judged on its own",
			profile: profileWithRequired(
				Pattern{Regex: `\bkapi\b`},
				Pattern{Regex: `(?i)\bregistered trademark\b`},
			),
			text: "Run kapi up to converge.",
			want: []string{`(?i)\bregistered trademark\b`},
		},
		{
			name:    "the regex is matched as authored: no implicit case folding",
			profile: profileWithRequired(Pattern{Regex: `\bKapi\b`}),
			text:    "run kapi up",
			want:    []string{`\bKapi\b`},
		},
		{
			name:    "an empty regex declares nothing",
			profile: profileWithRequired(Pattern{Regex: "   "}),
			text:    "",
		},
		{
			name:    "a regex that does not compile cannot fail a document",
			profile: profileWithRequired(Pattern{Regex: `[unclosed`}),
			text:    "",
		},
		{
			name:    "an empty document carries nothing a rule asks for",
			profile: profileWithRequired(Pattern{Regex: `\bkapi\b`}),
			text:    "",
			want:    []string{`\bkapi\b`},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			unmet := UnmetRequiredPatterns(tt.profile, tt.text)
			got := make([]string, 0, len(unmet))
			for _, p := range unmet {
				got = append(got, p.Regex)
			}
			assert.Equal(t, tt.want, nonEmpty(got))
		})
	}
}

func nonEmpty(s []string) []string {
	if len(s) == 0 {
		return nil
	}
	return s
}

func TestRequiredPatternFindings(t *testing.T) {
	p := profileWithRequired(
		Pattern{Regex: `(?i)\bstart free trial\b`, Description: "Every landing page carries the call to action", Severity: "critical"},
		Pattern{Regex: `©`},
	)
	findings := DocumentFindings(p, "A page with neither.")
	require.Len(t, findings, 2)

	assert.Equal(t, "Required pattern absent: Every landing page carries the call to action", findings[0].Message)
	assert.Equal(t, SeverityCritical, findings[0].Severity)
	assert.Equal(t, string(DimensionStyle), findings[0].Category)
	assert.Equal(t, `(?i)\bstart free trial\b`, findings[0].Metadata["pattern"])

	// A rule with no description names its regex, so a finding is never anonymous.
	assert.Equal(t, `Required pattern "©" is absent`, findings[1].Message)
	assert.Equal(t, SeverityMajor, findings[1].Severity, "patterns default to major, as the prohibited half does")

	// An absence sits nowhere: there is no offending text and no range to anchor.
	for _, f := range findings {
		assert.Empty(t, f.OriginalText)
		assert.True(t, f.Position.IsZero())
	}

	assert.Empty(t, DocumentFindings(p, "Start free trial © 2026"))
}

// The block-scope gate must stay block-scope: a required pattern evaluated there
// would flag every paragraph of every document, which is why the two gates are
// separate functions rather than one.
func TestFindings_LeavesRequiredPatternsToTheDocumentGate(t *testing.T) {
	p := profileWithRequired(Pattern{Regex: `\bkapi\b`})
	assert.Empty(t, Findings(p, "A paragraph that does not name the tool.", nil))
	assert.Len(t, DocumentFindings(p, "A paragraph that does not name the tool."), 1)
}

// TestPatternRuleCountIsTheRulesApplied is the agreement #1971 asks for: the
// number a profile card shows and the number of rules the gates can raise are
// the same number, because they come from the same place. A document that
// violates every declared pattern raises exactly one finding per counted rule.
func TestPatternRuleCountIsTheRulesApplied(t *testing.T) {
	p := &VoiceProfile{
		Name: "t",
		Style: StyleRules{
			ProhibitedPatterns: []Pattern{
				{Regex: `\bseamless\b`, Description: "Marketing superlative"},
				{Regex: `!!`},
			},
			RequiredPatterns: []Pattern{
				{Regex: `(?i)\bstart free trial\b`, Description: "Call to action"},
				{Regex: `©`},
			},
		},
	}
	require.Equal(t, 4, PatternRuleCount(p))

	doc := "Our seamless onboarding!!"
	var stylistic int
	for _, f := range append(Findings(p, doc, nil), DocumentFindings(p, doc)...) {
		if f.Category == string(DimensionStyle) {
			stylistic++
		}
	}
	assert.Equal(t, PatternRuleCount(p), stylistic,
		"every pattern rule the count includes is a rule some gate applies")

	assert.Zero(t, PatternRuleCount(nil))
}

// A required pattern reaches the model as well as the gate: a rule the profile
// declares but the prompt never mentions asks the model to guess.
func TestRenderVoiceGuideCarriesRequiredPatterns(t *testing.T) {
	p := profileWithRequired(Pattern{Regex: `(?i)\bstart free trial\b`, Description: "Every landing page carries the call to action"})
	guide := RenderVoiceGuide(p)
	assert.Contains(t, guide, "Every landing page carries the call to action")
}
