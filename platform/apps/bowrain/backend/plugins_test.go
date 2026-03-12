package backend

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/neokapi/neokapi/core/plugin/registry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestRegistryServer(t *testing.T, index registry.RegistryIndex) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(index)
	}))
}

func TestSearchPlugins(t *testing.T) {
	index := registry.RegistryIndex{
		Version: 1,
		Plugins: []registry.PluginManifest{
			{Name: "okapi-bridge", Version: "1.0.0", Description: "Okapi bridge", PluginType: "bridge", InstallType: "bridge"},
			{Name: "csv-tool", Version: "1.0.0", Description: "CSV reader tool", PluginType: "tool"},
		},
	}
	srv := newTestRegistryServer(t, index)
	defer srv.Close()

	app := NewApp()
	// Override config to use test server.
	app.pluginSearchRegistry = srv.URL

	results, err := app.SearchPlugins("okapi")
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, "okapi-bridge", results[0].Name)
	assert.Equal(t, "bridge", results[0].InstallType)
}

func TestListAvailablePlugins(t *testing.T) {
	index := registry.RegistryIndex{
		Version: 1,
		Plugins: []registry.PluginManifest{
			{Name: "p1", Version: "1.0.0", PluginType: "tool"},
			{Name: "p2", Version: "2.0.0", PluginType: "bridge", InstallType: "bridge"},
		},
	}
	srv := newTestRegistryServer(t, index)
	defer srv.Close()

	app := NewApp()
	app.pluginSearchRegistry = srv.URL

	results, err := app.ListAvailablePlugins()
	require.NoError(t, err)
	assert.Len(t, results, 2)
}

func TestCheckPluginUpdates(t *testing.T) {
	index := registry.RegistryIndex{
		Version: 1,
		Plugins: []registry.PluginManifest{
			{Name: "my-plugin", Version: "2.0.0"},
		},
	}
	srv := newTestRegistryServer(t, index)
	defer srv.Close()

	app := NewApp()
	app.pluginSearchRegistry = srv.URL

	// Write a version file with an older version.
	dir := app.PluginDir()
	require.NoError(t, registry.WriteVersionFile(dir, "my-plugin", "1.0.0", &registry.VersionFile{
		Name:    "my-plugin",
		Version: "1.0.0",
	}))

	updates, err := app.CheckPluginUpdates()
	require.NoError(t, err)
	require.Len(t, updates, 1)
	assert.Equal(t, "my-plugin", updates[0].Name)
	assert.Equal(t, "1.0.0", updates[0].InstalledVersion)
	assert.Equal(t, "2.0.0", updates[0].AvailableVersion)
}

func TestManifestsToSearchResults(t *testing.T) {
	manifests := []registry.PluginManifest{
		{Name: "a", Version: "1.0.0", InstallType: "bridge"},
		{Name: "b", Version: "2.0.0"}, // empty install_type defaults to "binary"
	}

	results := manifestsToSearchResults(manifests)
	require.Len(t, results, 2)
	assert.Equal(t, "bridge", results[0].InstallType)
	assert.Equal(t, "binary", results[1].InstallType)
}

func TestManifestsToSearchResultsWithCapabilities(t *testing.T) {
	manifests := []registry.PluginManifest{
		{
			Name:    "okapi-bridge",
			Version: "1.0.0",
			Capabilities: []registry.Capability{
				{
					Type:        "format",
					Name:        "openxml",
					DisplayName: "Microsoft Office (OpenXML)",
					MimeTypes:   []string{"application/vnd.openxmlformats-officedocument.wordprocessingml.document"},
					Extensions:  []string{".docx", ".xlsx"},
				},
				{
					Type: "format",
					Name: "html",
				},
			},
		},
	}

	results := manifestsToSearchResults(manifests)
	require.Len(t, results, 1)
	require.Len(t, results[0].Capabilities, 2)
	assert.Equal(t, "format", results[0].Capabilities[0].Type)
	assert.Equal(t, "openxml", results[0].Capabilities[0].Name)
	assert.Equal(t, "Microsoft Office (OpenXML)", results[0].Capabilities[0].DisplayName)
	assert.Equal(t, []string{".docx", ".xlsx"}, results[0].Capabilities[0].Extensions)
	assert.Equal(t, "html", results[0].Capabilities[1].Name)
}

func TestSearchPluginsByMimeType(t *testing.T) {
	index := registry.RegistryIndex{
		Version: 1,
		Plugins: []registry.PluginManifest{
			{
				Name: "okapi-bridge",
				Capabilities: []registry.Capability{
					{Type: "format", Name: "html", MimeTypes: []string{"text/html"}},
				},
			},
			{Name: "csv-tool", Description: "CSV reader tool"},
		},
	}
	srv := newTestRegistryServer(t, index)
	defer srv.Close()

	app := NewApp()
	app.pluginSearchRegistry = srv.URL

	results, err := app.SearchPluginsByMimeType("text/html")
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, "okapi-bridge", results[0].Name)

	results, err = app.SearchPluginsByMimeType("text/plain")
	require.NoError(t, err)
	assert.Empty(t, results)
}

func TestSearchPluginsByType(t *testing.T) {
	index := registry.RegistryIndex{
		Version: 1,
		Plugins: []registry.PluginManifest{
			{
				Name: "okapi-bridge",
				Capabilities: []registry.Capability{
					{Type: "format", Name: "html"},
					{Type: "tool", Name: "segmentation"},
				},
			},
			{
				Name: "counter",
				Capabilities: []registry.Capability{
					{Type: "tool", Name: "word-count"},
				},
			},
		},
	}
	srv := newTestRegistryServer(t, index)
	defer srv.Close()

	app := NewApp()
	app.pluginSearchRegistry = srv.URL

	results, err := app.SearchPluginsByType("format")
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, "okapi-bridge", results[0].Name)

	results, err = app.SearchPluginsByType("tool")
	require.NoError(t, err)
	assert.Len(t, results, 2)
}
