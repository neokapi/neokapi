package loader

import (
	"log"
	"os"
	"path/filepath"
	"testing"

	"github.com/gokapi/gokapi/registry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewPluginLoader(t *testing.T) {
	l := NewPluginLoader("/some/dir", nil)
	assert.NotNil(t, l)
	assert.Equal(t, "/some/dir", l.Dir())
}

func TestLoadAllNonexistentDir(t *testing.T) {
	l := NewPluginLoader("/nonexistent/plugin/dir", nil)
	reg := registry.NewFormatRegistry()
	err := l.LoadAll(reg, nil)
	assert.NoError(t, err)
	assert.Empty(t, l.Plugins())
}

func TestLoadAllEmptyDir(t *testing.T) {
	dir := t.TempDir()
	l := NewPluginLoader(dir, nil)
	reg := registry.NewFormatRegistry()
	err := l.LoadAll(reg, nil)
	assert.NoError(t, err)
	assert.Empty(t, l.Plugins())
}

func TestLoadAllEmptyDirString(t *testing.T) {
	l := NewPluginLoader("", nil)
	reg := registry.NewFormatRegistry()
	err := l.LoadAll(reg, nil)
	assert.NoError(t, err)
	assert.Empty(t, l.Plugins())
}

func TestLoadAllInvalidDescriptorLogged(t *testing.T) {
	dir := t.TempDir()

	// Create a versioned directory structure with an invalid bridge descriptor.
	vDir := filepath.Join(dir, "bad-bridge", "1.0.0")
	require.NoError(t, os.MkdirAll(vDir, 0755))

	// Write version.json
	require.NoError(t, os.WriteFile(filepath.Join(vDir, "version.json"), []byte(`{
		"name": "bad-bridge",
		"version": "1.0.0",
		"install_type": "bridge"
	}`), 0644))

	// Write an invalid bridge descriptor (bad type).
	require.NoError(t, os.WriteFile(filepath.Join(vDir, "bad.bridge.json"), []byte(`{
		"name": "bad",
		"type": "not-bridge",
		"jar": "x.jar"
	}`), 0644))

	logger := log.New(os.Stderr, "[test] ", log.LstdFlags)
	l := NewPluginLoader(dir, logger)
	reg := registry.NewFormatRegistry()

	// Should not return error — invalid descriptors are logged and skipped.
	err := l.LoadAll(reg, nil)
	assert.NoError(t, err)
}

func TestPluginsReturnsCorrectInfo(t *testing.T) {
	l := NewPluginLoader("", nil)
	// Manually set plugins for testing.
	l.plugins = []PluginInfo{
		{Name: "test-plugin", Type: "binary", Source: "/path/to/plugin", Formats: []string{"csv"}},
		{Name: "okapi", Version: "1.46.0", Type: "bridge", Source: "/path/to/okapi.bridge.json", Formats: []string{"okapi-openxml@1.46.0", "okapi-html@1.46.0"}},
	}
	plugins := l.Plugins()
	assert.Len(t, plugins, 2)
	assert.Equal(t, "test-plugin", plugins[0].Name)
	assert.Equal(t, "binary", plugins[0].Type)
	assert.Equal(t, "okapi", plugins[1].Name)
	assert.Equal(t, "1.46.0", plugins[1].Version)
	assert.Equal(t, "bridge", plugins[1].Type)
	assert.Equal(t, []string{"okapi-openxml@1.46.0", "okapi-html@1.46.0"}, plugins[1].Formats)
}

func TestShutdownIdempotent(t *testing.T) {
	l := NewPluginLoader("", nil)
	// Shutdown with no manager/bridges should be safe.
	l.Shutdown()
	l.Shutdown()
}

func TestShutdownAfterLoad(t *testing.T) {
	dir := t.TempDir()
	l := NewPluginLoader(dir, nil)
	reg := registry.NewFormatRegistry()
	err := l.LoadAll(reg, nil)
	require.NoError(t, err)
	// Shutdown after a successful (empty) load.
	l.Shutdown()
	assert.Nil(t, l.pool)
	assert.Empty(t, l.bridges)
}

func TestSanitizeFilterName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"OpenXML", "openxml"},
		{"HTML Filter", "html-filter"},
		{"Plain Text", "plain-text"},
		{"csv", "csv"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			assert.Equal(t, tt.expected, sanitizeFilterName(tt.input))
		})
	}
}

func TestLoadAllNotADirectory(t *testing.T) {
	// Create a file (not a directory) and use it as the plugin dir.
	dir := t.TempDir()
	filePath := filepath.Join(dir, "notadir")
	err := os.WriteFile(filePath, []byte("x"), 0644)
	require.NoError(t, err)

	l := NewPluginLoader(filePath, nil)
	reg := registry.NewFormatRegistry()
	err = l.LoadAll(reg, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not a directory")
}
