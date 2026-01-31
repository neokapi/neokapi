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
		Name:        "okapi-bridge",
		Version:     "1.0.0",
		InstallType: "bridge",
		InstalledAt: "2025-01-15T10:00:00Z",
		Checksum:    "abc123",
	}

	err := WriteVersionFile(dir, "okapi-bridge", vf)
	require.NoError(t, err)

	// File should exist on disk.
	_, err = os.Stat(filepath.Join(dir, "okapi-bridge.version.json"))
	require.NoError(t, err)

	got, err := ReadVersionFile(dir, "okapi-bridge")
	require.NoError(t, err)
	assert.Equal(t, vf.Name, got.Name)
	assert.Equal(t, vf.Version, got.Version)
	assert.Equal(t, vf.InstallType, got.InstallType)
	assert.Equal(t, vf.InstalledAt, got.InstalledAt)
	assert.Equal(t, vf.Checksum, got.Checksum)
}

func TestReadVersionFileNotFound(t *testing.T) {
	dir := t.TempDir()

	_, err := ReadVersionFile(dir, "nonexistent")
	assert.Error(t, err)
}

func TestListVersionFiles(t *testing.T) {
	dir := t.TempDir()

	// Write two version files.
	require.NoError(t, WriteVersionFile(dir, "plugin-a", &VersionFile{
		Name:        "plugin-a",
		Version:     "1.0.0",
		InstallType: "binary",
	}))
	require.NoError(t, WriteVersionFile(dir, "plugin-b", &VersionFile{
		Name:        "plugin-b",
		Version:     "2.0.0",
		InstallType: "bridge",
	}))

	// Add a non-version file that should be ignored.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "other.json"), []byte("{}"), 0o644))

	files, err := ListVersionFiles(dir)
	require.NoError(t, err)
	assert.Len(t, files, 2)

	names := map[string]bool{}
	for _, f := range files {
		names[f.Name] = true
	}
	assert.True(t, names["plugin-a"])
	assert.True(t, names["plugin-b"])
}

func TestListVersionFilesEmptyDir(t *testing.T) {
	dir := t.TempDir()

	files, err := ListVersionFiles(dir)
	require.NoError(t, err)
	assert.Empty(t, files)
}

func TestListVersionFilesNonexistentDir(t *testing.T) {
	files, err := ListVersionFiles("/tmp/nonexistent-plugin-dir-test")
	require.NoError(t, err)
	assert.Nil(t, files)
}

func TestWriteVersionFileCreatesDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "path")

	err := WriteVersionFile(dir, "test", &VersionFile{Name: "test", Version: "1.0.0"})
	require.NoError(t, err)

	got, err := ReadVersionFile(dir, "test")
	require.NoError(t, err)
	assert.Equal(t, "test", got.Name)
}
