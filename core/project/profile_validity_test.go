package project

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

const validityRecipe = `version: v1
name: seasonal
defaults:
  source_language: en
  target_languages: [nb]
  voice:
    profile_file: .kapi/voice.yaml
profiles:
  spring-sale:
    channels: [landing]
    voice: .kapi/profiles/spring-sale/voice.yaml
    valid_from: 2026-01-01
    valid_to: 2026-07-01
  evergreen:
    channels: [docs]
    voice: .kapi/profiles/evergreen/voice.yaml
collections:
  - name: promo
    channel: spring-sale/landing
    content:
      - path: promo/**/*.md
`

func mustDate(t *testing.T, s string) time.Time {
	t.Helper()
	d, err := time.Parse("2006-01-02", s)
	require.NoError(t, err)
	return d
}

func TestProfileValidity_CarriedAndParsed(t *testing.T) {
	p := loadRecipe(t, validityRecipe)

	rc, err := p.ResolveGovernance("promo")
	require.NoError(t, err)
	require.NotNil(t, rc.Validity, "a bounded profile carries its window")
	require.NotNil(t, rc.Validity.ValidFrom)
	assert.Equal(t, mustDate(t, "2026-01-01").UTC(), rc.Validity.ValidFrom.UTC())
	require.NotNil(t, rc.Validity.ValidTo)
	assert.Equal(t, mustDate(t, "2026-07-01").UTC(), rc.Validity.ValidTo.UTC())

	assert.True(t, rc.ActiveAt(mustDate(t, "2026-03-15")), "in force mid-window")
	assert.False(t, rc.ActiveAt(mustDate(t, "2025-12-31")), "not yet in force")
	assert.False(t, rc.ActiveAt(mustDate(t, "2026-07-01")), "valid_to is exclusive")
}

func TestResolveGovernanceAt_HonoursTheWindow(t *testing.T) {
	p := loadRecipe(t, validityRecipe)

	within, err := p.ResolveGovernanceAt("promo", mustDate(t, "2026-03-15"))
	require.NoError(t, err)
	assert.Equal(t, "spring-sale", within.Profile, "inside its window the profile governs")

	after, err := p.ResolveGovernanceAt("promo", mustDate(t, "2026-09-01"))
	require.NoError(t, err)
	assert.Empty(t, after.Profile, "outside its window the profile stops governing")
	assert.Equal(t, DefaultVoiceField, after.VoiceField, "resolution falls back to the default point")
}

func TestProfileWindows_ListsOnlyBoundedProfiles(t *testing.T) {
	p := loadRecipe(t, validityRecipe)

	windows := p.ProfileWindows()
	require.Len(t, windows, 1, "the unbounded profile is omitted")
	assert.Equal(t, "spring-sale", windows[0].Name)
	require.NotNil(t, windows[0].Validity.ValidTo)
}

func TestProfileValidity_LoadErrors(t *testing.T) {
	tests := []struct {
		name    string
		valid   string
		wantSub string
	}{
		{"an unparseable date", "valid_from: not-a-date", "is not a date"},
		{"an inverted range", "valid_from: 2026-07-01\n    valid_to: 2026-01-01", "before valid_from"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := `version: v1
name: bad
defaults:
  source_language: en
  target_languages: [nb]
profiles:
  p:
    channels: [docs]
    ` + tt.valid + `
collections: []
`
			var p KapiProject
			require.NoError(t, yaml.Unmarshal([]byte(src), &p))
			err := p.Validate()
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantSub)
		})
	}
}
