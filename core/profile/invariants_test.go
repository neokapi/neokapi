package profile

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The audience sample is the configuration that lost a safety constraint in the
// 2026-09-09 coverage lab: "A video appointment is risk-free." was a critical
// finding at the default point and no finding at all inside a child collection.
// The fix was to state the rule as a constraint rather than as a style pattern,
// because a channel replaces tone and style wholesale by design.
//
// This asserts the property against the shipped sample rather than a fixture,
// so an edit that moves the rule back into `style:` fails here.
func TestAudienceSampleConstraintSurvivesEveryChannel(t *testing.T) {
	path := filepath.Join("..", "..", "samples", "audience-context", ".kapi", "voice.yaml")
	f, err := os.Open(path)
	require.NoError(t, err, "the audience sample profile must exist")
	defer func() { _ = f.Close() }()

	p, err := LoadProfileYAML(f)
	require.NoError(t, err)
	require.NotEmpty(t, p.Channels, "the sample must declare audience channels")

	const offending = "A video appointment is risk-free."

	channels := []string{""}
	for name := range p.Channels {
		channels = append(channels, name)
	}

	for _, channel := range channels {
		name := channel
		if name == "" {
			name = "default"
		}
		t.Run(name, func(t *testing.T) {
			resolved := ResolveProfile(p, "", channel, "")
			require.NotNil(t, resolved)

			findings := Findings(resolved, offending, nil)
			require.NotEmpty(t, findings,
				"the risk-free rule must hold at every audience, not only the default point")

			var critical bool
			for _, f := range findings {
				if f.Severity == SeverityCritical {
					critical = true
				}
			}
			assert.True(t, critical, "the rule is critical wherever it applies")
		})
	}
}

// A rule stated under `style:` is presentation and a channel may replace it.
// That separation is the reason constraints exist, so it is worth pinning: if
// this ever starts surviving, the two mechanisms have collapsed into one.
func TestStylePatternsRemainReplaceableByAChannel(t *testing.T) {
	p := &VoiceProfile{
		Name:  "presentation",
		Style: StyleRules{ProhibitedPatterns: []Pattern{{Regex: "legacy", Severity: "critical"}}},
		Channels: map[string]ChannelOverride{
			"child": {Style: &StyleRules{SentenceLength: "short"}},
		},
	}

	base := ResolveProfile(p, "", "", "")
	assert.NotEmpty(t, Findings(base, "legacy", nil),
		"a style pattern applies at the default point")

	child := ResolveProfile(p, "", "child", "")
	assert.Empty(t, Findings(child, "legacy", nil),
		"a channel supplying its own style replaces the base style, by design")
}

func TestValidateWarnsWhenAnOverrideDropsABasePattern(t *testing.T) {
	p := &VoiceProfile{
		Name:  "harbor",
		Style: StyleRules{ProhibitedPatterns: []Pattern{{Regex: `(?i)risk-free`, Severity: "critical"}}},
		Channels: map[string]ChannelOverride{
			"child": {Style: &StyleRules{SentenceLength: "short"}},
		},
	}

	probs := ValidateProfile(p)
	var found *ProfileProblem
	for i := range probs {
		if probs[i].Field == "channels.child.style" {
			found = &probs[i]
		}
	}
	require.NotNil(t, found, "an override that drops a base pattern must be reported")
	assert.True(t, found.Warning, "replacement is legitimate, so this warns rather than fails")
	assert.Contains(t, found.Message, `(?i)risk-free`)
	assert.Contains(t, found.Message, "constraints:", "the message must name where the rule belongs")
}

func TestValidateStaysQuietWhenTheRuleIsAlsoAConstraint(t *testing.T) {
	p := &VoiceProfile{
		Name:  "harbor",
		Style: StyleRules{ProhibitedPatterns: []Pattern{{Regex: `(?i)risk-free`, Severity: "critical"}}},
		Constraints: []Constraint{{
			ID: "assurance", Version: 1, Source: "service-facts.md",
			Statement: "Do not promise risk-free appointments.",
			Kind:      ConstraintProhibitedPattern, Regex: `(?i)risk-free`,
		}},
		Channels: map[string]ChannelOverride{
			"child": {Style: &StyleRules{SentenceLength: "short"}},
		},
	}

	for _, pr := range ValidateProfile(p) {
		assert.NotEqual(t, "channels.child.style", pr.Field,
			"an author who stated the rule as a constraint has already done the right thing")
	}
}

func TestValidateIsQuietWithNoOverrides(t *testing.T) {
	p := &VoiceProfile{
		Name:  "harbor",
		Style: StyleRules{ProhibitedPatterns: []Pattern{{Regex: `(?i)risk-free`}}},
	}
	for _, pr := range ValidateProfile(p) {
		assert.NotContains(t, pr.Field, ".style", "nothing to warn about without an override")
	}
}
