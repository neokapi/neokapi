package voicescope

import (
	"testing"

	"github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/core/profile"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// channelledStore holds one project profile whose two channels and two personas
// each forbid a different term, and a constraint that applies only in
// Norwegian.
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
				"email": {Vocabulary: &profile.VocabularyRules{ForbiddenTerms: []profile.TermRule{{Term: "blast"}}}},
				"docs":  {Vocabulary: &profile.VocabularyRules{ForbiddenTerms: []profile.TermRule{{Term: "simply"}}}},
			},
			Personas: map[string]profile.PersonaOverride{
				"sam": {Avoided: []profile.TermRule{{Term: "awesome"}}},
				"kim": {Avoided: []profile.TermRule{{Term: "synergy"}}},
			},
		},
	}}
}

func forbiddenTerms(p *profile.VoiceProfile) []string {
	out := []string{}
	for _, r := range p.Vocabulary.ForbiddenTerms {
		out = append(out, r.Term)
	}
	return out
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
	assert.Equal(t, []string{"blast", "awesome"}, forbiddenTerms(bound))

	explicit, err := Resolve(t.Context(), cs, nil, channelledStore(), Scope{
		ProjectID: "p1", Channel: "docs", Persona: "kim",
	})
	require.NoError(t, err)
	require.NotNil(t, explicit)
	assert.Equal(t, []string{"simply", "synergy"}, forbiddenTerms(explicit),
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
