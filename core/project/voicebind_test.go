package project

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeBindRecipe(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), RecipeFileName)
	require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
	return path
}

func readBindRecipe(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	return string(data)
}

// TestBindVoice_InsertsOnlyTheBinding pins that binding a voice adds two lines
// and leaves every other byte of the recipe as written: the comments, the blank
// lines, the flow-style list and the odd spacing all survive.
func TestBindVoice_InsertsOnlyTheBinding(t *testing.T) {
	path := writeBindRecipe(t, `# Fernwell docs.
version: v1
name: fernwell   # the label

defaults:
    # Where the source sits.
    source_language: en
    target_languages: [nb,  de]   # two for now

collections:
  - path: "docs/**/*.md"
`)
	require.NoError(t, BindVoice(path, "", "fernwell"))
	assert.Equal(t, `# Fernwell docs.
version: v1
name: fernwell   # the label

defaults:
    voice:
        profile: fernwell
    # Where the source sits.
    source_language: en
    target_languages: [nb,  de]   # two for now

collections:
  - path: "docs/**/*.md"
`, readBindRecipe(t, path))

	p, err := Load(path)
	require.NoError(t, err)
	require.NotNil(t, p.Defaults.Voice)
	assert.Equal(t, "fernwell", p.Defaults.Voice.Profile)
}

func TestBindVoice_NoDefaultsAppendsOne(t *testing.T) {
	path := writeBindRecipe(t, "version: v1\nname: bare")
	require.NoError(t, BindVoice(path, "", "house"))
	assert.Equal(t, "version: v1\nname: bare\ndefaults:\n  voice:\n    profile: house\n", readBindRecipe(t, path))
}

func TestBindVoice_EmptyDefaults(t *testing.T) {
	path := writeBindRecipe(t, "version: v1\nname: e\ndefaults:\ncollections: []\n")
	require.NoError(t, BindVoice(path, "", "house"))
	assert.Equal(t, "version: v1\nname: e\ndefaults:\n  voice:\n    profile: house\ncollections: []\n", readBindRecipe(t, path))
}

func TestBindVoice_QuotesAnIDThatNeedsIt(t *testing.T) {
	path := writeBindRecipe(t, "version: v1\nname: q\ndefaults:\n  source_language: en\n")
	require.NoError(t, BindVoice(path, "", "yes"))
	p, err := Load(path)
	require.NoError(t, err)
	assert.Equal(t, "yes", p.Defaults.Voice.Profile, "a scalar YAML reads as a boolean is quoted")
}

func TestBindVoice_UnderAProfile(t *testing.T) {
	path := writeBindRecipe(t, `version: v1
name: p
profiles:
  acme:
    channels: [docs]
`)
	require.NoError(t, BindVoice(path, "acme", "acme-voice"))
	assert.Equal(t, `version: v1
name: p
profiles:
  acme:
    voice:
      profile: acme-voice
    channels: [docs]
`, readBindRecipe(t, path))

	assert.ErrorContains(t, BindVoice(path, "other", "x"), `declares no profile "other"`)
}

func TestBindVoice_RefusesAnExistingBinding(t *testing.T) {
	body := "version: v1\nname: b\ndefaults:\n  voice:\n    pack: professional-b2b\n"
	path := writeBindRecipe(t, body)
	require.ErrorIs(t, BindVoice(path, "", "house"), ErrVoiceAlreadyBound)
	assert.Equal(t, body, readBindRecipe(t, path), "a refused binding writes nothing")
}

func TestBindVoice_FlowStyleDefaultsFallsBackToSave(t *testing.T) {
	path := writeBindRecipe(t, "version: v1\nname: f\ndefaults: {source_language: en}\n")
	require.NoError(t, BindVoice(path, "", "house"))
	p, err := Load(path)
	require.NoError(t, err)
	assert.Equal(t, "house", p.Defaults.Voice.Profile)
	assert.Equal(t, "en", string(p.Defaults.SourceLanguage))
}

func TestBindVoice_KeepsCRLF(t *testing.T) {
	path := writeBindRecipe(t, "version: v1\r\nname: w\r\ndefaults:\r\n  source_language: en\r\n")
	require.NoError(t, BindVoice(path, "", "house"))
	assert.Equal(t, "version: v1\r\nname: w\r\ndefaults:\r\n  voice:\r\n    profile: house\r\n  source_language: en\r\n",
		readBindRecipe(t, path))
}
