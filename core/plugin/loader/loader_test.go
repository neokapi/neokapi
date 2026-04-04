package loader

import (
	"log"
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/registry"
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
	require.NoError(t, err)
	assert.Empty(t, l.Plugins())
}

func TestLoadAllEmptyDir(t *testing.T) {
	dir := t.TempDir()
	l := NewPluginLoader(dir, nil)
	reg := registry.NewFormatRegistry()
	err := l.LoadAll(reg, nil)
	require.NoError(t, err)
	assert.Empty(t, l.Plugins())
}

func TestLoadAllEmptyDirString(t *testing.T) {
	l := NewPluginLoader("", nil)
	reg := registry.NewFormatRegistry()
	err := l.LoadAll(reg, nil)
	require.NoError(t, err)
	assert.Empty(t, l.Plugins())
}

func TestLoadAllMissingManifestLogged(t *testing.T) {
	dir := t.TempDir()

	// Create a versioned directory structure without a manifest.json.
	vDir := filepath.Join(dir, "bad-bridge", "1.0.0")
	require.NoError(t, os.MkdirAll(vDir, 0755))

	// Write version.json
	require.NoError(t, os.WriteFile(filepath.Join(vDir, "version.json"), []byte(`{
		"name": "bad-bridge",
		"version": "1.0.0",
		"install_type": "bridge"
	}`), 0644))

	// No manifest.json — loader should log and skip.

	logger := log.New(os.Stderr, "[test] ", log.LstdFlags)
	l := NewPluginLoader(dir, logger)
	reg := registry.NewFormatRegistry()

	// Should not return error — missing manifest is logged and skipped.
	err := l.LoadAll(reg, nil)
	require.NoError(t, err)
}

func TestPluginsReturnsCorrectInfo(t *testing.T) {
	l := NewPluginLoader("", nil)
	// Manually set plugins for testing.
	l.plugins = []PluginInfo{
		{Name: "test-plugin", Type: "binary", Source: "/path/to/plugin", Formats: []string{"csv"}},
		{Name: "okapi", Version: "1.46.0", Type: "bridge", Source: "/path/to/okapi/1.46.0", Formats: []string{"okapi-openxml@1.46.0", "okapi-html@1.46.0"}},
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
	assert.Nil(t, l.registry)
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
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not a directory")
}

func TestScanMetadataEmptyDir(t *testing.T) {
	l := NewPluginLoader("", nil)
	err := l.ScanMetadata()
	require.NoError(t, err)
	assert.True(t, l.scanned)
	assert.Empty(t, l.Plugins())
}

func TestScanMetadataNonexistentDir(t *testing.T) {
	l := NewPluginLoader("/nonexistent/dir", nil)
	err := l.ScanMetadata()
	require.NoError(t, err)
	assert.True(t, l.scanned)
	assert.Empty(t, l.Plugins())
}

func TestScanMetadataNotADirectory(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "notadir")
	require.NoError(t, os.WriteFile(filePath, []byte("x"), 0644))

	l := NewPluginLoader(filePath, nil)
	err := l.ScanMetadata()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not a directory")
}

func TestScanMetadataBridgePlugin(t *testing.T) {
	dir := t.TempDir()

	// Create a bridge plugin with manifest and capabilities.
	vDir := filepath.Join(dir, "okapi", "1.46.0")
	require.NoError(t, os.MkdirAll(vDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(vDir, "version.json"), []byte(`{
		"name": "okapi",
		"version": "1.46.0",
		"install_type": "bridge"
	}`), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(vDir, "manifest.json"), []byte(`{
		"name": "okapi",
		"version": "1.46.0",
		"plugin_type": "bundle",
		"command": "java",
		"args": ["-jar", "bridge.jar"],
		"capabilities": [
			{"type": "format", "name": "HTML", "display_name": "HTML Filter", "mime_types": ["text/html"], "extensions": [".html", ".htm"]},
			{"type": "format", "name": "OpenXML", "display_name": "Microsoft Office", "mime_types": ["application/vnd.openxmlformats-officedocument.wordprocessingml.document"], "extensions": [".docx"]},
			{"type": "tool", "name": "segmenter"}
		]
	}`), 0644))

	reg := registry.NewFormatRegistry()
	l := NewPluginLoader(dir, nil)
	err := l.ScanMetadata(reg)
	require.NoError(t, err)

	// Should have plugin metadata without starting Java.
	plugins := l.Plugins()
	require.Len(t, plugins, 1)
	assert.Equal(t, "okapi", plugins[0].Name)
	assert.Equal(t, "1.46.0", plugins[0].Version)
	assert.Equal(t, "bridge", plugins[0].Type)
	// Format names derived from manifest capabilities (only "format" type).
	assert.Equal(t, []string{"okapi-html@1.46.0", "okapi-openxml@1.46.0"}, plugins[0].Formats)

	// Format info should be registered on the registry.
	htmlInfo := reg.FormatInfo("okapi-html@1.46.0")
	require.NotNil(t, htmlInfo, "format info should be registered for okapi-html@1.46.0")
	assert.Equal(t, "HTML Filter", htmlInfo.DisplayName)
	assert.Equal(t, []string{"text/html"}, htmlInfo.MimeTypes)
	assert.Equal(t, []string{".html", ".htm"}, htmlInfo.Extensions)
	assert.Equal(t, "okapi", htmlInfo.Source)

	docxInfo := reg.FormatInfo("okapi-openxml@1.46.0")
	require.NotNil(t, docxInfo, "format info should be registered for okapi-openxml@1.46.0")
	assert.Equal(t, "Microsoft Office", docxInfo.DisplayName)

	// No capabilities declared in manifest — read/write unknown.
	assert.False(t, htmlInfo.HasReader)
	assert.False(t, htmlInfo.HasWriter)

	// Bridge should NOT be started yet.
	assert.False(t, l.BridgesLoaded())
	assert.Nil(t, l.registry)
	assert.Len(t, l.pendingBridges, 1)
}

func TestScanMetadataWithSchemas(t *testing.T) {
	dir := t.TempDir()

	// Create a bridge plugin with manifest AND schema files.
	vDir := filepath.Join(dir, "okapi", "2.8.0")
	require.NoError(t, os.MkdirAll(vDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(vDir, "version.json"), []byte(`{
		"name": "okapi",
		"version": "2.8.0",
		"install_type": "bridge"
	}`), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(vDir, "manifest.json"), []byte(`{
		"name": "okapi",
		"version": "2.8.0",
		"plugin_type": "bundle",
		"command": "java",
		"args": ["-jar", "bridge.jar"],
		"capabilities": [
			{"type": "format", "id": "okf_html", "name": "html", "display_name": "HTML Filter", "capabilities": ["read", "write"], "mime_types": ["text/html"], "extensions": [".html"]},
			{"type": "format", "id": "okf_json", "name": "json", "display_name": "JSON Filter", "capabilities": ["read", "write"], "mime_types": ["application/json"], "extensions": [".json"]}
		]
	}`), 0644))

	// Create schema files with natural Okapi filter IDs.
	schemasDir := filepath.Join(vDir, "schemas")
	require.NoError(t, os.MkdirAll(schemasDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(schemasDir, "okf_html.schema.json"), []byte(`{
		"$id": "okf_html",
		"title": "HTML Filter",
		"type": "object",
		"formatMeta": {
			"id": "okf_html",
			"class": "net.sf.okapi.filters.html.HtmlFilter",
			"extensions": [".html", ".htm"],
			"mimeTypes": ["text/html"]
		},
		"properties": {}
	}`), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(schemasDir, "okf_json.schema.json"), []byte(`{
		"$id": "okf_json",
		"title": "JSON Filter",
		"type": "object",
		"formatMeta": {
			"id": "okf_json",
			"class": "net.sf.okapi.filters.json.JSONFilter",
			"extensions": [".json"],
			"mimeTypes": ["application/json"]
		},
		"properties": {}
	}`), 0644))

	reg := registry.NewFormatRegistry()
	l := NewPluginLoader(dir, nil)
	err := l.ScanMetadata(reg)
	require.NoError(t, err)

	plugins := l.Plugins()
	require.Len(t, plugins, 1)
	assert.Equal(t, "okapi", plugins[0].Name)
	// Format names should use schema IDs, not synthesized names.
	assert.Equal(t, []string{"okf_html@2.8.0", "okf_json@2.8.0"}, plugins[0].Formats)

	// Format info registered with schema-based names.
	htmlInfo := reg.FormatInfo("okf_html@2.8.0")
	require.NotNil(t, htmlInfo, "format info should be registered for okf_html@2.8.0")
	assert.Equal(t, "HTML Filter", htmlInfo.DisplayName)
	assert.Equal(t, []string{"text/html"}, htmlInfo.MimeTypes)
	assert.Equal(t, []string{".html", ".htm"}, htmlInfo.Extensions)
	assert.Equal(t, "okapi", htmlInfo.Source)
	assert.True(t, htmlInfo.HasReader, "read capability from manifest")
	assert.True(t, htmlInfo.HasWriter, "write capability from manifest")

	jsonInfo := reg.FormatInfo("okf_json@2.8.0")
	require.NotNil(t, jsonInfo, "format info should be registered for okf_json@2.8.0")
	assert.Equal(t, "JSON Filter", jsonInfo.DisplayName)
	assert.True(t, jsonInfo.HasReader)
	assert.True(t, jsonInfo.HasWriter)

	// Bare-name aliases should be registered (latest version).
	bareHTML := reg.FormatInfo("okf_html")
	require.NotNil(t, bareHTML, "bare name okf_html should have format info")
	assert.Equal(t, "HTML Filter", bareHTML.DisplayName)
	assert.Equal(t, "okapi", bareHTML.Source)

	bareJSON := reg.FormatInfo("okf_json")
	require.NotNil(t, bareJSON, "bare name okf_json should have format info")
	assert.Equal(t, "JSON Filter", bareJSON.DisplayName)
}

func TestScanMetadataBinaryPlugin(t *testing.T) {
	dir := t.TempDir()

	vDir := filepath.Join(dir, "my-plugin", "2.0.0")
	require.NoError(t, os.MkdirAll(vDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(vDir, "version.json"), []byte(`{
		"name": "my-plugin",
		"version": "2.0.0",
		"install_type": "binary"
	}`), 0644))

	l := NewPluginLoader(dir, nil)
	err := l.ScanMetadata()
	require.NoError(t, err)

	plugins := l.Plugins()
	require.Len(t, plugins, 1)
	assert.Equal(t, "my-plugin", plugins[0].Name)
	assert.Equal(t, "binary", plugins[0].Type)
	assert.Len(t, l.pendingBinaryDirs, 1)
}

func TestScanMetadataMissingManifest(t *testing.T) {
	dir := t.TempDir()

	vDir := filepath.Join(dir, "bad-bridge", "1.0.0")
	require.NoError(t, os.MkdirAll(vDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(vDir, "version.json"), []byte(`{
		"name": "bad-bridge",
		"version": "1.0.0",
		"install_type": "bridge"
	}`), 0644))

	logger := log.New(os.Stderr, "[test] ", log.LstdFlags)
	l := NewPluginLoader(dir, logger)
	err := l.ScanMetadata()
	require.NoError(t, err)

	// Missing manifest is skipped, not an error.
	assert.Empty(t, l.Plugins())
	assert.Empty(t, l.pendingBridges)
}

func TestLoadBridgesIdempotent(t *testing.T) {
	l := NewPluginLoader("", nil)
	require.NoError(t, l.ScanMetadata())

	reg := registry.NewFormatRegistry()
	require.NoError(t, l.LoadBridges(reg, nil))
	assert.True(t, l.BridgesLoaded())

	// Second call is a no-op.
	require.NoError(t, l.LoadBridges(reg, nil))
}

func TestLoadBridgesWithoutScanCallsScan(t *testing.T) {
	l := NewPluginLoader("", nil)
	assert.False(t, l.scanned)

	reg := registry.NewFormatRegistry()
	require.NoError(t, l.LoadBridges(reg, nil))
	assert.True(t, l.scanned)
	assert.True(t, l.BridgesLoaded())
}
