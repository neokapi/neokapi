package backend

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func channelRow(t *testing.T, res *ChannelMapResult, ref string) *ChannelMapRow {
	t.Helper()
	for i := range res.Channels {
		if res.Channels[i].Ref == ref {
			return &res.Channels[i]
		}
	}
	return nil
}

func TestChannelMap_ListsDeclaredChannels(t *testing.T) {
	app := NewApp()
	tab, _ := newContextProject(t, app)

	res, err := app.ChannelMap(tab.ID)
	require.NoError(t, err)

	docs := channelRow(t, res, "support/docs")
	require.NotNil(t, docs, "the map lists the support/docs channel")
	assert.True(t, docs.Declared)
	assert.Equal(t, "support", docs.Profile)
	assert.Equal(t, "docs", docs.Channel)
	assert.Contains(t, docs.Collections, "Docs")

	promo := channelRow(t, res, "campaign/promo")
	require.NotNil(t, promo, "the map lists the campaign/promo channel")
	assert.True(t, promo.Declared)

	// Every collection here names a declared channel, so nothing is derived.
	for _, c := range res.Channels {
		assert.True(t, c.Declared, "channel %q", c.Ref)
	}
}

func TestDeclareChannel_PersistsToRecipe(t *testing.T) {
	app := NewApp()
	tab, root := newContextProject(t, app)

	proj, err := app.DeclareChannel(tab.ID, "support", "guides")
	require.NoError(t, err)
	require.NotNil(t, proj)
	declared := false
	for _, ch := range proj.Profiles["support"].Channels {
		if ch.ID == "guides" {
			declared = true
		}
	}
	assert.True(t, declared, "the profile declares the new channel")

	// The recipe on disk carries it: a fresh open sees the new channel.
	fresh := NewApp()
	tab2, err := fresh.OpenProject(filepath.Join(root, "kapi.yaml"))
	require.NoError(t, err)
	t.Cleanup(func() { fresh.CloseProject(tab2.ID) })
	res, err := fresh.ChannelMap(tab2.ID)
	require.NoError(t, err)
	assert.NotNil(t, channelRow(t, res, "support/guides"))

	// A duplicate is refused.
	_, err = app.DeclareChannel(tab.ID, "support", "guides")
	require.Error(t, err)
}

func TestRenameChannel_MovesCollections(t *testing.T) {
	app := NewApp()
	tab, root := newContextProject(t, app)

	_, err := app.RenameChannel(tab.ID, "support", "docs", "guides")
	require.NoError(t, err)

	// A fresh open sees the channel renamed and the Docs collection moved with it.
	fresh := NewApp()
	tab2, err := fresh.OpenProject(filepath.Join(root, "kapi.yaml"))
	require.NoError(t, err)
	t.Cleanup(func() { fresh.CloseProject(tab2.ID) })
	res, err := fresh.ChannelMap(tab2.ID)
	require.NoError(t, err)

	assert.Nil(t, channelRow(t, res, "support/docs"), "the old channel is gone")
	guides := channelRow(t, res, "support/guides")
	require.NotNil(t, guides)
	assert.Contains(t, guides.Collections, "Docs")

	// A channel no profile declares cannot be renamed.
	_, err = app.RenameChannel(tab.ID, "support", "nope", "still-nope")
	require.Error(t, err)
}
