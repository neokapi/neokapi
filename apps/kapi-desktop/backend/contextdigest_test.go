package backend

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/host"
)

// TestDigestReadsWhatAnotherProcessRecorded: an agent suggests in its own
// process, the tab's digest shows it under its theme, a group keep
// establishes it, and the marker moves only when the person has looked.
func TestDigestReadsWhatAnotherProcessRecorded(t *testing.T) {
	t.Setenv("KAPI_CONFIG_DIR", t.TempDir())
	app := newWorkspaceApp(t)
	dir := t.TempDir()
	key := project.NewID()
	recipe := scaffoldCoordinateProject(t, dir, key, "Fernwell")
	tab, err := app.OpenProject(recipe)
	require.NoError(t, err)
	t.Cleanup(func() { app.CloseProject(tab.ID) })
	tabID := tab.ID
	engine := agentProcess(t, app)

	first := agentSuggest(t, engine, recipe, "sess-1", "Quick cast", "Quickcast", "docs/intro.md", "Try Quick cast today.")
	second := agentSuggest(t, engine, recipe, "sess-1", "business", "studio", "docs/billing.md", "Our business plan.")

	digest, err := app.ProjectContextDigest(tabID, "")
	require.NoError(t, err)
	assert.True(t, digest.Since.IsZero(), "nobody has looked yet")
	themes := map[string]string{}
	for _, theme := range digest.Suggested {
		for _, g := range theme.Groups {
			for _, it := range g.Items {
				themes[it.ID] = theme.Theme
				assert.True(t, it.New)
			}
		}
	}
	assert.Equal(t, host.DigestThemeNames, themes[first.ID])
	assert.Equal(t, host.DigestThemeWords, themes[second.ID])

	previous, err := app.MarkProjectContextDigestSeen(tabID)
	require.NoError(t, err)
	assert.Empty(t, previous)

	kept, err := app.KeepContextGroup(key, []string{first.ID, second.ID})
	require.NoError(t, err)
	assert.Equal(t, 2, kept)

	after, err := app.ProjectContextDigest(tabID, "")
	require.NoError(t, err)
	assert.False(t, after.Since.IsZero())
	assert.Empty(t, after.Suggested)
	require.Len(t, after.Established, 2)
	assert.True(t, after.Established[0].New, "kept after the look, so new")
	assert.Equal(t, []string{"kept by you"}, after.Established[0].How)
	assert.Equal(t, 2, after.Numbers.Rules)
}

// TestChoosingASideSettlesTheConflict: two agents disagree about one word,
// and choosing one drops the other and keeps the chosen rule.
func TestChoosingASideSettlesTheConflict(t *testing.T) {
	t.Setenv("KAPI_CONFIG_DIR", t.TempDir())
	app := newWorkspaceApp(t)
	recipe, key := openFeedProject(t, app, "Fernwell")
	engine := agentProcess(t, app)

	signIn := agentSuggest(t, engine, recipe, "sess-1", "login", "sign in", "docs/a.md", "Login here.")
	agentSuggest(t, engine, recipe, "sess-2", "login", "log in", "docs/b.md", "Login there.")

	entry, err := app.ChooseContextSide(ContextDecisionRequest{Project: key, ID: signIn.ID})
	require.NoError(t, err)
	assert.Equal(t, "keep", entry.Kind)

	feed, err := app.ContextFeed(key, 0)
	require.NoError(t, err)
	statuses := map[string]string{}
	for _, g := range feed.Groups {
		for _, e := range g.Entries {
			if e.Subject.Kind == "term" {
				statuses[e.Subject.Replacement] = e.Status
			}
		}
	}
	assert.Equal(t, "established", statuses["sign in"])
	assert.Equal(t, "dropped", statuses["log in"])
}
