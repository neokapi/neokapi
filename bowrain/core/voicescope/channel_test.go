package voicescope

import (
	"testing"

	"github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/core/profile"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// channelledStore holds one project profile whose two channels each set a
// different sentence length and whose two personas each set a different
// formality, and a constraint that applies only in Norwegian.
func channelledStore() *fakeVoiceStore {
	return &fakeVoiceStore{profiles: map[string]*profile.VoiceProfile{
		"project": {
			ID: "project", Name: "Project",
			Constraints: []profile.Constraint{{
				ID: "nb-only", Version: 1, Source: "facts.md",
				Statement: "Norwegian copy promises no guarantee.",
				Kind:      profile.ConstraintProhibitedPattern, Regex: `(?i)\bgaranti\b`,
				Scope: profile.ConstraintScope{Locale: "nb"},
			}},
			Channels: map[string]profile.ChannelOverride{
				"email": {Style: &profile.StyleRules{SentenceLength: "short"}},
				"docs":  {Style: &profile.StyleRules{SentenceLength: "varied"}},
			},
			Personas: map[string]profile.PersonaOverride{
				"sam": {Tone: &profile.ToneProfile{Formality: "casual"}},
				"kim": {Tone: &profile.ToneProfile{Formality: "formal"}},
			},
		},
	}}
}

// overrideMarks names the channel and persona a resolved profile carries: the
// channel's sentence length, then the persona's formality.
func overrideMarks(p *profile.VoiceProfile) []string {
	return []string{p.Style.SentenceLength, p.Tone.Formality}
}

func TestResolve_ExplicitChannelReplacesTheBoundChannel(t *testing.T) {
	cs := &fakeScopeStore{project: &store.Project{Properties: map[string]string{
		profile.PropertyProfileID: "project",
		profile.PropertyChannel:   "email",
		profile.PropertyPersona:   "sam",
	}}}

	bound, err := Resolve(t.Context(), cs, nil, channelledStore(), Scope{ProjectID: "p1"})
	require.NoError(t, err)
	require.NotNil(t, bound)
	assert.Equal(t, []string{"short", "casual"}, overrideMarks(bound))

	explicit, err := Resolve(t.Context(), cs, nil, channelledStore(), Scope{
		ProjectID: "p1", Channel: "docs", Persona: "kim",
	})
	require.NoError(t, err)
	require.NotNil(t, explicit)
	assert.Equal(t, []string{"varied", "formal"}, overrideMarks(explicit),
		"an explicit channel and persona take the bound ones' place rather than layering on them")
}

func TestResolve_ExplicitChannelKeepsTheLocale(t *testing.T) {
	cs := &fakeScopeStore{project: &store.Project{Properties: map[string]string{
		profile.PropertyProfileID: "project",
	}}}

	got, err := Resolve(t.Context(), cs, nil, channelledStore(), Scope{
		ProjectID: "p1", Locale: "nb", Channel: "docs",
	})
	require.NoError(t, err)
	require.NotNil(t, got)

	status := map[string]string{}
	for _, r := range profile.ConstraintResolutions(got) {
		status[r.Constraint.ID] = r.Status
	}
	assert.Equal(t, "applicable", status["nb-only"],
		"a per-call channel must not drop the locale the profile was resolved at")
}
