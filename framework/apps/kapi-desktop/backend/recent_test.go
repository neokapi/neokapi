package backend

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// createTempKapi creates a temp .kapi file and returns its path.
func createTempKapi(t *testing.T, name string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, name+".kapi")
	require.NoError(t, os.WriteFile(path, []byte("version: v1\nname: "+name), 0o644))
	return path
}

func TestRecentStoreEmpty(t *testing.T) {
	s := &recentStore{filePath: filepath.Join(t.TempDir(), "recent.json")}
	assert.Empty(t, s.list())
}

func TestRecentStoreAddAndList(t *testing.T) {
	s := &recentStore{filePath: filepath.Join(t.TempDir(), "recent.json")}
	path := createTempKapi(t, "project")

	s.add(path, "My Project")
	files := s.list()
	require.Len(t, files, 1)
	assert.Equal(t, path, files[0].Path)
	assert.Equal(t, "My Project", files[0].Name)
	assert.NotEmpty(t, files[0].OpenedAt)
}

func TestRecentStoreMostRecentFirst(t *testing.T) {
	s := &recentStore{filePath: filepath.Join(t.TempDir(), "recent.json")}
	p1 := createTempKapi(t, "first")
	p2 := createTempKapi(t, "second")

	s.add(p1, "First")
	s.add(p2, "Second")

	files := s.list()
	require.Len(t, files, 2)
	assert.Equal(t, p2, files[0].Path)
	assert.Equal(t, p1, files[1].Path)
}

func TestRecentStoreDeduplicates(t *testing.T) {
	s := &recentStore{filePath: filepath.Join(t.TempDir(), "recent.json")}
	p1 := createTempKapi(t, "project")
	p2 := createTempKapi(t, "other")

	s.add(p1, "V1")
	s.add(p2, "Other")
	s.add(p1, "V2")

	files := s.list()
	require.Len(t, files, 2)
	assert.Equal(t, p1, files[0].Path)
	assert.Equal(t, "V2", files[0].Name)
}

func TestRecentStoreFiltersDeleted(t *testing.T) {
	s := &recentStore{filePath: filepath.Join(t.TempDir(), "recent.json")}
	p1 := createTempKapi(t, "exists")
	p2 := createTempKapi(t, "deleted")

	s.add(p1, "Exists")
	s.add(p2, "Deleted")

	// Delete the second file.
	os.Remove(p2)

	files := s.list()
	require.Len(t, files, 1)
	assert.Equal(t, p1, files[0].Path)
}

func TestRecentStorePersistence(t *testing.T) {
	dir := t.TempDir()
	storePath := filepath.Join(dir, "sub", "recent.json")
	kapiPath := createTempKapi(t, "persist")

	s1 := &recentStore{filePath: storePath}
	s1.add(kapiPath, "Test")

	_, err := os.Stat(storePath)
	require.NoError(t, err)

	s2 := &recentStore{filePath: storePath}
	s2.load()
	files := s2.list()
	require.Len(t, files, 1)
	assert.Equal(t, kapiPath, files[0].Path)
}

func TestRecentStoreClear(t *testing.T) {
	s := &recentStore{filePath: filepath.Join(t.TempDir(), "recent.json")}
	p := createTempKapi(t, "clear")

	s.add(p, "Test")
	assert.Len(t, s.list(), 1)

	s.clear()
	assert.Empty(t, s.list())
}

func TestAppListRecentFiles(t *testing.T) {
	app := NewApp()
	_ = app.ListRecentFiles()
}

func TestAppClearRecentFiles(t *testing.T) {
	app := NewApp()
	app.ClearRecentFiles()
}
