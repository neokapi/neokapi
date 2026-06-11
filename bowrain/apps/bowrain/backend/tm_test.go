package backend

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAddTMEntry(t *testing.T) {
	app := newTestApp(t)
	info, err := app.CreateProject("TM Test", "en", []string{"fr"})
	require.NoError(t, err)

	entry, err := app.AddTMEntry(info.ID, "Hello", "Bonjour", "en", "fr")
	require.NoError(t, err)
	assert.NotEmpty(t, entry.ID)
	assert.Equal(t, "Hello", entry.Source)
	assert.Equal(t, "Bonjour", entry.Target)
	assert.Equal(t, "en", entry.SourceLocale)
	assert.Equal(t, "fr", entry.TargetLocale)
}

func TestGetTMCount(t *testing.T) {
	app := newTestApp(t)
	info, err := app.CreateProject("TM Test", "en", []string{"fr"})
	require.NoError(t, err)

	count, err := app.GetTMCount(info.ID)
	require.NoError(t, err)
	assert.Equal(t, 0, count)

	_, err = app.AddTMEntry(info.ID, "Hello", "Bonjour", "en", "fr")
	require.NoError(t, err)

	count, err = app.GetTMCount(info.ID)
	require.NoError(t, err)
	assert.Equal(t, 1, count)
}

func TestGetTMEntries(t *testing.T) {
	app := newTestApp(t)
	info, err := app.CreateProject("TM Test", "en", []string{"fr", "de"})
	require.NoError(t, err)

	_, err = app.AddTMEntry(info.ID, "Hello", "Bonjour", "en", "fr")
	require.NoError(t, err)
	_, err = app.AddTMEntry(info.ID, "Goodbye", "Au revoir", "en", "fr")
	require.NoError(t, err)
	_, err = app.AddTMEntry(info.ID, "Hello", "Hallo", "en", "de")
	require.NoError(t, err)

	// No filter
	result, err := app.GetTMEntries(info.ID, "", "", "", 0, 50)
	require.NoError(t, err)
	assert.Equal(t, 3, result.TotalCount)
	assert.Len(t, result.Entries, 3)

	// Search by text
	result, err = app.GetTMEntries(info.ID, "hello", "", "", 0, 50)
	require.NoError(t, err)
	assert.Equal(t, 2, result.TotalCount)

	// Filter by target locale
	result, err = app.GetTMEntries(info.ID, "", "", "de", 0, 50)
	require.NoError(t, err)
	assert.Equal(t, 1, result.TotalCount)
	assert.Equal(t, "Hallo", result.Entries[0].Target)

	// Pagination
	result, err = app.GetTMEntries(info.ID, "", "", "", 0, 2)
	require.NoError(t, err)
	assert.Equal(t, 3, result.TotalCount)
	assert.Len(t, result.Entries, 2)
}

func TestUpdateTMEntry(t *testing.T) {
	app := newTestApp(t)
	info, err := app.CreateProject("TM Test", "en", []string{"fr"})
	require.NoError(t, err)

	entry, err := app.AddTMEntry(info.ID, "Hello", "Bonjour", "en", "fr")
	require.NoError(t, err)

	err = app.UpdateTMEntry(TMUpdateRequest{
		ProjectID:    info.ID,
		EntryID:      entry.ID,
		Source:       "Hello",
		Target:       "Salut",
		SourceLocale: "en",
		TargetLocale: "fr",
	})
	require.NoError(t, err)

	// Verify update
	result, err := app.GetTMEntries(info.ID, "Hello", "", "", 0, 50)
	require.NoError(t, err)
	require.Len(t, result.Entries, 1)
	assert.Equal(t, "Salut", result.Entries[0].Target)
}

func TestUpdateTMEntry_NotFound(t *testing.T) {
	app := newTestApp(t)
	info, err := app.CreateProject("TM Test", "en", []string{"fr"})
	require.NoError(t, err)

	err = app.UpdateTMEntry(TMUpdateRequest{
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

func TestDeleteTMEntry(t *testing.T) {
	app := newTestApp(t)
	info, err := app.CreateProject("TM Test", "en", []string{"fr"})
	require.NoError(t, err)

	entry, err := app.AddTMEntry(info.ID, "Hello", "Bonjour", "en", "fr")
	require.NoError(t, err)

	err = app.DeleteTMEntry(info.ID, entry.ID)
	require.NoError(t, err)

	count, err := app.GetTMCount(info.ID)
	require.NoError(t, err)
	assert.Equal(t, 0, count)
}

func TestDeleteTMEntry_NotFound(t *testing.T) {
	app := newTestApp(t)
	info, err := app.CreateProject("TM Test", "en", []string{"fr"})
	require.NoError(t, err)

	err = app.DeleteTMEntry(info.ID, "nonexistent")
	assert.Error(t, err)
}

func TestTMTranslateItem_UsesProjectTM(t *testing.T) {
	app, info, itemName := setupProjectWithFile(t)

	// Add entries to the project TM
	_, err := app.AddTMEntry(info.ID, "Hello, world!", "Bonjour le monde!", "en", "fr")
	require.NoError(t, err)

	// Now run TM translate -- the project's TM should be used
	stats, err := app.TMTranslateItem(info.ID, itemName, "fr")
	require.NoError(t, err)
	assert.Greater(t, stats.TotalBlocks, 0)
}

func TestCloseProject_ClosesTM(t *testing.T) {
	app := newTestApp(t)
	info, err := app.CreateProject("TM Test", "en", []string{"fr"})
	require.NoError(t, err)

	// Force TM creation
	_, err = app.AddTMEntry(info.ID, "Hello", "Bonjour", "en", "fr")
	require.NoError(t, err)

	// Close should not error
	err = app.CloseProject(info.ID)
	require.NoError(t, err)

	// Project should no longer exist
	_, err = app.GetProject(info.ID)
	assert.Error(t, err)
}
