package profile

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Rates and scopes: the two things a pattern could not say, from issue #2242.
//
// Nearly every measured claim about writing is a density over a window, and a
// regex matches without counting. This repository's own CLAUDE.md states a rule
// in that shape — "one per 1,000 words is the ceiling" — that a profile could
// not express.

func ratePattern(regex string, maxN, per int) *VoiceProfile {
	return &VoiceProfile{Style: StyleRules{ProhibitedPatterns: []Pattern{{
		Regex: regex, Description: "em dashes", Severity: "minor",
		Rate: &PatternRate{Max: maxN, Per: per},
	}}}}
}

// TestRateAllowsWhatItPermits: under the ceiling nothing is reported, because
// the rule says so. A rate that still reported every match would be a
// prohibition with extra fields.
func TestRateAllowsWhatItPermits(t *testing.T) {
	p := ratePattern(`\x{2014}`, 1, 1000)
	text := strings.Repeat("word ", 1000) + "one — dash"

	assert.Empty(t, MatchPatterns(p, text), "one dash in a thousand words is what one-per-thousand means")
}

// TestRateReportsEveryMatchWhenExceeded.
//
// All of them rather than the "excess" ones: which occurrence is the excess is
// not a question the text can answer, and a writer fixing the document wants to
// see them all.
func TestRateReportsEveryMatchWhenExceeded(t *testing.T) {
	p := ratePattern(`\x{2014}`, 1, 1000)
	text := strings.Repeat("word ", 500) + "a — b — c — d"

	hits := MatchPatterns(p, text)
	assert.Len(t, hits, 3, "over the ceiling, every match is reported")
}

// TestShortTextGetsTheWholeAllowance.
//
// A ceiling that tightened as the text shortened would fail a paragraph for
// what it permits in a page: 1 per 1000 words must allow one dash in a
// 200-word note, not zero.
func TestShortTextGetsTheWholeAllowance(t *testing.T) {
	p := ratePattern(`\x{2014}`, 1, 1000)
	text := strings.Repeat("word ", 200) + "one — dash"

	assert.Empty(t, MatchPatterns(p, text))
	assert.Equal(t, 1, PatternRate{Max: 1, Per: 1000}.Allowance(200))
	assert.Equal(t, 1, PatternRate{Max: 1, Per: 1000}.Allowance(0))
}

// TestAllowanceScalesWithLength: 1 per 1000 allows 3 in 2500 words, because
// the window is a rate rather than a quota for the first thousand.
func TestAllowanceScalesWithLength(t *testing.T) {
	r := PatternRate{Max: 1, Per: 1000}
	assert.Equal(t, 1, r.Allowance(1000))
	assert.Equal(t, 2, r.Allowance(1001))
	assert.Equal(t, 3, r.Allowance(2500))
}

// TestNoRateMeansNever: every existing rule has no rate and must keep meaning
// what it meant.
func TestNoRateMeansNever(t *testing.T) {
	p := &VoiceProfile{Style: StyleRules{ProhibitedPatterns: []Pattern{{
		Regex: `\x{2014}`, Description: "em dashes",
	}}}}
	assert.Len(t, MatchPatterns(p, "a — b"), 1)
}

// TestScopeProseSkipsCode.
//
// The case that prompted this: a ban on implementation vocabulary fired inside
// a code sample that necessarily contains it. Vale skips code by default; here
// a rule asks, because changing what an existing rule matches is not a thing to
// do silently.
func TestScopeProseSkipsCode(t *testing.T) {
	p := &VoiceProfile{Style: StyleRules{ProhibitedPatterns: []Pattern{{
		Regex: `(?i)endpoint`, Description: "implementation vocabulary", Scope: ScopeProse,
	}}}}
	text := "Set the address you want.\n\n```\ncurl https://api/endpoint\n```\n\nThat is all."

	assert.Empty(t, MatchPatterns(p, text), "the only occurrence is inside a fence")

	text2 := "Set the endpoint you want.\n\n```\ncurl https://api/endpoint\n```"
	hits := MatchPatterns(p, text2)
	require.Len(t, hits, 1, "the one in prose still fires")
	assert.Less(t, hits[0].Start, strings.Index(text2, "```"))
}

// TestScopeProseSkipsInlineCode: a backtick span is code too, and a rule about
// wording should not fire on `--type-add` in running prose.
func TestScopeProseSkipsInlineCode(t *testing.T) {
	p := &VoiceProfile{Style: StyleRules{ProhibitedPatterns: []Pattern{{
		Regex: `(?i)utilize`, Description: "latinate", Scope: ScopeProse,
	}}}}
	assert.Empty(t, MatchPatterns(p, "Run `utilize --now` to start."))
	assert.Len(t, MatchPatterns(p, "You can utilize this."), 1)
}

// TestScopeHeadingOnlyMatchesHeadings: a house style for headings often differs
// from the body's, and a rule that cannot say so has to be two profiles.
func TestScopeHeadingOnlyMatchesHeadings(t *testing.T) {
	p := &VoiceProfile{Style: StyleRules{ProhibitedPatterns: []Pattern{{
		Regex: `(?i)guide`, Description: "no 'guide' in headings", Scope: ScopeHeading,
	}}}}
	text := "## The user guide\n\nThis guide explains the flags."

	hits := MatchPatterns(p, text)
	require.Len(t, hits, 1, "the heading fires and the sentence does not")
	assert.Less(t, hits[0].Start, strings.Index(text, "\n\n"))
}

// TestUnknownScopeStillMatches.
//
// A rule that quietly stops firing is worse than one that fires too widely:
// the first looks like a clean document. Validation reports the typo; matching
// keeps the rule alive.
func TestUnknownScopeStillMatches(t *testing.T) {
	p := &VoiceProfile{Style: StyleRules{ProhibitedPatterns: []Pattern{{
		Regex: `(?i)utilize`, Description: "latinate", Scope: "paragrpah",
	}}}}
	assert.Len(t, MatchPatterns(p, "You can utilize this."), 1)

	probs := ValidateProfile(&VoiceProfile{Name: "x", Style: p.Style})
	var named bool
	for _, pr := range probs {
		if strings.Contains(pr.Field, "scope") {
			named = true
		}
	}
	assert.True(t, named, "and validation says so")
}

// TestZeroRateIsRejected: it means the same as no rate, so writing it is a
// mistake rather than a very strict rule.
func TestZeroRateIsRejected(t *testing.T) {
	p := &VoiceProfile{Name: "x", Style: StyleRules{ProhibitedPatterns: []Pattern{{
		Regex: `x`, Rate: &PatternRate{Max: 0},
	}}}}
	probs := Blocking(ValidateProfile(p))
	var named bool
	for _, pr := range probs {
		if strings.Contains(pr.Field, "rate.max") {
			named = true
		}
	}
	assert.True(t, named)
}

// TestTheGuideCarriesTheRateAndTheScope.
//
// A rate the model never hears is a stricter rule in the prompt than the one
// the check enforces, and a scope it never hears is a wider one. Both are the
// #2240 defect a field along: the profile can say it and the model cannot hear
// it.
func TestTheGuideCarriesTheRateAndTheScope(t *testing.T) {
	p := &VoiceProfile{Style: StyleRules{ProhibitedPatterns: []Pattern{
		{
			Regex: `\x{2014}`, Description: "em dashes",
			Rate: &PatternRate{Max: 1, Per: 1000},
		},
		{
			Regex: `(?i)\b(?:endpoint|payload)\b`, Description: "implementation vocabulary",
			Scope: ScopeProse,
		},
	}}}

	for _, got := range []string{RenderVoiceGuide(p), RenderVoiceGuideCompact(p)} {
		assert.Contains(t, got, "at most 1 per 1000 words",
			"the model is told the ceiling, not just the ban")
		assert.Contains(t, got, "in prose, not in code")
		// And the words still arrive, from #2240.
		assert.Contains(t, got, "endpoint")
	}
}

// TestARateWithoutAWindowStatesTheDefault: "at most 1 per words" would be
// meaningless, so the rendered rule names the window it actually uses.
func TestARateWithoutAWindowStatesTheDefault(t *testing.T) {
	p := &VoiceProfile{Style: StyleRules{ProhibitedPatterns: []Pattern{{
		Regex: `\x{2014}`, Description: "em dashes", Rate: &PatternRate{Max: 2},
	}}}}
	assert.Contains(t, RenderVoiceGuideCompact(p), "at most 2 per 1000 words")
}

// TestTermRulesTakeAScopeToo.
//
// This gap existed because of a fix. #2240 pushed word lists out of
// prohibited_patterns and into forbidden_terms, since a term renders to the
// model as the word itself while a pattern rendered as its description. A "no
// implementation vocabulary" rule written the recommended way could then not
// say "in prose", and fired inside the code sample the document exists to
// explain — the exact case scopes were added for, reachable only through the
// mechanism we now recommend.
func TestTermRulesTakeAScopeToo(t *testing.T) {
	sets := []TermRuleSet{{
		Category: DimensionVocabulary,
		Rules: []TermRule{
			{Term: "endpoint", Scope: ScopeProse},
			{Term: "Ripgrep", CaseSensitive: true},
		},
	}}

	prose := MatchTermRules(sets, "Point it at the endpoint you chose.")
	assert.Len(t, prose, 1, "a scoped term still fires in prose")

	fenced := MatchTermRules(sets, "Run this:\n\n```\ncurl https://x/endpoint\n```\n")
	assert.Empty(t, fenced, "and not inside a fence")

	inline := MatchTermRules(sets, "The `endpoint` argument takes a URL.")
	assert.Empty(t, inline, "nor inside a backtick span")

	// An unscoped rule is unchanged: a name written wrongly in a code sample is
	// still written wrongly.
	code := MatchTermRules(sets, "```\n# Ripgrep is fast\n```")
	assert.Len(t, code, 1, "an unscoped rule still matches everywhere")
}
