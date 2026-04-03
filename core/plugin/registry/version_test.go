package registry

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWriteAndReadVersionFile(t *testing.T) {
	dir := t.TempDir()

	vf := &VersionFile{
		Name:        "okapi",
		Version:     "1.0.0",
		InstallType: "bridge",
		PluginType:  "bundle",
		Capabilities: []Capability{
			{Type: "format", Name: "html"},
			{Type: "format", Name: "openxml"},
			{Type: "tool", Name: "segmentation"},
		},
		InstalledAt: "2025-01-15T10:00:00Z",
		Checksum:    "abc123",
	}

	err := WriteVersionFile(dir, "okapi", "1.0.0", vf)
	require.NoError(t, err)

	// File should exist on disk at {dir}/okapi/1.0.0/version.json.
	_, err = os.Stat(filepath.Join(dir, "okapi", "1.0.0", "version.json"))
	require.NoError(t, err)

	got, err := ReadVersionFile(dir, "okapi", "1.0.0")
	require.NoError(t, err)
	assert.Equal(t, vf.Name, got.Name)
	assert.Equal(t, vf.Version, got.Version)
	assert.Equal(t, vf.InstallType, got.InstallType)
	assert.Equal(t, vf.PluginType, got.PluginType)
	assert.Len(t, got.Capabilities, 3)
	assert.Equal(t, 2, got.FormatCount())
	assert.Equal(t, vf.InstalledAt, got.InstalledAt)
	assert.Equal(t, vf.Checksum, got.Checksum)
}

func TestReadVersionFileBackwardsCompatible(t *testing.T) {
	// Old version.json files without plugin_type/capabilities should still parse.
	dir := t.TempDir()
	vf := &VersionFile{
		Name:        "old-plugin",
		Version:     "1.0.0",
		InstallType: "binary",
	}
	require.NoError(t, WriteVersionFile(dir, "old-plugin", "1.0.0", vf))

	got, err := ReadVersionFile(dir, "old-plugin", "1.0.0")
	require.NoError(t, err)
	assert.Equal(t, "", got.PluginType)
	assert.Empty(t, got.Capabilities)
	assert.Equal(t, 0, got.FormatCount())
}

func TestReadVersionFileNotFound(t *testing.T) {
	dir := t.TempDir()

	_, err := ReadVersionFile(dir, "nonexistent", "1.0.0")
	assert.Error(t, err)
}

func TestListInstalledVersions(t *testing.T) {
	dir := t.TempDir()

	// Write two versions of plugin-a.
	require.NoError(t, WriteVersionFile(dir, "plugin-a", "1.0.0", &VersionFile{
		Name:        "plugin-a",
		Version:     "1.0.0",
		InstallType: "binary",
	}))
	require.NoError(t, WriteVersionFile(dir, "plugin-a", "2.0.0", &VersionFile{
		Name:        "plugin-a",
		Version:     "2.0.0",
		InstallType: "binary",
	}))

	versions, err := ListInstalledVersions(dir, "plugin-a")
	require.NoError(t, err)
	assert.Len(t, versions, 2)

	versionSet := map[string]bool{}
	for _, v := range versions {
		versionSet[v.Version] = true
		assert.NotEmpty(t, v.Dir)
	}
	assert.True(t, versionSet["1.0.0"])
	assert.True(t, versionSet["2.0.0"])
}

func TestListInstalledVersionsNonexistent(t *testing.T) {
	dir := t.TempDir()

	versions, err := ListInstalledVersions(dir, "nonexistent")
	require.NoError(t, err)
	assert.Empty(t, versions)
}

func TestListAllInstalled(t *testing.T) {
	dir := t.TempDir()

	require.NoError(t, WriteVersionFile(dir, "plugin-a", "1.0.0", &VersionFile{
		Name:        "plugin-a",
		Version:     "1.0.0",
		InstallType: "binary",
	}))
	require.NoError(t, WriteVersionFile(dir, "plugin-b", "2.0.0", &VersionFile{
		Name:        "plugin-b",
		Version:     "2.0.0",
		InstallType: "bridge",
	}))

	// Add a non-version file that should be ignored.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "other.json"), []byte("{}"), 0o644))

	all, err := ListAllInstalled(dir)
	require.NoError(t, err)
	assert.Len(t, all, 2)
	assert.Len(t, all["plugin-a"], 1)
	assert.Len(t, all["plugin-b"], 1)
}

func TestListAllInstalledEmptyDir(t *testing.T) {
	dir := t.TempDir()

	all, err := ListAllInstalled(dir)
	require.NoError(t, err)
	assert.Empty(t, all)
}

func TestListAllInstalledNonexistentDir(t *testing.T) {
	all, err := ListAllInstalled("/tmp/nonexistent-plugin-dir-test")
	require.NoError(t, err)
	assert.Nil(t, all)
}

func TestLatestInstalledVersion(t *testing.T) {
	dir := t.TempDir()

	require.NoError(t, WriteVersionFile(dir, "okapi", "1.45.0", &VersionFile{
		Name: "okapi", Version: "1.45.0",
	}))
	require.NoError(t, WriteVersionFile(dir, "okapi", "1.47.0", &VersionFile{
		Name: "okapi", Version: "1.47.0",
	}))
	require.NoError(t, WriteVersionFile(dir, "okapi", "1.46.0", &VersionFile{
		Name: "okapi", Version: "1.46.0",
	}))

	latest, err := LatestInstalledVersion(dir, "okapi")
	require.NoError(t, err)
	assert.Equal(t, "1.47.0", latest.Version)
}

func TestLatestInstalledVersionNotFound(t *testing.T) {
	dir := t.TempDir()

	_, err := LatestInstalledVersion(dir, "nonexistent")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no installed versions")
}

func TestVersionedPluginDir(t *testing.T) {
	dir := VersionedPluginDir("/base", "okapi", "1.46.0")
	assert.Equal(t, filepath.Join("/base", "okapi", "1.46.0"), dir)
}

func TestWriteVersionFileCreatesDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "path")

	err := WriteVersionFile(dir, "test", "1.0.0", &VersionFile{Name: "test", Version: "1.0.0"})
	require.NoError(t, err)

	got, err := ReadVersionFile(dir, "test", "1.0.0")
	require.NoError(t, err)
	assert.Equal(t, "test", got.Name)
}
