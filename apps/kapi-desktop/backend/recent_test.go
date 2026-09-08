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

// TestRecentStoreMarksUnavailable covers #2560: a project whose recipe is gone
// keeps its place in the list, marked with which shape of loss it is, so the
// user can see where it went and remove it themselves.
func TestRecentStoreMarksUnavailable(t *testing.T) {
	s := &recentStore{filePath: filepath.Join(t.TempDir(), "recent.json")}
	exists := createTempKapi(t, "exists")
	deleted := createTempKapi(t, "deleted")
	moved := createTempKapi(t, "moved")

	s.add(exists, "Exists")
	s.add(deleted, "Deleted")
	s.add(moved, "Moved")

	require.NoError(t, os.Remove(deleted))                // folder stays
	require.NoError(t, os.RemoveAll(filepath.Dir(moved))) // folder goes

	files := s.list()
	require.Len(t, files, 3)

	byPath := make(map[string]RecentFile, len(files))
	for _, f := range files {
		byPath[f.Path] = f
	}

	assert.True(t, byPath[exists].Available)
	assert.Empty(t, byPath[exists].Unavailable)

	assert.False(t, byPath[deleted].Available)
	assert.Equal(t, recentReasonDeleted, byPath[deleted].Unavailable)

	assert.False(t, byPath[moved].Available)
	assert.Equal(t, recentReasonMoved, byPath[moved].Unavailable)

	// Names survive, so an unavailable row still says which project it was.
	assert.Equal(t, "Moved", byPath[moved].Name)
}

func TestRecentStoreRemove(t *testing.T) {
	s := &recentStore{filePath: filepath.Join(t.TempDir(), "recent.json")}
	p1 := createTempKapi(t, "keep")
	p2 := createTempKapi(t, "drop")

	changes := 0
	s.onChange = func() { changes++ }

	s.add(p1, "Keep")
	s.add(p2, "Drop")
	changes = 0

	s.remove(p2)
	files := s.list()
	require.Len(t, files, 1)
	assert.Equal(t, p1, files[0].Path)
	assert.Equal(t, 1, changes, "removing an entry rebuilds the native menu")

	// Removal survives a reload: it is written through, not just dropped in memory.
	reloaded := &recentStore{filePath: s.filePath}
	reloaded.load()
	require.Len(t, reloaded.list(), 1)

	// Removing a path that is not in the list is a no-op, menu included.
	s.remove("/nowhere/kapi.yaml")
	assert.Len(t, s.list(), 1)
	assert.Equal(t, 1, changes)
}

// TestRecentStoreRemoveKeepsTheProjectOnDisk: the remove action forgets the
// project, it does not delete anything the user still has.
func TestRecentStoreRemoveKeepsTheProjectOnDisk(t *testing.T) {
	s := &recentStore{filePath: filepath.Join(t.TempDir(), "recent.json")}
	p := createTempKapi(t, "still-there")

	s.add(p, "Still There")
	s.remove(p)

	assert.Empty(t, s.list())
	_, err := os.Stat(p)
	require.NoError(t, err)
}

func TestRecipeStatus(t *testing.T) {
	present := createTempKapi(t, "present")
	deleted := createTempKapi(t, "deleted")
	moved := createTempKapi(t, "moved")
	require.NoError(t, os.Remove(deleted))
	require.NoError(t, os.RemoveAll(filepath.Dir(moved)))

	tests := []struct {
		name          string
		path          string
		wantAvailable bool
		wantReason    string
	}{
		{"present", present, true, ""},
		{"recipe removed from a folder that stays", deleted, false, recentReasonDeleted},
		{"folder gone", moved, false, recentReasonMoved},
		{"empty path", "", false, recentReasonMoved},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			available, reason := recipeStatus(tt.path)
			assert.Equal(t, tt.wantAvailable, available)
			assert.Equal(t, tt.wantReason, reason)
		})
	}
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

func TestAppRemoveRecentFile(t *testing.T) {
	app := &App{recent: &recentStore{filePath: filepath.Join(t.TempDir(), "recent.json")}}
	p := createTempKapi(t, "forget-me")
	app.recent.add(p, "Forget Me")
	require.Len(t, app.ListRecentFiles(), 1)

	app.RemoveRecentFile(p)
	assert.Empty(t, app.ListRecentFiles())
}

func TestAppClearRecentFiles(t *testing.T) {
	app := NewApp()
	app.ClearRecentFiles()
}
