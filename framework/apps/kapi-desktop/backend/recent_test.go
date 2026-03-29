package backend

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRecentStoreEmpty(t *testing.T) {
	s := &recentStore{
		filePath: filepath.Join(t.TempDir(), "recent.json"),
	}
	assert.Empty(t, s.list())
}

func TestRecentStoreAddAndList(t *testing.T) {
	s := &recentStore{
		filePath: filepath.Join(t.TempDir(), "recent.json"),
	}

	s.add("/path/to/project.kapi", "My Project")
	files := s.list()
	require.Len(t, files, 1)
	assert.Equal(t, "/path/to/project.kapi", files[0].Path)
	assert.Equal(t, "My Project", files[0].Name)
	assert.NotEmpty(t, files[0].OpenedAt)
}

func TestRecentStoreMostRecentFirst(t *testing.T) {
	s := &recentStore{
		filePath: filepath.Join(t.TempDir(), "recent.json"),
	}

	s.add("/first.kapi", "First")
	s.add("/second.kapi", "Second")

	files := s.list()
	require.Len(t, files, 2)
	assert.Equal(t, "/second.kapi", files[0].Path) // most recent first
	assert.Equal(t, "/first.kapi", files[1].Path)
}

func TestRecentStoreDeduplicates(t *testing.T) {
	s := &recentStore{
		filePath: filepath.Join(t.TempDir(), "recent.json"),
	}

	s.add("/project.kapi", "V1")
	s.add("/other.kapi", "Other")
	s.add("/project.kapi", "V2") // reopening same file

	files := s.list()
	require.Len(t, files, 2)
	assert.Equal(t, "/project.kapi", files[0].Path)
	assert.Equal(t, "V2", files[0].Name) // updated name
}

func TestRecentStoreMaxLimit(t *testing.T) {
	s := &recentStore{
		filePath: filepath.Join(t.TempDir(), "recent.json"),
	}

	for i := 0; i < 15; i++ {
		s.add(filepath.Join("/tmp", "project"+string(rune('a'+i))+".kapi"), "Project")
	}

	assert.Len(t, s.list(), maxRecentFiles)
}

func TestRecentStorePersistence(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "recent.json")

	s1 := &recentStore{filePath: path}
	s1.add("/project.kapi", "Test")

	// Verify file was created.
	_, err := os.Stat(path)
	require.NoError(t, err)

	// Reload from disk.
	s2 := &recentStore{filePath: path}
	s2.load()
	files := s2.list()
	require.Len(t, files, 1)
	assert.Equal(t, "/project.kapi", files[0].Path)
}

func TestRecentStoreClear(t *testing.T) {
	s := &recentStore{
		filePath: filepath.Join(t.TempDir(), "recent.json"),
	}

	s.add("/project.kapi", "Test")
	assert.Len(t, s.list(), 1)

	s.clear()
	assert.Empty(t, s.list())
}

func TestAppListRecentFiles(t *testing.T) {
	app := NewApp()
	// May have entries from other runs; just verify it doesn't panic.
	_ = app.ListRecentFiles()
}

func TestAppClearRecentFiles(t *testing.T) {
	app := NewApp()
	app.ClearRecentFiles() // should not panic
}
