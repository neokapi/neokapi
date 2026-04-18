package backend

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/project"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// snapshotForTest builds a .klz archive in `dst` from a scratch
// project with the given id. Returns the archive path.
func snapshotForTest(t *testing.T, dst, projectID string) string {
	t.Helper()
	src := t.TempDir()
	recipe := filepath.Join(src, projectID+".kapi")
	require.NoError(t, os.WriteFile(recipe, []byte(
		"version: v1\nid: "+projectID+"\nsourceLocale: en\n",
	), 0o644))

	layout, err := project.LayoutFor(recipe)
	require.NoError(t, err)
	require.NoError(t, project.EnsureLayout(layout))

	var buf bytes.Buffer
	require.NoError(t, project.Snapshot(layout, &buf, project.SnapshotOptions{}))

	archivePath := filepath.Join(dst, projectID+".klz")
	require.NoError(t, os.WriteFile(archivePath, buf.Bytes(), 0o644))
	return archivePath
}

func TestOpenArchive_extractsToSiblingDirectory(t *testing.T) {
	app := NewApp()

	dir := t.TempDir()
	archive := snapshotForTest(t, dir, "demo")

	recipe, err := app.OpenArchive(archive, "")
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(dir, "demo", "demo.kapi"), recipe)
	info, err := os.Stat(recipe)
	require.NoError(t, err)
	assert.False(t, info.IsDir())
}

func TestOpenArchive_refusesNonEmptyTarget(t *testing.T) {
	app := NewApp()
	dir := t.TempDir()
	archive := snapshotForTest(t, dir, "demo")

	// Pre-create demo/ with a file in it — emulates a user already
	// having extracted the archive once.
	existing := filepath.Join(dir, "demo")
	require.NoError(t, os.MkdirAll(existing, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(existing, "something.txt"), []byte("x"), 0o644))

	_, err := app.OpenArchive(archive, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not empty")
}

func TestOpenPath_recipeGoesToOpenProject(t *testing.T) {
	app := NewApp()
	dir := t.TempDir()

	// Build a recipe directly — OpenPath on a .kapi goes straight
	// to OpenProject, no archive plumbing.
	recipe := filepath.Join(dir, "direct.kapi")
	require.NoError(t, os.WriteFile(recipe, []byte(
		"version: v1\nname: direct\n",
	), 0o644))

	tab, err := app.OpenPath(recipe)
	require.NoError(t, err)
	require.NotNil(t, tab)
	assert.Equal(t, recipe, tab.Path)
	t.Cleanup(func() { app.CloseProject(tab.ID) })
}

func TestOpenPath_archiveIsExtractedThenOpened(t *testing.T) {
	app := NewApp()
	dir := t.TempDir()
	archive := snapshotForTest(t, dir, "demo")

	tab, err := app.OpenPath(archive)
	require.NoError(t, err)
	require.NotNil(t, tab)
	// Recipe was extracted next to the archive and then opened.
	expected := filepath.Join(dir, "demo", "demo.kapi")
	assert.Equal(t, expected, tab.Path)
	t.Cleanup(func() { app.CloseProject(tab.ID) })
}

func TestOpenPath_unknownExtensionIsRejected(t *testing.T) {
	app := NewApp()
	dir := t.TempDir()
	other := filepath.Join(dir, "notes.txt")
	require.NoError(t, os.WriteFile(other, []byte("hello"), 0o644))

	_, err := app.OpenPath(other)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported file extension")
}
