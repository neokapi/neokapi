package project

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// twoProductRecipe is the shape this repository itself carries: two profiles,
// one channel (`docs`) shared by both, the rest unique to one.
const twoProductRecipe = `version: v1
name: two-products
defaults:
  source_language: en
  target_languages: [nb]
  voice:
    profile_file: .kapi/voice.yaml
profiles:
  neokapi:
    channels: [cli, docs]
    voice: .kapi/voice.yaml
  bowrain:
    channels: [app, docs]
    voice: .kapi/profiles/bowrain/voice.yaml
    termstore: .kapi/profiles/bowrain/terms.json
collections:
  - name: neokapi-cli
    channel: neokapi/cli
    content:
      - path: host/i18n/commands.json
  - name: bowrain-docs
    channel: bowrain/docs
    base: bowrain/web/docs
    content:
      - path: docs/**/*.mdx
        target: i18n/{lang}/current/{path}.mdx
  - name: loose
    content:
      - path: README.md
`

func loadRecipe(t *testing.T, src string) *KapiProject {
	t.Helper()
	var p KapiProject
	require.NoError(t, yaml.Unmarshal([]byte(src), &p))
	require.NoError(t, p.Validate())
	return &p
}

func TestKapiProject_ProfilesAreKeyed(t *testing.T) {
	p := loadRecipe(t, twoProductRecipe)

	require.Len(t, p.Profiles, 2)
	assert.Equal(t, ".kapi/profiles/bowrain/voice.yaml", p.Profiles["bowrain"].Voice.ProfileFile)
	assert.Equal(t, ".kapi/profiles/bowrain/terms.json", p.Profiles["bowrain"].TermStore)
	assert.True(t, p.HasContextSpace())
	assert.True(t, p.BindsTermsByProfile())
}

func TestKapiProject_ResolveChannel(t *testing.T) {
	p := loadRecipe(t, twoProductRecipe)

	tests := []struct {
		name string
		ref  string
		want ChannelRef
	}{
		{"the empty ref is the default point", "", ChannelRef{}},
		{"a qualified ref names both", "bowrain/docs", ChannelRef{Profile: "bowrain", Channel: "docs"}},
		{"a qualified ref on the other profile", "neokapi/cli", ChannelRef{Profile: "neokapi", Channel: "cli"}},
		{"the other side of the shared channel", "neokapi/docs", ChannelRef{Profile: "neokapi", Channel: "docs"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := p.ResolveChannel(tt.ref)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestKapiProject_ResolveChannel_Errors(t *testing.T) {
	p := loadRecipe(t, twoProductRecipe)

	tests := []struct {
		name    string
		ref     string
		wantSub []string
	}{
		{
			"a bare channel spells out the qualified form",
			"cli",
			[]string{"must name its profile", "neokapi/cli"},
		},
		{
			"a bare channel two profiles declare spells out both",
			"docs",
			[]string{"must name its profile", "bowrain/docs", "neokapi/docs"},
		},
		{
			"a bare channel nothing declares names what is declared",
			"landing",
			[]string{"must be `profile/channel`", "bowrain/app", "neokapi/cli"},
		},
		{
			"an unknown profile names the declared profiles",
			"acme/docs",
			[]string{`names profile "acme"`, "bowrain, neokapi"},
		},
		{
			"a profile that does not declare that channel says so",
			"neokapi/app",
			[]string{`not declared by profile "neokapi"`, "cli, docs"},
		},
		{
			"a three-part ref is malformed",
			"a/b/c",
			[]string{"too many parts"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := p.ResolveChannel(tt.ref)
			require.Error(t, err)
			for _, sub := range tt.wantSub {
				assert.Contains(t, err.Error(), sub)
			}
		})
	}
}

func TestChannelRef_Coordinates(t *testing.T) {
	assert.Nil(t, ChannelRef{}.Coordinates(), "the default point puts nothing on the wire")
	assert.Equal(t,
		map[string]string{ProductAxis: "bowrain", ChannelAxis: "docs"},
		ChannelRef{Profile: "bowrain", Channel: "docs"}.Coordinates())
}

func TestKapiProject_ResolveGovernance(t *testing.T) {
	p := loadRecipe(t, twoProductRecipe)

	t.Run("a bound collection carries its profile", func(t *testing.T) {
		rc, err := p.ResolveGovernance("bowrain-docs")
		require.NoError(t, err)
		assert.Equal(t, "bowrain", rc.Profile)
		assert.Equal(t, "docs", rc.Channel)
		assert.Equal(t, ".kapi/profiles/bowrain/voice.yaml", rc.Voice.ProfileFile)
		assert.Equal(t, ".kapi/profiles/bowrain/terms.json", rc.TermStore)
		assert.Equal(t, "profiles.bowrain.voice", rc.VoiceField)
	})

	t.Run("an unbound collection sits at the default point", func(t *testing.T) {
		rc, err := p.ResolveGovernance("loose")
		require.NoError(t, err)
		assert.Empty(t, rc.Profile)
		assert.Empty(t, rc.Channel)
		assert.Empty(t, rc.TermStore)
		assert.Equal(t, ".kapi/voice.yaml", rc.Voice.ProfileFile)
		assert.Equal(t, DefaultVoiceField, rc.VoiceField)
	})

	t.Run("a name no collection claims resolves the default point", func(t *testing.T) {
		rc, err := p.ResolveGovernance("nothing-declares-this")
		require.NoError(t, err)
		assert.Empty(t, rc.Profile)
		assert.Equal(t, DefaultVoiceField, rc.VoiceField)
	})
}

func TestKapiProject_ValidateContextSpace(t *testing.T) {
	tests := []struct {
		name    string
		recipe  string
		wantSub string
	}{
		{
			"an unresolvable channel fails the load",
			"version: v1\nprofiles:\n  a:\n    channels: [x]\ncollections:\n  - name: c\n    channel: a/y\n    content:\n      - path: a.md\n",
			`channel "a/y" is not declared by profile "a"`,
		},
		{
			"a bare channel fails the load spelling out the fix",
			"version: v1\nprofiles:\n  a:\n    channels: [x]\n  b:\n    channels: [x]\ncollections:\n  - name: c\n    channel: x\n    content:\n      - path: a.md\n",
			"must name its profile. Write a/x or b/x",
		},
		{
			"a channel needs a named collection",
			"version: v1\nprofiles:\n  a:\n    channels: [x]\ncollections:\n  - channel: x\n    path: a.md\n",
			"channel requires a named collection",
		},
		{
			"a profile name must be a slug",
			"version: v1\nprofiles:\n  Not A Slug:\n    channels: [x]\n",
			"is not a slug",
		},
		{
			"a channel must be a slug",
			"version: v1\nprofiles:\n  a:\n    channels: [\"Not A Slug\"]\n",
			"is not a slug",
		},
		{
			"a channel declared twice is a mistake",
			"version: v1\nprofiles:\n  a:\n    channels: [x, x]\n",
			`channel "x" is declared twice`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var p KapiProject
			require.NoError(t, yaml.Unmarshal([]byte(tt.recipe), &p))
			err := p.Validate()
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantSub)
		})
	}
}

func TestChannel_RoundTrip(t *testing.T) {
	const src = `version: v1
profiles:
  a:
    concept: term:9a1c0f42b7
    channels:
      - docs
      - id: app
        concept: term:0f42b79a1c
`
	p := loadRecipe(t, src)
	prof := p.Profiles["a"]
	assert.Equal(t, "term:9a1c0f42b7", prof.Concept)
	require.Len(t, prof.Channels, 2)
	assert.Equal(t, Channel{ID: "docs"}, prof.Channels[0])
	assert.Equal(t, Channel{ID: "app", Concept: "term:0f42b79a1c"}, prof.Channels[1])

	out, err := yaml.Marshal(p)
	require.NoError(t, err)
	assert.Contains(t, string(out), "- docs", "a plain channel round-trips as a scalar")
	assert.Contains(t, string(out), "concept: term:0f42b79a1c")
}

func TestKapiProject_CollectionForPath(t *testing.T) {
	p := loadRecipe(t, twoProductRecipe)

	assert.Equal(t, "bowrain-docs", p.CollectionForPath("bowrain/web/docs/docs/guide/intro.mdx"),
		"base joins onto the item pattern before matching")
	assert.Equal(t, "neokapi-cli", p.CollectionForPath("host/i18n/commands.json"))
	assert.Empty(t, p.CollectionForPath("nothing/claims/this.md"))
}
