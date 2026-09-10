package profile

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func constraintProfile() *VoiceProfile {
	return &VoiceProfile{
		Name:  "Harbor Help",
		Style: StyleRules{PersonPOV: "second", ProhibitedPatterns: []Pattern{{Regex: "legacy", Severity: "critical"}}},
		Constraints: []Constraint{{
			ID: "assurance", Version: 1, Source: "service-facts.md#assurances",
			Statement: "Do not promise risk-free appointments.", Kind: ConstraintProhibitedPattern, Regex: `risk-free`,
			Exceptions: []ConstraintException{{
				Scope: ConstraintScope{Channel: "policy-example"}, Reason: "Quoted policy example",
				ApprovedBy: "fixture-author", ApprovalRef: "fixture-review-1",
			}},
		}, {
			ID: "recording", Version: 1, Source: "service-facts.md#recording",
			Statement: "Appointments are not recorded.", Kind: ConstraintGuidance,
		}},
		Channels: map[string]ChannelOverride{
			"child":       {Style: &StyleRules{SentenceLength: "short"}},
			"teen":        {Style: &StyleRules{SentenceLength: "short"}},
			"adult":       {Style: &StyleRules{SentenceLength: "varied"}},
			"older-adult": {Style: &StyleRules{SentenceLength: "short"}},
		},
		Personas: map[string]PersonaOverride{"writer": {Style: &StyleRules{PersonPOV: "first_plural"}}},
	}
}

func TestConstraintsSurvivePresentationOverrides(t *testing.T) {
	p := constraintProfile()
	for _, channel := range []string{"", "child", "teen", "adult", "older-adult"} {
		t.Run(channel, func(t *testing.T) {
			resolved := ResolveProfile(p, "en", channel, "writer")
			assert.Equal(t, "first_plural", resolved.Style.PersonPOV)
			assert.Empty(t, Findings(resolved, "legacy", nil), "legacy style replacement stays intact")
			findings := Findings(resolved, "This is risk-free.", nil)
			require.Len(t, findings, 1)
			assert.Equal(t, SeverityCritical, findings[0].Severity)
			assert.Equal(t, "assurance", findings[0].Metadata["constraint_id"])
			assert.Equal(t, "1", findings[0].Metadata["constraint_version"])
			assert.Equal(t, "service-facts.md#assurances", findings[0].Metadata["constraint_source"])
			assert.Equal(t, "risk-free", findings[0].OriginalText)
			assert.Equal(t, 1, PatternRuleCount(resolved))
			assert.Empty(t, Findings(resolved, "Appointments are recorded.", nil))
			assert.Contains(t, RenderVoiceGuide(resolved), "semantic verification unsupported")
			assert.Contains(t, RenderVoiceGuideCompact(resolved), "Appointments are not recorded.")
		})
	}
	assert.Equal(t, "second", p.Style.PersonPOV)
	assert.Len(t, p.Style.ProhibitedPatterns, 1)
}

func TestConstraintScopeAndException(t *testing.T) {
	p := constraintProfile()
	excepted := ResolveProfile(p, "en", "policy-example", "")
	assert.Empty(t, Findings(excepted, "risk-free", nil))
	r := ConstraintResolutions(excepted)
	require.Len(t, r, 2)
	assert.Equal(t, "excepted", r[0].Status)
	assert.Contains(t, RenderVoiceGuide(excepted), "asserted approval: fixture-author")
	assert.Len(t, Findings(ResolveProfile(p, "en", "child", ""), "risk-free", nil), 1)
	p.Constraints[0].Scope = ConstraintScope{Locale: "en", Channel: "child", Persona: "writer"}
	for _, tc := range []struct {
		locale           model.LocaleID
		channel, persona string
		want             string
	}{
		{"EN", "child", "writer", "applicable"},
		{"en-US", "child", "writer", "out_of_scope"},
		{"en", "adult", "writer", "out_of_scope"},
		{"en", "child", "", "out_of_scope"},
	} {
		assert.Equal(t, tc.want, ConstraintResolutions(ResolveProfile(p, tc.locale, tc.channel, tc.persona))[0].Status)
	}
}

func TestConstraintsRejectInvalidConfiguration(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*VoiceProfile)
	}{
		{"regex", func(p *VoiceProfile) { p.Constraints[0].Regex = "[" }},
		{"empty-match", func(p *VoiceProfile) { p.Constraints[0].Regex = ".*" }},
		{"duplicate", func(p *VoiceProfile) { p.Constraints[1].ID = p.Constraints[0].ID }},
		{"kind", func(p *VoiceProfile) { p.Constraints[0].Kind = "magic" }},
		{"provenance", func(p *VoiceProfile) { p.Constraints[0].Source = "" }},
		{"version", func(p *VoiceProfile) { p.Constraints[0].Version = 0 }},
		{"exception-scope", func(p *VoiceProfile) { p.Constraints[0].Exceptions[0].Scope = ConstraintScope{} }},
		{"exception-approval", func(p *VoiceProfile) { p.Constraints[0].Exceptions[0].ApprovalRef = "" }},
		{"scope-locale", func(p *VoiceProfile) { p.Constraints[0].Scope.Locale = "not a locale" }},
		{"exception-locale", func(p *VoiceProfile) { p.Constraints[0].Exceptions[0].Scope.Locale = "en--US" }},
		{"exception-blank-scope", func(p *VoiceProfile) {
			p.Constraints[0].Exceptions[0].Scope = ConstraintScope{Channel: " "}
		}},
		{"guidance-regex", func(p *VoiceProfile) { p.Constraints[1].Regex = "recorded" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := constraintProfile()
			tc.mutate(p)
			assert.NotEmpty(t, Blocking(ValidateProfile(p)))
			_, err := ResolveProfileFromContext(t.Context(), ResolveContext{CollectionProfile: p}, nil)
			require.Error(t, err)
			findings := Findings(p, "clean", nil)
			require.Len(t, findings, 1)
			assert.Equal(t, "true", findings[0].Metadata["constraint_error"])
		})
	}
}

func TestConstraintDecodingAndClone(t *testing.T) {
	p := constraintProfile()
	data, err := json.Marshal(p)
	require.NoError(t, err)
	var decoded VoiceProfile
	require.NoError(t, json.Unmarshal(data, &decoded))
	assert.Equal(t, p.Constraints, decoded.Constraints)
	clone := p.Clone()
	clone.Constraints[0].Exceptions[0].Reason = "changed"
	assert.Equal(t, "Quoted policy example", p.Constraints[0].Exceptions[0].Reason)
	for _, doc := range []string{
		`{"constraints":[{"unknown":1}]}`,
		`{"constraints":[{"scope":{"chanel":"child"}}]}`,
		`{"constraints":[{"exceptions":[{"approved_by":"x","scope":{"chanel":"child"}}]}]}`,
	} {
		require.Error(t, json.Unmarshal([]byte(doc), &decoded))
	}
	for _, doc := range []string{
		"name: test\nconstraints:\n  - unknown: 1\n",
		"name: test\nconstraints:\n  - scope:\n      chanel: child\n",
		"name: test\nconstraints:\n  - id: missing-fields\n",
		"name: test\nconstraints:\n  - id: locale\n    version: 1\n    source: facts.md\n    statement: test\n    kind: guidance\n    scope:\n      locale: en--US\n",
	} {
		_, err := LoadProfileYAML(strings.NewReader(doc))
		require.Error(t, err)
	}
	_, err = LoadProfileYAML(strings.NewReader("name: test\nlegacy_unknown: ignored\n"))
	assert.NoError(t, err, "legacy profile loading remains lenient")
}

func TestConstraintEmptyListSerialization(t *testing.T) {
	for _, tc := range []struct {
		name    string
		data    string
		wantNil bool
	}{
		{name: "omitted", data: `{}`, wantNil: true},
		{name: "remove", data: `{"constraints":[]}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var p VoiceProfile
			require.NoError(t, json.Unmarshal([]byte(tc.data), &p))
			assert.Equal(t, tc.wantNil, p.Constraints == nil)
			assert.Equal(t, tc.wantNil, p.Clone().Constraints == nil)
			data, err := json.Marshal(p)
			require.NoError(t, err)
			var roundTrip VoiceProfile
			require.NoError(t, json.Unmarshal(data, &roundTrip))
			assert.Equal(t, tc.wantNil, roundTrip.Constraints == nil)
		})
	}
}

func TestConstraintResolutionsRetainIndependentRulesAndExceptions(t *testing.T) {
	p := constraintProfile()
	p.Constraints[0].Exceptions = append(p.Constraints[0].Exceptions, ConstraintException{
		Scope:  ConstraintScope{Locale: "en", Channel: "policy-example"},
		Reason: "English policy review", ApprovedBy: "reviewer", ApprovalRef: "review-2",
	})
	// An overlapping pattern retains its own identity and has no exception.
	p.Constraints = append(p.Constraints, Constraint{
		ID: "second-assurance", Version: 2, Source: "policy.md", Statement: "Do not promise risk-free service",
		Kind: ConstraintProhibitedPattern, Regex: "risk-free",
	})
	resolved := ResolveProfile(p, "EN", "policy-example", "")
	resolutions := ConstraintResolutions(resolved)
	require.Len(t, resolutions[0].Exceptions, 2)
	assert.Equal(t, "excepted", resolutions[0].Status)
	findings := Findings(resolved, "risk-free", nil)
	require.Len(t, findings, 1)
	assert.Equal(t, "second-assurance", findings[0].Metadata["constraint_id"])
	assert.Equal(t, 1, PatternRuleCount(resolved)-len(resolved.Style.ProhibitedPatterns))

	data, err := json.Marshal(resolutions)
	require.NoError(t, err)
	decoded := []ConstraintResolution{}
	require.NoError(t, json.Unmarshal(data, &decoded))
	assert.Equal(t, resolutions, decoded)
	resolutions[0].Constraint.Exceptions[0].Reason = "client edit"
	resolutions[0].Exceptions[1].ApprovalRef = "client edit"
	assert.Equal(t, "Quoted policy example", p.Constraints[0].Exceptions[0].Reason)
	assert.Equal(t, "review-2", ConstraintResolutions(resolved)[0].Exceptions[1].ApprovalRef)
	assert.Len(t, Findings(ResolveProfile(p, "en", "child", ""), "risk-free", nil), 2)
}
