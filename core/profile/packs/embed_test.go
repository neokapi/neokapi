package packs

import (
	"regexp"
	"testing"

	"github.com/neokapi/neokapi/core/profile"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestList(t *testing.T) {
	names, err := List()
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(names), 5)
	assert.Contains(t, names, "professional-b2b")
	assert.Contains(t, names, "friendly-dtc")
	assert.Contains(t, names, "technical-docs")
	assert.Contains(t, names, "marketing-blog")
	assert.Contains(t, names, "customer-support")
}

func TestLoad(t *testing.T) {
	p, err := Load("professional-b2b")
	require.NoError(t, err)
	assert.Equal(t, "Professional B2B", p.Name)
	assert.NotEmpty(t, p.Tone.Personality)
	assert.Equal(t, "formal", p.Tone.Formality)
	assert.NotEmpty(t, p.Examples)
	assert.NotEmpty(t, p.Vocabulary.ForbiddenTerms)
}

func TestLoadAll(t *testing.T) {
	profiles, err := LoadAll()
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(profiles), 5)
	for _, p := range profiles {
		assert.NotEmpty(t, p.Name)
		assert.NotEmpty(t, p.Tone.Formality)
		assert.NotEmpty(t, p.Examples, "pack %q should have examples", p.Name)
	}
}

func TestLoadInvalid(t *testing.T) {
	_, err := Load("nonexistent")
	require.Error(t, err)
}

// TestProhibitedPatternsAreEnforced: every pack's prohibited patterns are matched
// against content, not merely rendered into the LLM prompt. A pack that declares
// a pattern major while its minor vocabulary rules decide the score has a
// severity ladder that reads backwards, and the offline check its scaffold
// advertises as deterministic is not a check at all.
func TestProhibitedPatternsAreEnforced(t *testing.T) {
	p, err := Load("professional-b2b")
	require.NoError(t, err)
	require.NotEmpty(t, p.Style.ProhibitedPatterns, "the pack declares patterns")

	hits := profile.MatchPatterns(p, "This is gonna work")
	require.NotEmpty(t, hits, "the pack's `gonna` rule must fire against content")
	assert.Equal(t, profile.SeverityMajor, hits[0].Severity,
		"the pack declares the rule major, and the matcher must honour it")

	// The score moves, so the rule decides something.
	findings := profile.Findings(p, "This is gonna work", nil)
	assert.Less(t, profile.CalculateScore(findings).Overall, 100)
}

// TestEveryPackPatternCompiles: an uncompilable regex matches nothing, silently.
func TestEveryPackPatternCompiles(t *testing.T) {
	profiles, err := LoadAll()
	require.NoError(t, err)
	for _, p := range profiles {
		for _, pat := range p.Style.ProhibitedPatterns {
			_, cerr := regexp.Compile(pat.Regex)
			assert.NoError(t, cerr, "pack %q pattern %q", p.Name, pat.Regex)
		}
	}
}
