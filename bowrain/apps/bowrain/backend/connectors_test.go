package backend

import (
	"testing"

	"github.com/neokapi/neokapi/bowrain/core/connector"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestListConnectorTypes(t *testing.T) {
	app := NewAppWithoutPlugins()
	types := app.ListConnectorTypes()
	assert.True(t, len(types) > 0, "should have at least one connector type")

	// The desktop offers remote/CMS connectors only.
	assert.Contains(t, types, "wordpress", "should offer the wordpress connector")

	// The desktop must NOT offer the local-filesystem connectors: kapi owns
	// local files + project configuration, and the desktop's local footprint
	// is a working copy / cache of the server, never a source of truth.
	assert.NotContains(t, types, "file", "desktop must not offer the local 'file' connector")
	assert.NotContains(t, types, "git", "desktop must not offer the local 'git' connector")
}

func TestConnectorLifecycle(t *testing.T) {
	app := NewAppWithoutPlugins()

	// Create a remote connector. (Local file/git connectors are not offered by
	// the desktop — see TestListConnectorTypes.)
	info, err := app.ConfigureConnector("wordpress", map[string]string{
		"url":      "https://example.com",
		"username": "user",
		"password": "pass",
	})
	require.NoError(t, err)
	assert.NotEmpty(t, info.ID)

	// List active connectors.
	active := app.ListConnectors()
	assert.Len(t, active, 1)
	assert.Equal(t, info.ID, active[0].ID)

	// Remove connector.
	require.NoError(t, app.RemoveConnector(info.ID))
	active = app.ListConnectors()
	assert.Len(t, active, 0)

	// Cleanup: reset global state for other tests.
	activeConnectors = map[string]connector.IntegrationConnector{}
}

func TestInitContentStore(t *testing.T) {
	app := NewAppWithoutPlugins()
	require.NoError(t, app.InitContentStore(":memory:"))

	// Create a project.
	proj, err := app.StoreProject("Test", "en", []string{"fr"})
	require.NoError(t, err)
	assert.NotEmpty(t, proj.ID)

	// List projects.
	projects, err := app.ListStoreProjects()
	require.NoError(t, err)
	assert.Len(t, projects, 1)

	// Version.
	v, err := app.CreateStoreVersion(proj.ID, "v1", "test version")
	require.NoError(t, err)
	assert.Equal(t, "v1", v.Label)

	versions, err := app.ListStoreVersions(proj.ID)
	require.NoError(t, err)
	assert.Len(t, versions, 1)
}
