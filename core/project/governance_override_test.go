package project

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// overrideRecipe binds a docs collection to bowrain/docs, but routes its legal
// sub-tree to a different profile with a per-file channel override.
const overrideRecipe = `version: v1
name: overrides
defaults:
  source_language: en
  target_languages: [nb]
  voice:
    profile_file: .kapi/voice.yaml
profiles:
  bowrain:
    channels: [app, docs]
    voice: .kapi/profiles/bowrain/voice.yaml
  neokapi:
    channels: [cli, docs]
    voice: .kapi/profiles/neokapi/voice.yaml
collections:
  - name: docs
    channel: bowrain/docs
    content:
      - path: docs/legal/**/*.mdx
        channel: neokapi/docs
      - path: docs/**/*.mdx
  - name: loose
    content:
      - path: README.md
`

func TestKapiProject_ResolveGovernanceForPath(t *testing.T) {
	p := loadRecipe(t, overrideRecipe)

	t.Run("a file overriding one axis resolves a different profile", func(t *testing.T) {
		rc, err := p.ResolveGovernanceForPath("docs/legal/terms.mdx")
		require.NoError(t, err)
		assert.Equal(t, "neokapi", rc.Profile, "the per-file channel override wins over the collection's")
		assert.Equal(t, "docs", rc.Channel)
		assert.Equal(t, ".kapi/profiles/neokapi/voice.yaml", rc.Voice.ProfileFile)
	})

	t.Run("a sibling file keeps the collection's profile", func(t *testing.T) {
		rc, err := p.ResolveGovernanceForPath("docs/guide/intro.mdx")
		require.NoError(t, err)
		assert.Equal(t, "bowrain", rc.Profile)
		assert.Equal(t, ".kapi/profiles/bowrain/voice.yaml", rc.Voice.ProfileFile)
	})

	t.Run("a path no item claims sits at the default point", func(t *testing.T) {
		rc, err := p.ResolveGovernanceForPath("nothing/claims/this.md")
		require.NoError(t, err)
		assert.Empty(t, rc.Profile)
		assert.Equal(t, DefaultVoiceField, rc.VoiceField)
	})

	t.Run("an unbound collection's file sits at the default point", func(t *testing.T) {
		rc, err := p.ResolveGovernanceForPath("README.md")
		require.NoError(t, err)
		assert.Empty(t, rc.Profile)
	})
}

func TestKapiProject_ItemChannel_UndeclaredAxisIsALoadError(t *testing.T) {
	const bad = `version: v1
name: bad-override
defaults:
  source_language: en
  target_languages: [nb]
profiles:
  neokapi:
    channels: [cli, docs]
collections:
  - name: docs
    channel: neokapi/docs
    content:
      - path: docs/legal/**/*.mdx
        channel: neokapi/marketing
`
	var p KapiProject
	require.NoError(t, yaml.Unmarshal([]byte(bad), &p))
	err := p.Validate()
	require.Error(t, err, "an item channel naming an undeclared axis fails at load")
	assert.Contains(t, err.Error(), "content[0]")
	assert.Contains(t, err.Error(), "not declared by profile")
}
