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
	carried := p.CarriedTerms()
	assert.Equal(t, "pack professional-b2b", carried.From, "a pack's terms name the pack")
	assert.Equal(t, From("professional-b2b"), carried.From)
	assert.True(t, profile.HasWordRules(p), "the pack carries terms that reject a word")
}

// TestEveryPackCarriesItsTerms: each pack lists its word rules under `terms:`,
// and loading it carries them named as coming from that pack.
func TestEveryPackCarriesItsTerms(t *testing.T) {
	names, err := List()
	require.NoError(t, err)
	for _, name := range names {
		p, err := Load(name)
		require.NoError(t, err)
		carried := p.CarriedTerms()
		assert.Equal(t, From(name), carried.From, "pack %q", name)
		assert.NotEmpty(t, carried.Rules, "pack %q carries terms", name)
		for _, r := range carried.Rules {
			assert.True(t, r.Term != "" || r.Replacement != "", "pack %q has an empty rule", name)
		}
		assert.Empty(t, p.VoiceOnly().CarriedTerms().Rules, "the voice alone carries no terms")
	}
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
// a pattern that decides nothing leaves the offline check its scaffold
// advertises as deterministic without a rule to hold.
func TestProhibitedPatternsAreEnforced(t *testing.T) {
	p, err := Load("professional-b2b")
	require.NoError(t, err)
	require.NotEmpty(t, p.Style.ProhibitedPatterns, "the pack declares patterns")

	hits := profile.MatchPatterns(p, "This is gonna work")
	require.NotEmpty(t, hits, "the pack's `gonna` rule must fire against content")
	assert.True(t, hits[0].Fails,
		"the pack's rule is not advisory, so a match fails")

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
