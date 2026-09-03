package project

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDeclareChannel(t *testing.T) {
	p := &KapiProject{}

	require.NoError(t, p.DeclareChannel("campaign", "promo"))
	require.True(t, p.declaresChannelRef("campaign", "promo"))

	// A second channel on the same profile leaves the first alone.
	require.NoError(t, p.DeclareChannel("campaign", "email"))
	assert.Len(t, p.Profiles["campaign"].Channels, 2)

	// Duplicates and non-slugs are refused.
	assert.Error(t, p.DeclareChannel("campaign", "promo"))
	assert.Error(t, p.DeclareChannel("Campaign", "promo"))
	assert.Error(t, p.DeclareChannel("campaign", "Promo"))
}

func TestRenameChannel_MovesCollections(t *testing.T) {
	p := &KapiProject{
		Profiles:    map[string]Profile{"campaign": {Channels: []Channel{{ID: "promo"}}}},
		Collections: []Collection{{Name: "Ads", Channel: "campaign/promo"}},
	}

	require.NoError(t, p.RenameChannel("campaign", "promo", "email"))
	assert.True(t, p.declaresChannelRef("campaign", "email"))
	assert.False(t, p.declaresChannelRef("campaign", "promo"))
	// The collection that named the old channel moved with it.
	assert.Equal(t, "campaign/email", p.Collections[0].Channel)
}

func TestRenameChannel_Refusals(t *testing.T) {
	p := &KapiProject{
		Profiles: map[string]Profile{"campaign": {Channels: []Channel{{ID: "promo"}, {ID: "email"}}}},
	}

	// A channel the profile does not declare (i.e. derived) cannot be renamed.
	assert.Error(t, p.RenameChannel("campaign", "web", "site"))
	// The target must not already exist.
	assert.Error(t, p.RenameChannel("campaign", "promo", "email"))
	// The new id must be a slug.
	assert.Error(t, p.RenameChannel("campaign", "promo", "Promo"))
	// Renaming to itself is a no-op, not an error.
	assert.NoError(t, p.RenameChannel("campaign", "promo", "promo"))
}
