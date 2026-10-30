package profile

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/model"
)

// codeSpanRuns is "Run `leverage --now`, then leverage it." as the markdown
// reader holds it: the code span is an inline-code pair around its text, so
// the text of the runs carries no backticks.
func codeSpanRuns() []model.Run {
	return []model.Run{
		model.TextR("Run "),
		model.PcOpenR(model.PcOpenRun{ID: "1", Type: model.TypeInlineCode, Data: "`"}),
		model.TextR("leverage --now"),
		model.PcCloseR(model.PcCloseRun{ID: "1", Type: model.TypeInlineCode, Data: "`"}),
		model.TextR(", then leverage it."),
	}
}

// A rule scoped to prose stays out of a code span the runs declare, and a rule
// with no scope matches inside it on purpose.
func TestMatchTermRulesInRuns_ProseScopeReadsCodeFromTheRuns(t *testing.T) {
	runs := codeSpanRuns()
	text := model.RunsText(runs)
	require.NotContains(t, text, "`")
	last := strings.LastIndex(text, "leverage")

	prose := []TermRuleSet{{Rules: []TermRule{{Term: "leverage", Replacement: "rely on", Scope: ScopeProse}}}}
	hits := MatchTermRulesInRuns(prose, text, runs)
	require.Len(t, hits, 1, "only the use in prose")
	assert.Equal(t, last, hits[0].Start)

	code := []TermRuleSet{{Rules: []TermRule{{Term: "leverage", Scope: ScopeCode}}}}
	hits = MatchTermRulesInRuns(code, text, runs)
	require.Len(t, hits, 1, "only the use in code")
	assert.Equal(t, strings.Index(text, "leverage"), hits[0].Start)

	unscoped := []TermRuleSet{{Rules: []TermRule{{Term: "leverage", Replacement: "rely on"}}}}
	assert.Len(t, MatchTermRulesInRuns(unscoped, text, runs), 2, "a rule with no scope matches inside code")

	// Without the runs the text alone says nothing about code, so a prose
	// rule reads all of it as prose.
	assert.Len(t, MatchTermRulesInText(prose, text), 2)
	// The same text written with backticks says it.
	assert.Len(t, MatchTermRulesInText(prose, "Run `leverage --now`, then leverage it."), 1)
}

func TestScopedText(t *testing.T) {
	runs := codeSpanRuns()
	text := model.RunsText(runs)
	s := NewScopedText(text, runs)
	inCode, inProse := strings.Index(text, "leverage"), strings.LastIndex(text, "leverage")
	assert.True(t, s.Applies("", inCode))
	assert.False(t, s.Applies(ScopeProse, inCode))
	assert.True(t, s.Applies(ScopeProse, inProse))
	assert.True(t, s.Applies(ScopeCode, inCode))
	assert.False(t, s.Applies(ScopeCode, inProse))
}

// A URL is an address: no word rule fires inside it, scoped or not, and the
// prose beside it is still read.
func TestMatchTermRules_URLsAreNotHits(t *testing.T) {
	sets := []TermRuleSet{{Rules: []TermRule{{Term: "utilize", Replacement: "use"}}}}
	text := "See https://example.com/utilize/setup, then utilize it."
	hits := MatchTermRulesInText(sets, text)
	require.Len(t, hits, 1)
	assert.Equal(t, strings.LastIndex(text, "utilize"), hits[0].Start)
}

// A prohibited pattern does not fire on the words of a URL, and one that
// matches from where the URL starts is about the address and still does.
func TestMatchPatterns_URLs(t *testing.T) {
	p := &VoiceProfile{Style: StyleRules{ProhibitedPatterns: []Pattern{
		{Regex: `(?i)\butilize\b`, Description: "utilize"},
		{Regex: `http://`, Description: "an address that is not secure"},
	}}}
	text := "Read http://example.com/utilize first."
	hits := MatchPatterns(p, text)
	require.Len(t, hits, 1)
	assert.Equal(t, "http://", text[hits[0].Start:hits[0].End])

	runs := []model.Run{model.TextR("Utilize https://example.com/utilize today.")}
	hits = MatchPatternsInRuns(p, runs)
	require.Len(t, hits, 1)
	assert.Equal(t, 0, hits[0].Start)
}
