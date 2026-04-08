package project

import (
	"testing"

	"github.com/neokapi/neokapi/core/registry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMatchVersionConstraint(t *testing.T) {
	tests := []struct {
		name       string
		constraint string
		installed  string
		want       bool
	}{
		{"empty constraint", "", "1.0.0", true},
		{"wildcard", "*", "1.0.0", true},
		{"exact match", "1.2.3", "1.2.3", true},
		{"exact mismatch", "1.2.3", "1.2.4", false},
		{"exact with v prefix", "1.2.3", "v1.2.3", true},
		{"compatible same", "^1.2.3", "1.2.3", true},
		{"compatible higher minor", "^1.2.3", "1.3.0", true},
		{"compatible higher patch", "^1.2.3", "1.2.5", true},
		{"compatible lower", "^1.2.3", "1.2.0", false},
		{"compatible major mismatch", "^1.2.3", "2.0.0", false},
		{"gte equal", ">=1.2.3", "1.2.3", true},
		{"gte higher", ">=1.2.3", "2.0.0", true},
		{"gte lower", ">=1.2.3", "1.2.2", false},
		{"pre-release stripped", "1.47.0", "1.47.0-rc1", true},
		{"build metadata stripped", "1.47.0", "1.47.0+build42", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MatchVersionConstraint(tt.constraint, tt.installed)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestCheckPlugins_NoPlugins(t *testing.T) {
	proj := &KapiProject{Version: CurrentVersion}
	status := CheckPlugins(proj, nil)
	require.True(t, status.Satisfied)
	assert.Empty(t, status.Issues)
}

func TestCheckPlugins_AllSatisfied(t *testing.T) {
	proj := &KapiProject{
		Version: CurrentVersion,
		Plugins: map[string]PluginSpec{
			"okapi": {FrameworkVersion: "^1.47.0"},
		},
	}
	installed := []InstalledPlugin{
		{Name: "okapi", Version: "0.40.0", FrameworkVersion: "1.48.0"},
	}
	status := CheckPlugins(proj, installed)
	require.True(t, status.Satisfied)
	assert.Empty(t, status.Issues)
}

func TestCheckPlugins_Missing(t *testing.T) {
	proj := &KapiProject{
		Version: CurrentVersion,
		Plugins: map[string]PluginSpec{
			"okapi": {},
		},
	}
	status := CheckPlugins(proj, nil)
	require.False(t, status.Satisfied)
	require.Len(t, status.Issues, 1)
	assert.Equal(t, "okapi", status.Issues[0].Plugin)
	assert.Equal(t, "missing", status.Issues[0].Type)
}

func TestCheckPlugins_VersionMismatch(t *testing.T) {
	proj := &KapiProject{
		Version: CurrentVersion,
		Plugins: map[string]PluginSpec{
			"okapi": {FrameworkVersion: "1.47.0"}, // exact
		},
	}
	installed := []InstalledPlugin{
		{Name: "okapi", Version: "0.40.0", FrameworkVersion: "1.48.0"},
	}
	status := CheckPlugins(proj, installed)
	require.False(t, status.Satisfied)
	require.Len(t, status.Issues, 1)
	assert.Equal(t, "version_mismatch", status.Issues[0].Type)
	assert.Equal(t, "1.47.0", status.Issues[0].Required)
	assert.Equal(t, "1.48.0", status.Issues[0].InstalledVersion)
}

func TestCheckPlugins_HighestVersionUsed(t *testing.T) {
	proj := &KapiProject{
		Version: CurrentVersion,
		Plugins: map[string]PluginSpec{
			"okapi": {FrameworkVersion: "^1.47.0"},
		},
	}
	installed := []InstalledPlugin{
		{Name: "okapi", Version: "0.38.0", FrameworkVersion: "1.44.0"},
		{Name: "okapi", Version: "0.40.0", FrameworkVersion: "1.48.0"},
	}
	status := CheckPlugins(proj, installed)
	require.True(t, status.Satisfied)
}

func TestPopulatePlugins(t *testing.T) {
	proj := &KapiProject{Version: CurrentVersion}
	installed := []InstalledPlugin{
		{Name: "okapi", Version: "0.40.0"},
		{Name: "my-tool", Version: "1.0.0"},
	}
	PopulatePlugins(proj, installed)
	require.Len(t, proj.Plugins, 2)
	assert.Contains(t, proj.Plugins, "okapi")
	assert.Contains(t, proj.Plugins, "my-tool")
	// Should be empty specs (no constraints).
	assert.Equal(t, PluginSpec{}, proj.Plugins["okapi"])
}

func TestAllowedFormatSources_NoPlugins(t *testing.T) {
	proj := &KapiProject{Version: CurrentVersion}
	sources := AllowedFormatSources(proj)
	assert.Equal(t, []string{registry.SourceBuiltIn}, sources)
}

func TestAllowedFormatSources_WithPlugins(t *testing.T) {
	proj := &KapiProject{
		Version: CurrentVersion,
		Plugins: map[string]PluginSpec{
			"okapi-bridge": {},
		},
	}
	sources := AllowedFormatSources(proj)
	assert.Contains(t, sources, registry.SourceBuiltIn)
	assert.Contains(t, sources, "okapi-bridge")
	assert.Len(t, sources, 2)
}

func TestPopulatePlugins_DoesNotOverwrite(t *testing.T) {
	proj := &KapiProject{
		Version: CurrentVersion,
		Plugins: map[string]PluginSpec{
			"okapi": {FrameworkVersion: "^1.47.0"},
		},
	}
	installed := []InstalledPlugin{
		{Name: "okapi", Version: "0.40.0"},
		{Name: "new-plugin", Version: "1.0.0"},
	}
	PopulatePlugins(proj, installed)
	require.Len(t, proj.Plugins, 2)
	// Existing spec should be preserved.
	assert.Equal(t, "^1.47.0", proj.Plugins["okapi"].FrameworkVersion)
	// New plugin should be added with empty spec.
	assert.Equal(t, PluginSpec{}, proj.Plugins["new-plugin"])
}
