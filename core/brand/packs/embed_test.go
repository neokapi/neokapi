package packs

import (
	"testing"

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
	profile, err := Load("professional-b2b")
	require.NoError(t, err)
	assert.Equal(t, "Professional B2B", profile.Name)
	assert.NotEmpty(t, profile.Tone.Personality)
	assert.Equal(t, "formal", profile.Tone.Formality)
	assert.NotEmpty(t, profile.Examples)
	assert.NotEmpty(t, profile.Vocabulary.ForbiddenTerms)
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
