package project

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func channelProject() *KapiProject {
	return &KapiProject{
		Version: "v1",
		Name:    "Channels",
		Collections: []Collection{
			{Name: "docs", Path: "docs/**/*.md"},
			{Name: "app", Path: "app/**/*.json"},
		},
	}
}

func set(t *testing.T, p *KapiProject, path, value string) (bool, error) {
	t.Helper()
	raw, err := json.Marshal(value)
	require.NoError(t, err)
	return SetField(p, path, raw)
}

// TestSetCollectionChannelDeclaresThePointItNames: a `channel:` naming a
// product that `profiles:` does not declare will not load, so writing one and
// leaving the declaration to a second change would mean an ordering contract
// between two rows — and a working tree that does not load if they arrive out
// of order. One edit, one coherent recipe.
func TestSetCollectionChannelDeclaresThePointItNames(t *testing.T) {
	p := channelProject()

	changed, err := set(t, p, "collections.docs.channel", "cloud/docs")
	require.NoError(t, err)
	assert.True(t, changed)
	assert.Equal(t, "cloud/docs", p.collectionNamed("docs").Channel)

	require.Contains(t, p.Profiles, "cloud", "the product it names is declared")
	assert.True(t, declaresChannel(p.Profiles["cloud"].Channels, "docs"))

	// And the recipe loads: the reference resolves against what was declared.
	ref, err := p.ResolveChannel("cloud/docs")
	require.NoError(t, err)
	assert.Equal(t, ChannelRef{Profile: "cloud", Channel: "docs"}, ref)
	assert.Equal(t, map[string]string{ProductAxis: "cloud", ChannelAxis: "docs"}, ref.Coordinates())
}

// A second collection at the same product adds its channel beside the first
// rather than replacing the profile.
func TestSetCollectionChannelExtendsAnExistingProduct(t *testing.T) {
	p := channelProject()
	_, err := set(t, p, "collections.docs.channel", "cloud/docs")
	require.NoError(t, err)
	_, err = set(t, p, "collections.app.channel", "cloud/web")
	require.NoError(t, err)

	assert.Len(t, p.Profiles["cloud"].Channels, 2)
	assert.True(t, declaresChannel(p.Profiles["cloud"].Channels, "docs"))
	assert.True(t, declaresChannel(p.Profiles["cloud"].Channels, "web"))
}

// Setting the same value twice reports no change, so a pull that replays a
// pending row does not dirty the working tree.
func TestSetCollectionChannelIsIdempotent(t *testing.T) {
	p := channelProject()
	_, err := set(t, p, "collections.docs.channel", "cloud/docs")
	require.NoError(t, err)

	changed, err := set(t, p, "collections.docs.channel", "cloud/docs")
	require.NoError(t, err)
	assert.False(t, changed, "the recipe already says this")
}

// An empty value withdraws the binding, so a collection falls back to the
// project's default point. The declaration it made is left standing: another
// collection may sit there, and removing a product because one collection
// moved off it is not this change's decision to make.
func TestSetCollectionChannelWithdrawsTheBinding(t *testing.T) {
	p := channelProject()
	_, err := set(t, p, "collections.docs.channel", "cloud/docs")
	require.NoError(t, err)

	changed, err := set(t, p, "collections.docs.channel", "")
	require.NoError(t, err)
	assert.True(t, changed)
	assert.Empty(t, p.collectionNamed("docs").Channel)
	assert.Contains(t, p.Profiles, "cloud")
}

func TestSetCollectionChannelRefusals(t *testing.T) {
	cases := []struct {
		name, path, value, wants string
	}{
		{"unknown collection", "collections.nope.channel", "cloud/docs", "no collection named"},
		{"unqualified", "collections.docs.channel", "docs", "must be `product/channel`"},
		{"too many parts", "collections.docs.channel", "a/b/c", "too many parts"},
		{"not a slug", "collections.docs.channel", "Cloud/Docs", "on both sides of the slash"},
		{"another field", "collections.docs.base", "docs/", "is not settable"},
		{"no field", "collections.docs", "cloud/docs", "not a settable collection field"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := channelProject()
			changed, err := set(t, p, tc.path, tc.value)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wants)
			assert.False(t, changed)
		})
	}
}

// The declared/derived split still holds at the other end: a structural axis is
// refused as a default coordinate, and the error names where it belongs.
func TestStructuralAxesAreStillRefusedAsDefaults(t *testing.T) {
	for _, axis := range []string{ProductAxis, ChannelAxis} {
		p := channelProject()
		_, err := set(t, p, "defaults.coordinates."+axis, "cloud")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "set the collection's channel instead")
	}
}
