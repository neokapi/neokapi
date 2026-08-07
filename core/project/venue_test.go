package project

import (
	"testing"

	"github.com/neokapi/neokapi/core/project/projecttest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// registerVenue installs a venue extension under an arbitrary key, so these
// tests assert the framework finds a venue by its registration and never by a
// key name it knows.
func registerVenue(t *testing.T, key string) {
	t.Helper()
	projecttest.ResetExtensions()
	t.Cleanup(projecttest.ResetExtensions)
	RegisterExtensionGroup("platform", []Extension{
		{Name: key, Scope: ScopeProject, Venue: true},
		{Name: "automations", Scope: ScopeProject, DependsOn: key},
	})
}

func TestKapiProject_Venue(t *testing.T) {
	registerVenue(t, "acme")

	const src = `version: v1
acme:
  url: "https://acme.example/ws/proj"
  converge: manual
`
	var p KapiProject
	require.NoError(t, yaml.Unmarshal([]byte(src), &p))
	require.NoError(t, p.Validate())

	venue, ok := p.Venue()
	require.True(t, ok)
	assert.Equal(t, "acme", venue.Key)
	assert.Equal(t, "https://acme.example/ws/proj", venue.URL)
	assert.Equal(t, "manual", venue.Converge)
}

func TestKapiProject_Venue_Absent(t *testing.T) {
	registerVenue(t, "acme")

	t.Run("a recipe binding none reports none", func(t *testing.T) {
		var p KapiProject
		require.NoError(t, yaml.Unmarshal([]byte("version: v1\n"), &p))
		_, ok := p.Venue()
		assert.False(t, ok)
	})

	t.Run("an unregistered key of the same shape is not a venue", func(t *testing.T) {
		var p KapiProject
		require.NoError(t, yaml.Unmarshal([]byte("version: v1\nsomeoneelse:\n  url: https://x.test\n"), &p))
		_, ok := p.Venue()
		assert.False(t, ok, "a key no extension claims carries no framework meaning")
	})
}

func TestVenueKey(t *testing.T) {
	registerVenue(t, "acme")

	key, ok := VenueKey()
	require.True(t, ok)
	assert.Equal(t, "acme", key)
	assert.True(t, IsVenueKey("acme"))
	assert.False(t, IsVenueKey("automations"))
}

func TestVenueKey_NoneRegistered(t *testing.T) {
	projecttest.ResetExtensions()
	t.Cleanup(projecttest.ResetExtensions)

	_, ok := VenueKey()
	assert.False(t, ok)
	assert.False(t, IsVenueKey("acme"))
}

func TestKapiProject_InertProjectExtras_DependsOnVenue(t *testing.T) {
	registerVenue(t, "acme")

	var p KapiProject
	require.NoError(t, yaml.Unmarshal([]byte("version: v1\nautomations: []\n"), &p))
	require.NoError(t, p.Validate())

	inert := p.InertProjectExtras()
	require.Len(t, inert, 1)
	assert.Equal(t, "automations", inert[0].Name)
	assert.Equal(t, "acme", inert[0].DependsOn)
	assert.True(t, IsVenueKey(inert[0].DependsOn))
}
