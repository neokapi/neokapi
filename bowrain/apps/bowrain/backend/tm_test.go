package backend

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAddMemoryEntry(t *testing.T) {
	app := newTestApp(t)
	info, err := app.CreateProject("content memory Test", "en", []string{"fr"})
	require.NoError(t, err)

	entry, err := app.AddMemoryEntry(info.ID, "Hello", "Bonjour", "en", "fr")
	require.NoError(t, err)
	assert.NotEmpty(t, entry.ID)
	assert.Equal(t, "Hello", entry.Source)
	assert.Equal(t, "Bonjour", entry.Target)
	assert.Equal(t, "en", entry.SourceLocale)
	assert.Equal(t, "fr", entry.TargetLocale)
}

func TestGetMemoryCount(t *testing.T) {
	app := newTestApp(t)
	info, err := app.CreateProject("content memory Test", "en", []string{"fr"})
	require.NoError(t, err)

	count, err := app.GetMemoryCount(info.ID)
	require.NoError(t, err)
	assert.Equal(t, 0, count)

	_, err = app.AddMemoryEntry(info.ID, "Hello", "Bonjour", "en", "fr")
	require.NoError(t, err)

	count, err = app.GetMemoryCount(info.ID)
	require.NoError(t, err)
	assert.Equal(t, 1, count)
}

func TestGetMemoryEntries(t *testing.T) {
	app := newTestApp(t)
	info, err := app.CreateProject("content memory Test", "en", []string{"fr", "de"})
	require.NoError(t, err)

	_, err = app.AddMemoryEntry(info.ID, "Hello", "Bonjour", "en", "fr")
	require.NoError(t, err)
	_, err = app.AddMemoryEntry(info.ID, "Goodbye", "Au revoir", "en", "fr")
	require.NoError(t, err)
	_, err = app.AddMemoryEntry(info.ID, "Hello", "Hallo", "en", "de")
	require.NoError(t, err)

	// No filter
	result, err := app.GetMemoryEntries(info.ID, "", "", "", 0, 50)
	require.NoError(t, err)
	assert.Equal(t, 3, result.TotalCount)
	assert.Len(t, result.Entries, 3)

	// Search by text
	result, err = app.GetMemoryEntries(info.ID, "hello", "", "", 0, 50)
	require.NoError(t, err)
	assert.Equal(t, 2, result.TotalCount)

	// Filter by target locale
	result, err = app.GetMemoryEntries(info.ID, "", "", "de", 0, 50)
	require.NoError(t, err)
	assert.Equal(t, 1, result.TotalCount)
	assert.Equal(t, "Hallo", result.Entries[0].Target)

	// Pagination
	result, err = app.GetMemoryEntries(info.ID, "", "", "", 0, 2)
	require.NoError(t, err)
	assert.Equal(t, 3, result.TotalCount)
	assert.Len(t, result.Entries, 2)
}

func TestUpdateMemoryEntry(t *testing.T) {
	app := newTestApp(t)
	info, err := app.CreateProject("content memory Test", "en", []string{"fr"})
	require.NoError(t, err)

	entry, err := app.AddMemoryEntry(info.ID, "Hello", "Bonjour", "en", "fr")
	require.NoError(t, err)

	err = app.UpdateMemoryEntry(MemoryUpdateRequest{
		ProjectID:    info.ID,
		EntryID:      entry.ID,
		Source:       "Hello",
		Target:       "Salut",
		SourceLocale: "en",
		TargetLocale: "fr",
	})
	require.NoError(t, err)

	// Verify update
	result, err := app.GetMemoryEntries(info.ID, "Hello", "", "", 0, 50)
	require.NoError(t, err)
	require.Len(t, result.Entries, 1)
	assert.Equal(t, "Salut", result.Entries[0].Target)
}

func TestUpdateMemoryEntry_NotFound(t *testing.T) {
	app := newTestApp(t)
	info, err := app.CreateProject("content memory Test", "en", []string{"fr"})
	require.NoError(t, err)

	err = app.UpdateMemoryEntry(MemoryUpdateRequest{
		ProjectID:    info.ID,
		EntryID:      "nonexistent",
		Source:       "Hello",
		Target:       "Salut",
		SourceLocale: "en",
		TargetLocale: "fr",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestDeleteMemoryEntry(t *testing.T) {
	app := newTestApp(t)
	info, err := app.CreateProject("content memory Test", "en", []string{"fr"})
	require.NoError(t, err)

	entry, err := app.AddMemoryEntry(info.ID, "Hello", "Bonjour", "en", "fr")
	require.NoError(t, err)

	err = app.DeleteMemoryEntry(info.ID, entry.ID)
	require.NoError(t, err)

	count, err := app.GetMemoryCount(info.ID)
	require.NoError(t, err)
	assert.Equal(t, 0, count)
}

func TestDeleteMemoryEntry_NotFound(t *testing.T) {
	app := newTestApp(t)
	info, err := app.CreateProject("content memory Test", "en", []string{"fr"})
	require.NoError(t, err)

	err = app.DeleteMemoryEntry(info.ID, "nonexistent")
	assert.Error(t, err)
}

func TestMemoryTranslateItem_UsesProjectMemory(t *testing.T) {
	app, info, itemName := setupProjectWithFile(t)

	// Add entries to the project content memory
	_, err := app.AddMemoryEntry(info.ID, "Hello, world!", "Bonjour le monde!", "en", "fr")
	require.NoError(t, err)

	// Now run content memory translate -- the project's content memory should be used
	stats, err := app.MemoryTranslateItem(info.ID, itemName, "fr")
	require.NoError(t, err)
	assert.Greater(t, stats.TotalBlocks, 0)
}

func TestCloseProject_ClosesMemory(t *testing.T) {
	app := newTestApp(t)
	info, err := app.CreateProject("content memory Test", "en", []string{"fr"})
	require.NoError(t, err)

	// Force content memory creation
	_, err = app.AddMemoryEntry(info.ID, "Hello", "Bonjour", "en", "fr")
	require.NoError(t, err)

	// Close should not error
	err = app.CloseProject(info.ID)
	require.NoError(t, err)

	// Project should no longer exist
	_, err = app.GetProject(info.ID)
	assert.Error(t, err)
}
