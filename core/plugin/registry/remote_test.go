package registry

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func makeTarGz(t *testing.T, files map[string][]byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)

	for name, content := range files {
		err := tw.WriteHeader(&tar.Header{
			Name: name,
			Size: int64(len(content)),
			Mode: 0o644,
		})
		require.NoError(t, err)
		_, err = tw.Write(content)
		require.NoError(t, err)
	}

	require.NoError(t, tw.Close())
	require.NoError(t, gw.Close())
	return buf.Bytes()
}

func checksum(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

// serveJSON writes v as JSON to the response writer (test helper).
func serveJSON(t *testing.T, w http.ResponseWriter, v any) {
	t.Helper()
	if err := json.NewEncoder(w).Encode(v); err != nil {
		t.Fatalf("encoding JSON response: %v", err)
	}
}

// serveBytes writes raw bytes to the response writer (test helper).
func serveBytes(t *testing.T, w http.ResponseWriter, data []byte) {
	t.Helper()
	if _, err := w.Write(data); err != nil {
		t.Fatalf("writing response: %v", err)
	}
}

func TestFetchIndex(t *testing.T) {
	index := RegistryIndex{
		Version: 1,
		Plugins: []PluginManifest{
			{Name: "test-plugin", Version: "1.0.0", PluginType: "tool"},
		},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		serveJSON(t, w, index)
	}))
	defer srv.Close()

	reg := NewRemoteRegistry(srv.URL, t.TempDir())
	got, err := reg.FetchIndex()
	require.NoError(t, err)
	assert.Equal(t, 1, got.Version)
	require.Len(t, got.Plugins, 1)
	assert.Equal(t, "test-plugin", got.Plugins[0].Name)
}

func TestInstallPluginBinary(t *testing.T) {
	binaryContent := []byte("fake-binary")

	index := RegistryIndex{
		Version: 1,
		Plugins: []PluginManifest{
			{
				Name:        "my-tool",
				Version:     "2.0.0",
				PluginType:  "tool",
				InstallType: "binary",
				Checksum:    checksum(binaryContent),
			},
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			serveJSON(t, w, index)
		case "/download/my-tool":
			serveBytes(t, w, binaryContent)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	index.Plugins[0].DownloadURL = srv.URL + "/download/my-tool"

	dir := t.TempDir()
	reg := NewRemoteRegistry(srv.URL, dir)

	result, err := reg.InstallPlugin(PluginRef{Name: "my-tool"})
	require.NoError(t, err)
	assert.Equal(t, "my-tool", result.Name)
	assert.Equal(t, "2.0.0", result.Version)
	assert.Equal(t, "binary", result.InstallType)
	require.Len(t, result.Files, 1)

	// Version file should be written in versioned directory.
	vf, err := ReadVersionFile(dir, "my-tool", "2.0.0")
	require.NoError(t, err)
	assert.Equal(t, "my-tool", vf.Name)
	assert.Equal(t, "2.0.0", vf.Version)
	assert.Equal(t, "binary", vf.InstallType)
}

func TestInstallPluginBridge(t *testing.T) {
	archive := makeTarGz(t, map[string][]byte{
		"gokapi-bridge-jar-with-dependencies.jar": []byte("fake-jar"),
		"okapi.bridge.json":                       []byte(`{"name":"okapi","type":"bridge"}`),
	})

	index := RegistryIndex{
		Version: 1,
		Plugins: []PluginManifest{
			{
				Name:        "okapi",
				Version:     "1.0.0",
				PluginType:  "bridge",
				InstallType: "bridge",
				Checksum:    checksum(archive),
			},
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			serveJSON(t, w, index)
		case "/download/bridge":
			serveBytes(t, w, archive)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	index.Plugins[0].DownloadURL = srv.URL + "/download/bridge"

	dir := t.TempDir()
	reg := NewRemoteRegistry(srv.URL, dir)

	result, err := reg.InstallPlugin(PluginRef{Name: "okapi"})
	require.NoError(t, err)
	assert.Equal(t, "okapi", result.Name)
	assert.Equal(t, "bridge", result.InstallType)
	assert.Len(t, result.Files, 2)

	// Verify extracted files exist in versioned directory.
	versionDir := VersionedPluginDir(dir, "okapi", "1.0.0")
	_, err = os.Stat(filepath.Join(versionDir, "gokapi-bridge-jar-with-dependencies.jar"))
	assert.NoError(t, err)
	_, err = os.Stat(filepath.Join(versionDir, "okapi.bridge.json"))
	assert.NoError(t, err)

	// Version file should be written.
	vf, err := ReadVersionFile(dir, "okapi", "1.0.0")
	require.NoError(t, err)
	assert.Equal(t, "bridge", vf.InstallType)
}

func TestInstallPluginExactVersion(t *testing.T) {
	binaryContent := []byte("fake-binary-v1")

	index := RegistryIndex{
		Version: 1,
		Plugins: []PluginManifest{
			{
				Name:        "my-tool",
				Version:     "1.0.0",
				PluginType:  "tool",
				InstallType: "binary",
				Checksum:    checksum(binaryContent),
			},
			{
				Name:        "my-tool",
				Version:     "2.0.0",
				PluginType:  "tool",
				InstallType: "binary",
				Checksum:    checksum([]byte("fake-binary-v2")),
			},
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			serveJSON(t, w, index)
		case "/download/my-tool-v1":
			serveBytes(t, w, binaryContent)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	index.Plugins[0].DownloadURL = srv.URL + "/download/my-tool-v1"
	index.Plugins[1].DownloadURL = srv.URL + "/download/my-tool-v2"

	dir := t.TempDir()
	reg := NewRemoteRegistry(srv.URL, dir)

	// Install specific version 1.0.0.
	result, err := reg.InstallPlugin(PluginRef{Name: "my-tool", Version: "1.0.0"})
	require.NoError(t, err)
	assert.Equal(t, "1.0.0", result.Version)

	// Version file should exist for 1.0.0.
	vf, err := ReadVersionFile(dir, "my-tool", "1.0.0")
	require.NoError(t, err)
	assert.Equal(t, "1.0.0", vf.Version)
}

func TestCheckUpdates(t *testing.T) {
	dir := t.TempDir()

	// Install an old version.
	require.NoError(t, WriteVersionFile(dir, "my-tool", "1.0.0", &VersionFile{
		Name:    "my-tool",
		Version: "1.0.0",
	}))

	index := RegistryIndex{
		Version: 1,
		Plugins: []PluginManifest{
			{Name: "my-tool", Version: "2.0.0"},
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		serveJSON(t, w, index)
	}))
	defer srv.Close()

	reg := NewRemoteRegistry(srv.URL, dir)

	updates, err := reg.CheckUpdates()
	require.NoError(t, err)
	require.Len(t, updates, 1)
	assert.Equal(t, "my-tool", updates[0].Name)
	assert.Equal(t, "1.0.0", updates[0].InstalledVersion)
	assert.Equal(t, "2.0.0", updates[0].AvailableVersion)
}

func TestCheckUpdatesNoUpdates(t *testing.T) {
	dir := t.TempDir()

	require.NoError(t, WriteVersionFile(dir, "my-tool", "1.0.0", &VersionFile{
		Name:    "my-tool",
		Version: "1.0.0",
	}))

	index := RegistryIndex{
		Version: 1,
		Plugins: []PluginManifest{
			{Name: "my-tool", Version: "1.0.0"},
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		serveJSON(t, w, index)
	}))
	defer srv.Close()

	reg := NewRemoteRegistry(srv.URL, dir)

	updates, err := reg.CheckUpdates()
	require.NoError(t, err)
	assert.Empty(t, updates)
}

func TestSearchPlugins(t *testing.T) {
	index := RegistryIndex{
		Version: 1,
		Plugins: []PluginManifest{
			{Name: "okapi", Description: "Okapi Framework bridge"},
			{Name: "csv-reader", Description: "CSV format reader"},
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		serveJSON(t, w, index)
	}))
	defer srv.Close()

	reg := NewRemoteRegistry(srv.URL, t.TempDir())

	results, err := reg.SearchPlugins("okapi")
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, "okapi", results[0].Name)

	// Search by description.
	results, err = reg.SearchPlugins("csv")
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, "csv-reader", results[0].Name)

	// Case-insensitive.
	results, err = reg.SearchPlugins("BRIDGE")
	require.NoError(t, err)
	require.Len(t, results, 1)
}

func TestListInstalled(t *testing.T) {
	dir := t.TempDir()

	require.NoError(t, WriteVersionFile(dir, "a", "1.0.0", &VersionFile{Name: "a", Version: "1.0.0"}))
	require.NoError(t, WriteVersionFile(dir, "b", "2.0.0", &VersionFile{Name: "b", Version: "2.0.0"}))

	reg := NewRemoteRegistry("http://unused", dir)
	installed, err := reg.ListInstalled()
	require.NoError(t, err)
	assert.Len(t, installed, 2)
}

func TestHasMimeType(t *testing.T) {
	m := PluginManifest{
		Capabilities: []Capability{
			{Type: "format", Name: "html", MimeTypes: []string{"text/html", "application/xhtml+xml"}},
			{Type: "format", Name: "xml", MimeTypes: []string{"application/xml"}},
		},
	}

	assert.True(t, m.HasMimeType("text/html"))
	assert.True(t, m.HasMimeType("TEXT/HTML")) // case-insensitive
	assert.True(t, m.HasMimeType("application/xml"))
	assert.False(t, m.HasMimeType("text/plain"))
}

func TestHasMimeTypeEmpty(t *testing.T) {
	m := PluginManifest{}
	assert.False(t, m.HasMimeType("text/html"))
}

func TestHasCapabilityType(t *testing.T) {
	m := PluginManifest{
		Capabilities: []Capability{
			{Type: "format", Name: "html"},
			{Type: "tool", Name: "word-count"},
		},
	}

	assert.True(t, m.HasCapabilityType("format"))
	assert.True(t, m.HasCapabilityType("FORMAT")) // case-insensitive
	assert.True(t, m.HasCapabilityType("tool"))
	assert.False(t, m.HasCapabilityType("bridge"))
}

func TestHasCapabilityTypeEmpty(t *testing.T) {
	m := PluginManifest{}
	assert.False(t, m.HasCapabilityType("format"))
}

func TestSearchPluginsAdvancedByMimeType(t *testing.T) {
	index := RegistryIndex{
		Version: 1,
		Plugins: []PluginManifest{
			{
				Name: "okapi",
				Capabilities: []Capability{
					{Type: "format", Name: "openxml", MimeTypes: []string{
						"application/vnd.openxmlformats-officedocument.wordprocessingml.document",
					}, Extensions: []string{".docx"}},
					{Type: "format", Name: "html", MimeTypes: []string{"text/html"}},
				},
			},
			{Name: "csv-reader", Description: "CSV format reader"},
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		serveJSON(t, w, index)
	}))
	defer srv.Close()

	reg := NewRemoteRegistry(srv.URL, t.TempDir())

	results, err := reg.SearchPluginsAdvanced(SearchOptions{MimeType: "text/html"})
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, "okapi", results[0].Name)
}

func TestSearchPluginsAdvancedByType(t *testing.T) {
	index := RegistryIndex{
		Version: 1,
		Plugins: []PluginManifest{
			{
				Name: "okapi",
				Capabilities: []Capability{
					{Type: "format", Name: "html"},
					{Type: "tool", Name: "segmentation"},
				},
			},
			{
				Name: "counter-tool",
				Capabilities: []Capability{
					{Type: "tool", Name: "word-count"},
				},
			},
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		serveJSON(t, w, index)
	}))
	defer srv.Close()

	reg := NewRemoteRegistry(srv.URL, t.TempDir())

	// Both plugins have "tool" capabilities.
	results, err := reg.SearchPluginsAdvanced(SearchOptions{Type: "tool"})
	require.NoError(t, err)
	assert.Len(t, results, 2)

	// Only okapi has "format" capabilities.
	results, err = reg.SearchPluginsAdvanced(SearchOptions{Type: "format"})
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, "okapi", results[0].Name)
}

func TestSearchPluginsAdvancedByExtension(t *testing.T) {
	index := RegistryIndex{
		Version: 1,
		Plugins: []PluginManifest{
			{
				Name: "okapi",
				Capabilities: []Capability{
					{Type: "format", Name: "openxml", Extensions: []string{".docx", ".xlsx", ".pptx"}},
				},
			},
			{Name: "csv-reader"},
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		serveJSON(t, w, index)
	}))
	defer srv.Close()

	reg := NewRemoteRegistry(srv.URL, t.TempDir())

	results, err := reg.SearchPluginsAdvanced(SearchOptions{Extension: ".docx"})
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, "okapi", results[0].Name)

	// No plugin handles .xyz.
	results, err = reg.SearchPluginsAdvanced(SearchOptions{Extension: ".xyz"})
	require.NoError(t, err)
	assert.Empty(t, results)
}

func TestSearchPluginsAdvancedCombinedFilters(t *testing.T) {
	index := RegistryIndex{
		Version: 1,
		Plugins: []PluginManifest{
			{
				Name: "okapi",
				Capabilities: []Capability{
					{Type: "format", Name: "openxml", MimeTypes: []string{
						"application/vnd.openxmlformats-officedocument.wordprocessingml.document",
					}, Extensions: []string{".docx"}},
					{Type: "format", Name: "html", MimeTypes: []string{"text/html"}, Extensions: []string{".html"}},
				},
			},
			{
				Name: "html-only",
				Capabilities: []Capability{
					{Type: "format", Name: "html", MimeTypes: []string{"text/html"}, Extensions: []string{".html"}},
				},
			},
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		serveJSON(t, w, index)
	}))
	defer srv.Close()

	reg := NewRemoteRegistry(srv.URL, t.TempDir())

	// MIME type + extension: only okapi has both text/html and .docx.
	results, err := reg.SearchPluginsAdvanced(SearchOptions{
		MimeType:  "text/html",
		Extension: ".docx",
	})
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, "okapi", results[0].Name)

	// Type + query: both have "format" capability; only "okapi" matches query.
	results, err = reg.SearchPluginsAdvanced(SearchOptions{
		Type:  "format",
		Query: "okapi",
	})
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, "okapi", results[0].Name)
}

func TestSearchPluginsAdvancedQueryMatchesCapabilities(t *testing.T) {
	index := RegistryIndex{
		Version: 1,
		Plugins: []PluginManifest{
			{
				Name:        "okapi",
				Description: "Okapi Framework bridge",
				Capabilities: []Capability{
					{Type: "format", Name: "openxml", DisplayName: "Microsoft Office (OpenXML)"},
					{Type: "format", Name: "html", DisplayName: "HTML"},
				},
			},
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		serveJSON(t, w, index)
	}))
	defer srv.Close()

	reg := NewRemoteRegistry(srv.URL, t.TempDir())

	// Match by capability name.
	results, err := reg.SearchPluginsAdvanced(SearchOptions{Query: "openxml"})
	require.NoError(t, err)
	require.Len(t, results, 1)

	// Match by capability display name.
	results, err = reg.SearchPluginsAdvanced(SearchOptions{Query: "Microsoft Office"})
	require.NoError(t, err)
	require.Len(t, results, 1)
}

func TestSearchPluginsAdvancedNoFilters(t *testing.T) {
	index := RegistryIndex{
		Version: 1,
		Plugins: []PluginManifest{
			{Name: "a"},
			{Name: "b"},
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		serveJSON(t, w, index)
	}))
	defer srv.Close()

	reg := NewRemoteRegistry(srv.URL, t.TempDir())

	// No filters: all plugins returned.
	results, err := reg.SearchPluginsAdvanced(SearchOptions{})
	require.NoError(t, err)
	assert.Len(t, results, 2)
}

func TestExtractTarGz(t *testing.T) {
	archive := makeTarGz(t, map[string][]byte{
		"subdir/file.txt": []byte("hello"),
		"root.txt":        []byte("world"),
	})

	dir := t.TempDir()
	files, err := extractTarGz(bytes.NewReader(archive), dir)
	require.NoError(t, err)
	assert.Len(t, files, 2)

	// Files should be extracted flat.
	data, err := os.ReadFile(filepath.Join(dir, "file.txt"))
	require.NoError(t, err)
	assert.Equal(t, "hello", string(data))

	data, err = os.ReadFile(filepath.Join(dir, "root.txt"))
	require.NoError(t, err)
	assert.Equal(t, "world", string(data))
}

func TestInstallPluginChecksumMismatch(t *testing.T) {
	archive := makeTarGz(t, map[string][]byte{
		"file.jar": []byte("jar-content"),
	})

	index := RegistryIndex{
		Version: 1,
		Plugins: []PluginManifest{
			{
				Name:        "bad-bridge",
				Version:     "1.0.0",
				InstallType: "bridge",
				Checksum:    "0000000000000000000000000000000000000000000000000000000000000000",
			},
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			serveJSON(t, w, index)
		case "/download":
			serveBytes(t, w, archive)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	index.Plugins[0].DownloadURL = srv.URL + "/download"

	reg := NewRemoteRegistry(srv.URL, t.TempDir())
	_, err := reg.InstallPlugin(PluginRef{Name: "bad-bridge"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "checksum mismatch")
}

func TestRemovePluginSpecificVersion(t *testing.T) {
	dir := t.TempDir()

	// Install two versions.
	require.NoError(t, WriteVersionFile(dir, "okapi", "1.0.0", &VersionFile{
		Name: "okapi", Version: "1.0.0",
	}))
	require.NoError(t, WriteVersionFile(dir, "okapi", "2.0.0", &VersionFile{
		Name: "okapi", Version: "2.0.0",
	}))

	reg := NewRemoteRegistry("http://unused", dir)

	// Remove only 1.0.0.
	err := reg.RemovePlugin(PluginRef{Name: "okapi", Version: "1.0.0"})
	require.NoError(t, err)

	// 1.0.0 should be gone.
	_, err = ReadVersionFile(dir, "okapi", "1.0.0")
	assert.Error(t, err)

	// 2.0.0 should still exist.
	vf, err := ReadVersionFile(dir, "okapi", "2.0.0")
	require.NoError(t, err)
	assert.Equal(t, "2.0.0", vf.Version)
}

func TestRemovePluginAllVersions(t *testing.T) {
	dir := t.TempDir()

	require.NoError(t, WriteVersionFile(dir, "okapi", "1.0.0", &VersionFile{
		Name: "okapi", Version: "1.0.0",
	}))
	require.NoError(t, WriteVersionFile(dir, "okapi", "2.0.0", &VersionFile{
		Name: "okapi", Version: "2.0.0",
	}))

	reg := NewRemoteRegistry("http://unused", dir)

	// Remove all versions.
	err := reg.RemovePlugin(PluginRef{Name: "okapi"})
	require.NoError(t, err)

	// Plugin directory should be gone.
	_, err = os.Stat(filepath.Join(dir, "okapi"))
	assert.True(t, os.IsNotExist(err))
}

func TestRemovePluginNotInstalled(t *testing.T) {
	dir := t.TempDir()
	reg := NewRemoteRegistry("http://unused", dir)

	err := reg.RemovePlugin(PluginRef{Name: "nonexistent"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not installed")
}

func TestListAvailableGrouped(t *testing.T) {
	index := RegistryIndex{
		Version: 1,
		Plugins: []PluginManifest{
			{Name: "okapi", Version: "1.47.0", Description: "Okapi bridge"},
			{Name: "okapi", Version: "1.46.0", Description: "Okapi bridge"},
			{Name: "deepl", Version: "1.0.0", Description: "DeepL connector"},
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		serveJSON(t, w, index)
	}))
	defer srv.Close()

	reg := NewRemoteRegistry(srv.URL, t.TempDir())

	groups, err := reg.ListAvailableGrouped()
	require.NoError(t, err)
	require.Len(t, groups, 2)

	// Sorted alphabetically: deepl first, then okapi.
	assert.Equal(t, "deepl", groups[0].Name)
	assert.Equal(t, "1.0.0", groups[0].Latest.Version)

	assert.Equal(t, "okapi", groups[1].Name)
	assert.Equal(t, "1.47.0", groups[1].Latest.Version)
	assert.Len(t, groups[1].Versions, 2)
}

func TestListAvailableGroupedEmpty(t *testing.T) {
	index := RegistryIndex{Version: 1}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		serveJSON(t, w, index)
	}))
	defer srv.Close()

	reg := NewRemoteRegistry(srv.URL, t.TempDir())

	groups, err := reg.ListAvailableGrouped()
	require.NoError(t, err)
	assert.Empty(t, groups)
}
