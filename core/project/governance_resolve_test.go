package project

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ladderRecipe declares the three rungs a point can bind on: a content item's
// own channel, its collection's, and the project's default. `promo` bounds its
// governance in time, `evergreen` does not, so one recipe covers the whole
// resolution table.
const ladderRecipe = `version: v1
name: ladder
defaults:
  source_language: en
  target_languages: [nb]
  voice:
    profile_file: .kapi/voice.yaml
profiles:
  promo:
    channels: [landing, legal]
    voice: .kapi/profiles/promo/voice.yaml
    valid_from: 2026-01-01
    valid_to: 2026-07-01
  upcoming:
    channels: [landing]
    voice: .kapi/profiles/upcoming/voice.yaml
    valid_from: 2026-09-01
  evergreen:
    channels: [docs, legal]
    voice: .kapi/profiles/evergreen/voice.yaml
collections:
  - name: docs
    channel: evergreen/docs
    content:
      - path: docs/legal/**/*.mdx
        channel: promo/legal
      - path: docs/**/*.mdx
  - name: landing
    channel: promo/landing
    content:
      - path: site/**/*.mdx
  - name: soon
    channel: upcoming/landing
    content:
      - path: soon/**/*.mdx
  - name: loose
    content:
      - path: README.md
`

func TestResolveGovernanceFor(t *testing.T) {
	p := loadRecipe(t, ladderRecipe)

	inWindow := mustDate(t, "2026-03-15")
	afterWindow := mustDate(t, "2026-09-15")
	beforeWindow := mustDate(t, "2025-12-01")

	tests := []struct {
		name         string
		point        GovernancePoint
		wantProfile  string
		wantChannel  string
		wantVoice    string
		wantFallback string
	}{
		{
			name:        "a file takes its item's own channel over its collection's",
			point:       GovernancePoint{Path: "docs/legal/terms.mdx", At: inWindow},
			wantProfile: "promo",
			wantChannel: "legal",
			wantVoice:   ".kapi/profiles/promo/voice.yaml",
		},
		{
			name:        "a sibling file keeps its collection's channel",
			point:       GovernancePoint{Path: "docs/guide/intro.mdx", At: inWindow},
			wantProfile: "evergreen",
			wantChannel: "docs",
			wantVoice:   ".kapi/profiles/evergreen/voice.yaml",
		},
		{
			name:        "a collection resolves without a path",
			point:       GovernancePoint{Collection: "landing", At: inWindow},
			wantProfile: "promo",
			wantChannel: "landing",
			wantVoice:   ".kapi/profiles/promo/voice.yaml",
		},
		{
			name:        "an unbounded profile is untouched by the instant",
			point:       GovernancePoint{Path: "docs/guide/intro.mdx", At: afterWindow},
			wantProfile: "evergreen",
			wantChannel: "docs",
			wantVoice:   ".kapi/profiles/evergreen/voice.yaml",
		},
		{
			name:         "an expired item override falls through to the collection's channel",
			point:        GovernancePoint{Path: "docs/legal/terms.mdx", At: afterWindow},
			wantProfile:  "evergreen",
			wantChannel:  "docs",
			wantVoice:    ".kapi/profiles/evergreen/voice.yaml",
			wantFallback: `profile "promo" expired 2026-07-01; governing with profile "evergreen"`,
		},
		{
			name:         "an expired collection channel falls through to the project default",
			point:        GovernancePoint{Collection: "landing", At: afterWindow},
			wantVoice:    ".kapi/voice.yaml",
			wantFallback: `profile "promo" expired 2026-07-01; governing with the project default`,
		},
		{
			name:         "a profile not yet in force does not govern either",
			point:        GovernancePoint{Path: "site/index.mdx", At: beforeWindow},
			wantVoice:    ".kapi/voice.yaml",
			wantFallback: `profile "promo" is not in force until 2026-01-01; governing with the project default`,
		},
		{
			name:         "an upcoming profile reports its own boundary",
			point:        GovernancePoint{Path: "soon/index.mdx", At: inWindow},
			wantVoice:    ".kapi/voice.yaml",
			wantFallback: `profile "upcoming" is not in force until 2026-09-01; governing with the project default`,
		},
		{
			name:      "a path no item claims sits at the default point",
			point:     GovernancePoint{Path: "nothing/claims/this.md", At: inWindow},
			wantVoice: ".kapi/voice.yaml",
		},
		{
			name:        "a path no item claims still belongs to the collection the caller named",
			point:       GovernancePoint{Collection: "landing", Path: "nothing/claims/this.md", At: inWindow},
			wantProfile: "promo",
			wantChannel: "landing",
			wantVoice:   ".kapi/profiles/promo/voice.yaml",
		},
		{
			name:      "the as-declared view applies no window",
			point:     GovernancePoint{Collection: "landing"},
			wantVoice: ".kapi/profiles/promo/voice.yaml", wantProfile: "promo", wantChannel: "landing",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rc, err := p.ResolveGovernanceFor(tt.point)
			require.NoError(t, err)
			assert.Equal(t, tt.wantProfile, rc.Profile, "profile")
			assert.Equal(t, tt.wantChannel, rc.Channel, "channel")
			require.NotNil(t, rc.Voice)
			assert.Equal(t, tt.wantVoice, rc.Voice.ProfileFile, "voice")
			if tt.wantFallback == "" {
				assert.Nil(t, rc.Fallback, "no transition to report")
				return
			}
			require.NotNil(t, rc.Fallback, "the transition must be reportable")
			assert.Equal(t, tt.wantFallback, rc.Fallback.String())
		})
	}
}

// TestResolveGovernanceFor_ValidToIsExclusive pins the boundary itself: the day
// named by valid_to is already outside the window, matching the half-open model
// terms and graph edges carry.
func TestResolveGovernanceFor_ValidToIsExclusive(t *testing.T) {
	p := loadRecipe(t, ladderRecipe)

	last, err := p.ResolveGovernanceFor(GovernancePoint{
		Collection: "landing",
		At:         mustDate(t, "2026-06-30").Add(23 * time.Hour),
	})
	require.NoError(t, err)
	assert.Equal(t, "promo", last.Profile, "the last instant inside the window still governs")

	boundary, err := p.ResolveGovernanceFor(GovernancePoint{
		Collection: "landing",
		At:         mustDate(t, "2026-07-01"),
	})
	require.NoError(t, err)
	assert.Empty(t, boundary.Profile, "valid_to is exclusive")
	require.NotNil(t, boundary.Fallback)
	assert.True(t, boundary.Fallback.Expired)
}

// TestResolvedGovernance_RefCarriesWhatGoverned is why a push can trust the
// resolution: the coordinates come from what actually governed, so an expired
// profile puts nothing on the wire rather than a point it no longer holds.
func TestResolvedGovernance_RefCarriesWhatGoverned(t *testing.T) {
	p := loadRecipe(t, ladderRecipe)

	within, err := p.ResolveGovernanceFor(GovernancePoint{Collection: "landing", At: mustDate(t, "2026-03-15")})
	require.NoError(t, err)
	assert.Equal(t, "promo/landing", within.Ref().String())
	assert.Equal(t, map[string]string{ProductAxis: "promo", ChannelAxis: "landing"}, within.Ref().Coordinates())

	after, err := p.ResolveGovernanceFor(GovernancePoint{Collection: "landing", At: mustDate(t, "2026-09-15")})
	require.NoError(t, err)
	assert.Empty(t, after.Ref().String())
	assert.Nil(t, after.Ref().Coordinates(), "the default point puts no coordinates on the wire")
}
