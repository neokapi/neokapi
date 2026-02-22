package registry

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFindVersions(t *testing.T) {
	idx := &RegistryIndex{
		Plugins: []PluginManifest{
			{Name: "okapi", Version: "1.45.0", Platform: "darwin/arm64"},
			{Name: "okapi", Version: "1.47.0", Platform: "darwin/arm64"},
			{Name: "okapi", Version: "1.46.0", Platform: "darwin/arm64"},
			{Name: "okapi", Version: "1.46.0", Platform: "linux/amd64"},
			{Name: "other", Version: "1.0.0", Platform: "darwin/arm64"},
		},
	}

	versions := idx.FindVersions("okapi", "darwin/arm64")
	require.Len(t, versions, 3)
	// Should be sorted descending.
	assert.Equal(t, "1.47.0", versions[0].Version)
	assert.Equal(t, "1.46.0", versions[1].Version)
	assert.Equal(t, "1.45.0", versions[2].Version)
}

func TestFindVersionsEmptyPlatform(t *testing.T) {
	idx := &RegistryIndex{
		Plugins: []PluginManifest{
			{Name: "okapi", Version: "1.0.0", Platform: ""},
		},
	}
	versions := idx.FindVersions("okapi", "darwin/arm64")
	assert.Len(t, versions, 1)
}

func TestFindVersionsNoMatch(t *testing.T) {
	idx := &RegistryIndex{
		Plugins: []PluginManifest{
			{Name: "okapi", Version: "1.0.0", Platform: "linux/amd64"},
		},
	}
	versions := idx.FindVersions("okapi", "darwin/arm64")
	assert.Empty(t, versions)
}

func TestFindLatest(t *testing.T) {
	idx := &RegistryIndex{
		Plugins: []PluginManifest{
			{Name: "okapi", Version: "1.45.0", Platform: "darwin/arm64"},
			{Name: "okapi", Version: "1.47.0", Platform: "darwin/arm64"},
			{Name: "okapi", Version: "1.46.0", Platform: "darwin/arm64"},
		},
	}

	m, err := idx.FindLatest("okapi", "darwin/arm64")
	require.NoError(t, err)
	assert.Equal(t, "1.47.0", m.Version)
}

func TestFindLatestNotFound(t *testing.T) {
	idx := &RegistryIndex{}
	_, err := idx.FindLatest("okapi", "darwin/arm64")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestFindExactVersion(t *testing.T) {
	idx := &RegistryIndex{
		Plugins: []PluginManifest{
			{Name: "okapi", Version: "1.46.0", Platform: "darwin/arm64"},
			{Name: "okapi", Version: "1.47.0", Platform: "darwin/arm64"},
		},
	}

	m, err := idx.FindExactVersion("okapi", "1.46.0", "darwin/arm64")
	require.NoError(t, err)
	assert.Equal(t, "1.46.0", m.Version)
}

func TestFindExactVersionNotFound(t *testing.T) {
	idx := &RegistryIndex{
		Plugins: []PluginManifest{
			{Name: "okapi", Version: "1.46.0", Platform: "darwin/arm64"},
		},
	}

	_, err := idx.FindExactVersion("okapi", "9.9.9", "darwin/arm64")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestGroupByName(t *testing.T) {
	idx := &RegistryIndex{
		Plugins: []PluginManifest{
			{Name: "okapi", Version: "1.45.0", Platform: "darwin/arm64"},
			{Name: "okapi", Version: "1.47.0", Platform: "darwin/arm64"},
			{Name: "okapi", Version: "1.46.0", Platform: "darwin/arm64"},
			{Name: "okapi", Version: "1.46.0", Platform: "linux/amd64"},
			{Name: "deepl", Version: "1.0.0", Platform: "darwin/arm64"},
		},
	}

	groups := idx.GroupByName("darwin/arm64")
	require.Len(t, groups, 2)

	// Groups should be sorted alphabetically.
	assert.Equal(t, "deepl", groups[0].Name)
	assert.Equal(t, "okapi", groups[1].Name)

	// deepl has one version.
	assert.Equal(t, "1.0.0", groups[0].Latest.Version)
	assert.Len(t, groups[0].Versions, 1)

	// okapi has three versions for darwin/arm64, sorted descending.
	assert.Equal(t, "1.47.0", groups[1].Latest.Version)
	require.Len(t, groups[1].Versions, 3)
	assert.Equal(t, "1.47.0", groups[1].Versions[0].Version)
	assert.Equal(t, "1.46.0", groups[1].Versions[1].Version)
	assert.Equal(t, "1.45.0", groups[1].Versions[2].Version)
}

func TestGroupByNameEmptyPlatformMatchesAll(t *testing.T) {
	idx := &RegistryIndex{
		Plugins: []PluginManifest{
			{Name: "okapi", Version: "1.0.0", Platform: ""},
		},
	}

	groups := idx.GroupByName("darwin/arm64")
	require.Len(t, groups, 1)
	assert.Equal(t, "okapi", groups[0].Name)
}

func TestGroupByNameNoMatch(t *testing.T) {
	idx := &RegistryIndex{
		Plugins: []PluginManifest{
			{Name: "okapi", Version: "1.0.0", Platform: "linux/amd64"},
		},
	}

	groups := idx.GroupByName("darwin/arm64")
	assert.Empty(t, groups)
}

func TestGroupByNameEmpty(t *testing.T) {
	idx := &RegistryIndex{}
	groups := idx.GroupByName("darwin/arm64")
	assert.Empty(t, groups)
}

func TestGroupByNameSingleVersion(t *testing.T) {
	idx := &RegistryIndex{
		Plugins: []PluginManifest{
			{Name: "okapi", Version: "1.47.0", Platform: "darwin/arm64"},
		},
	}

	groups := idx.GroupByName("darwin/arm64")
	require.Len(t, groups, 1)
	assert.Equal(t, "okapi", groups[0].Name)
	assert.Equal(t, "1.47.0", groups[0].Latest.Version)
	assert.Len(t, groups[0].Versions, 1)
}

func TestIsBundle(t *testing.T) {
	tests := []struct {
		name       string
		pluginType string
		want       bool
	}{
		{"bundle lowercase", "bundle", true},
		{"bundle uppercase", "Bundle", true},
		{"bundle mixed case", "BUNDLE", true},
		{"format is not bundle", "format", false},
		{"tool is not bundle", "tool", false},
		{"empty is not bundle", "", false},
		{"format-reader is not bundle", "format-reader", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &PluginManifest{PluginType: tt.pluginType}
			assert.Equal(t, tt.want, m.IsBundle())
		})
	}
}

func TestPluginTypeConstants(t *testing.T) {
	assert.Equal(t, "bundle", PluginTypeBundle)
	assert.Equal(t, "format", PluginTypeFormat)
	assert.Equal(t, "tool", PluginTypeTool)
}

func TestBundleWithMixedCapabilities(t *testing.T) {
	m := PluginManifest{
		Name:       "okapi",
		PluginType: PluginTypeBundle,
		Capabilities: []Capability{
			{Type: "format", Name: "html", MimeTypes: []string{"text/html"}},
			{Type: "format", Name: "openxml", MimeTypes: []string{"application/vnd.openxmlformats-officedocument.wordprocessingml.document"}},
			{Type: "tool", Name: "segmentation"},
		},
	}

	assert.True(t, m.IsBundle())
	assert.True(t, m.HasCapabilityType("format"))
	assert.True(t, m.HasCapabilityType("tool"))
	assert.True(t, m.HasMimeType("text/html"))
	assert.False(t, m.HasCapabilityType("connector"))
}

func TestGroupByNameIncludesBundles(t *testing.T) {
	idx := &RegistryIndex{
		Plugins: []PluginManifest{
			{Name: "okapi", Version: "1.47.0", Platform: "darwin/arm64", PluginType: PluginTypeBundle},
			{Name: "csv-format", Version: "1.0.0", Platform: "darwin/arm64", PluginType: PluginTypeFormat},
			{Name: "qa-tool", Version: "2.0.0", Platform: "darwin/arm64", PluginType: PluginTypeTool},
		},
	}

	groups := idx.GroupByName("darwin/arm64")
	require.Len(t, groups, 3)

	// Sorted alphabetically: csv-format, okapi, qa-tool.
	assert.Equal(t, "csv-format", groups[0].Name)
	assert.Equal(t, "okapi", groups[1].Name)
	assert.Equal(t, "qa-tool", groups[2].Name)
}
