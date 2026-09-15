package pluginhost

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/plugin/manifest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFormatProvider(t *testing.T) {
	installed := []*Plugin{
		{Manifest: &manifest.Manifest{Plugin: "acme", Capabilities: manifest.Capabilities{Formats: []manifest.Format{{Name: "acme_doc"}, {Name: "okf_custom"}}}}},
		{Manifest: &manifest.Manifest{Plugin: "gone", Capabilities: manifest.Capabilities{Formats: []manifest.Format{{Name: "gone_doc"}}}}, Retired: &Tombstone{Plugin: "gone"}},
	}

	for _, tc := range []struct {
		name      string
		installed []*Plugin
		format    string
		plugin    string
		ok        bool
	}{
		{name: "a bridge format", format: "okf_idml", plugin: "okapi-bridge", ok: true},
		{name: "another bridge format", format: "okf_openxml", plugin: "okapi-bridge", ok: true},
		{name: "the sourcecode format", format: "sourcecode", plugin: "sourcecode", ok: true},
		{name: "pdf", format: "pdf", plugin: "pdfium", ok: true},
		{name: "an installed manifest declares it", installed: installed, format: "acme_doc", plugin: "acme", ok: true},
		{name: "an installed manifest wins over the known table", installed: installed, format: "okf_custom", plugin: "acme", ok: true},
		{name: "a retired plugin supplies nothing", installed: installed, format: "gone_doc"},
		{name: "the bare bridge prefix is no format", format: "okf_"},
		{name: "an unknown format", format: "frob"},
		{name: "a built-in format no plugin supplies", format: "json"},
		{name: "no format", format: ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			plugin, ok := FormatProvider(tc.installed, tc.format)
			assert.Equal(t, tc.ok, ok)
			assert.Equal(t, tc.plugin, plugin)
		})
	}
}

// knownFormatPlugins lets kapi name the plugin to install with no plugin
// present. It lists exactly the formats the plugins in this repository declare,
// each with the plugin that declares it.
func TestKnownFormatPluginsMatchTheManifests(t *testing.T) {
	paths, err := filepath.Glob(filepath.Join("..", "..", "plugins", "*", "manifest.json"))
	require.NoError(t, err)
	require.NotEmpty(t, paths)

	declared := map[string]string{}
	for _, path := range paths {
		data, err := os.ReadFile(path)
		require.NoError(t, err)
		m, err := manifest.Parse(data)
		require.NoError(t, err, path)
		for _, f := range m.Capabilities.Formats {
			declared[f.Name] = m.Plugin
		}
	}
	assert.Equal(t, declared, knownFormatPlugins)
}
