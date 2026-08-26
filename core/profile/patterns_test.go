package profile

import (
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func profileWithPatterns(pats ...Pattern) *VoiceProfile {
	return &VoiceProfile{Style: StyleRules{ProhibitedPatterns: pats}}
}

func TestMatchPatterns(t *testing.T) {
	tests := []struct {
		name      string
		profile   *VoiceProfile
		text      string
		wantMatch []string   // matched substrings, in order
		wantSev   []Severity // parallel to wantMatch
	}{
		{
			name:      "prohibited pattern defaults to major",
			profile:   profileWithPatterns(Pattern{Regex: `\bgonna\b`, Description: "Casual contraction"}),
			text:      "This is gonna work",
			wantMatch: []string{"gonna"},
			wantSev:   []Severity{SeverityMajor},
		},
		{
			name:      "rule severity overrides the default",
			profile:   profileWithPatterns(Pattern{Regex: `\d+\s+formats`, Severity: "critical"}),
			text:      "kapi reads 42 formats",
			wantMatch: []string{"42 formats"},
			wantSev:   []Severity{SeverityCritical},
		},
		{
			name:      "every occurrence is a separate hit",
			profile:   profileWithPatterns(Pattern{Regex: `\bhype\b`}),
			text:      "hype and more hype",
			wantMatch: []string{"hype", "hype"},
			wantSev:   []Severity{SeverityMajor, SeverityMajor},
		},
		{
			name: "rules are independent and may overlap",
			profile: profileWithPatterns(
				Pattern{Regex: `powerful engine`},
				Pattern{Regex: `\bengine\b`, Severity: "minor"},
			),
			text:      "a powerful engine",
			wantMatch: []string{"powerful engine", "engine"},
			wantSev:   []Severity{SeverityMajor, SeverityMinor},
		},
		{
			name:      "the regex is matched as authored — no implicit case folding",
			profile:   profileWithPatterns(Pattern{Regex: `\bpowerful\b`}),
			text:      "Powerful things happen",
			wantMatch: nil,
		},
		{
			name:      "an author opts into case folding with (?i)",
			profile:   profileWithPatterns(Pattern{Regex: `(?i)\bpowerful\b`}),
			text:      "Powerful things happen",
			wantMatch: []string{"Powerful"},
			wantSev:   []Severity{SeverityMajor},
		},
		{
			name:      "a regex that does not compile yields nothing",
			profile:   profileWithPatterns(Pattern{Regex: `(unclosed`}),
			text:      "(unclosed",
			wantMatch: nil,
		},
		{
			name:      "an empty regex yields nothing",
			profile:   profileWithPatterns(Pattern{Regex: "  "}),
			text:      "anything at all",
			wantMatch: nil,
		},
		{
			name:      "a zero-width match marks no text and is dropped",
			profile:   profileWithPatterns(Pattern{Regex: `x*`}),
			text:      "nothing to see",
			wantMatch: nil,
		},
		{
			name:      "unicode class matches an emoji",
			profile:   profileWithPatterns(Pattern{Regex: `[\x{1F300}-\x{1FAFF}]`, Severity: "major"}),
			text:      "ship it 🚀 now",
			wantMatch: []string{"🚀"},
			wantSev:   []Severity{SeverityMajor},
		},
		{
			name:      "a nil profile yields nothing",
			profile:   nil,
			text:      "gonna",
			wantMatch: nil,
		},
		{
			name:      "empty text yields nothing",
			profile:   profileWithPatterns(Pattern{Regex: `\bgonna\b`}),
			text:      "",
			wantMatch: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hits := MatchPatterns(tt.profile, tt.text)
			require.Len(t, hits, len(tt.wantMatch))
			for i, want := range tt.wantMatch {
				assert.Equal(t, want, tt.text[hits[i].Start:hits[i].End], "hit %d text", i)
				assert.Equal(t, tt.wantSev[i], hits[i].Severity, "hit %d severity", i)
				assert.Equal(t, DimensionStyle, hits[i].Category, "hit %d category", i)
			}
		})
	}
}

func TestPatternHitsToFindings(t *testing.T) {
	p := profileWithPatterns(
		Pattern{Regex: `\bgonna\b`, Description: "Casual contraction", Severity: "major"},
		Pattern{Regex: `!!`},
	)
	text := "This is gonna be great!!"
	findings := PatternHitsToFindings(MatchPatterns(p, text), text, nil)
	require.Len(t, findings, 2)

	assert.Equal(t, "Prohibited pattern: Casual contraction", findings[0].Message)
	assert.Equal(t, "gonna", findings[0].OriginalText)
	assert.Equal(t, SeverityMajor, findings[0].Severity)
	assert.Equal(t, string(DimensionStyle), findings[0].Category)
	assert.Equal(t, `\bgonna\b`, findings[0].Metadata["pattern"])

	// A rule with no description falls back to naming the regex, so a finding is
	// never anonymous.
	assert.Equal(t, `Prohibited pattern "!!" matched`, findings[1].Message)
	assert.Equal(t, "!!", findings[1].OriginalText)

	assert.Empty(t, PatternHitsToFindings(nil, text, nil))
}

func TestPatternHitsToFindings_AnchorsToRuns(t *testing.T) {
	runs := []model.Run{
		{Text: &model.TextRun{Text: "This is "}},
		{Text: &model.TextRun{Text: "gonna work"}},
	}
	text := "This is gonna work"
	p := profileWithPatterns(Pattern{Regex: `\bgonna\b`})

	findings := PatternHitsToFindings(MatchPatterns(p, text), text, runs)
	require.Len(t, findings, 1)
	assert.Equal(t, 1, findings[0].Position.Start.Run, "the match starts in the second run")
}

// The severity ladder must read forwards: a pattern the pack marks critical
// outweighs a minor vocabulary rule. Before the pattern matcher existed only the
// vocabulary rules scored, so a profile's critical rule decided nothing.
func TestFindings_PatternsAndVocabularyScoreTogether(t *testing.T) {
	p := &VoiceProfile{
		Style: StyleRules{ProhibitedPatterns: []Pattern{
			{Regex: `\bgonna\b`, Description: "Casual contraction", Severity: "critical"},
		}},
		Vocabulary: VocabularyRules{ForbiddenTerms: []TermRule{
			{Term: "leverage", Replacement: "use", Severity: "minor"},
		}},
	}
	text := "We leverage this and it is gonna work"

	findings := Findings(p, text, nil)
	require.Len(t, findings, 2)
	// Vocabulary findings come first, then patterns.
	assert.Equal(t, string(DimensionVocabulary), findings[0].Category)
	assert.Equal(t, string(DimensionStyle), findings[1].Category)
	assert.Equal(t, SeverityCritical, findings[1].Severity)

	withPattern := CalculateScore(findings).Overall
	vocabOnly := CalculateScore(HitsToFindings(MatchVocabulary(p, text), text, nil)).Overall
	assert.Less(t, withPattern, vocabOnly, "the critical pattern must move the score")
}

func TestFindings_NilProfile(t *testing.T) {
	assert.Empty(t, Findings(nil, "anything", nil))
}

// A profile is matched once per block, so the compiled form is cached by regex
// source; a source that does not compile caches its failure rather than being
// retried per block.
func TestCompilePattern_CachesBothOutcomes(t *testing.T) {
	good := `\bcache-probe-ok\b`
	bad := `(cache-probe-bad`

	first := compilePattern(good)
	require.NotNil(t, first)
	assert.Same(t, first, compilePattern(good), "the second call reuses the compiled form")

	require.Nil(t, compilePattern(bad))
	cached, ok := patternCache.Load(bad)
	require.True(t, ok, "an uncompilable source is cached")
	assert.Nil(t, cached)
}
