package backend

import (
	"testing"

	"github.com/neokapi/neokapi/platform/connector"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestListConnectorTypes(t *testing.T) {
	app := NewAppWithoutPlugins()
	types := app.ListConnectorTypes()
	assert.True(t, len(types) > 0, "should have at least one connector type")

	// Should contain the file connector.
	found := false
	for _, ct := range types {
		if ct == "file" {
			found = true
		}
	}
	assert.True(t, found, "should contain 'file' connector type")
}

func TestConnectorLifecycle(t *testing.T) {
	app := NewAppWithoutPlugins()

	// Create a file connector.
	info, err := app.ConfigureConnector("file", map[string]string{
		"path":   t.TempDir(),
		"format": "json",
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
