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
