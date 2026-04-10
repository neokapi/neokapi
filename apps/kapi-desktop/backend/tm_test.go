package backend

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/sievepen"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestApp(t *testing.T) *App {
	t.Helper()
	app := NewApp()
	t.Cleanup(func() {
		app.tmHandles.CloseAll()
		app.tbHandles.CloseAll()
	})
	return app
}

func openTestTM(t *testing.T, app *App) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "test.db")
	handle, err := app.OpenTM(path)
	require.NoError(t, err)
	require.NotEmpty(t, handle)
	t.Cleanup(func() { app.CloseTM(handle) })
	return handle
}

// multilingualInput builds a variants map with three locales.
func multilingualInput(en, fr, de string) map[string]VariantInputDTO {
	return map[string]VariantInputDTO{
		"en-US": {Text: en},
		"fr-FR": {Text: fr},
		"de-DE": {Text: de},
	}
}

func TestTM_AddMultilingualEntry(t *testing.T) {
	app := newTestApp(t)
	handle := openTestTM(t, app)

	err := app.AddTMEntry(handle, AddTMEntryRequest{
		Variants:    multilingualInput("Hello", "Bonjour", "Hallo"),
		HintSrcLang: "en-US",
	})
	require.NoError(t, err)
	assert.Equal(t, 1, app.GetTMStats(handle).Count)

	result := app.SearchTMEntries(handle, "", "", "", 0, 50)
	require.Equal(t, 1, result.TotalCount)
	require.Len(t, result.Entries, 1)
	e := result.Entries[0]
	require.Contains(t, e.Variants, "en-US")
	assert.Equal(t, "Hello", e.Variants["en-US"].Text)
	assert.Equal(t, "Bonjour", e.Variants["fr-FR"].Text)
	assert.Equal(t, "Hallo", e.Variants["de-DE"].Text)
	assert.Equal(t, "en-US", e.HintSrcLang)
}

func TestTM_UpdateEntry_VariantsMap(t *testing.T) {
	app := newTestApp(t)
	handle := openTestTM(t, app)

	require.NoError(t, app.AddTMEntry(handle, AddTMEntryRequest{
		Variants:    multilingualInput("Save", "Enregistrer", "Speichern"),
		HintSrcLang: "en-US",
	}))
	r := app.SearchTMEntries(handle, "", "", "", 0, 10)
	require.Len(t, r.Entries, 1)
	eid := r.Entries[0].ID

	// Replace: add Italian, drop German.
	require.NoError(t, app.UpdateTMEntry(handle, UpdateTMEntryRequest{
		EntryID: eid,
		Variants: map[string]VariantInputDTO{
			"en-US": {Text: "Save"},
			"fr-FR": {Text: "Enregistrer"},
			"it-IT": {Text: "Salva"},
		},
		HintSrcLang: "en-US",
	}))
	got := app.GetTMEntry(handle, eid)
	require.NotNil(t, got)
	assert.Contains(t, got.Variants, "it-IT")
	assert.NotContains(t, got.Variants, "de-DE")
}

func TestTM_SearchReturnsVariants(t *testing.T) {
	app := newTestApp(t)
	handle := openTestTM(t, app)
	require.NoError(t, app.AddTMEntry(handle, AddTMEntryRequest{
		Variants:    multilingualInput("Hello world", "Bonjour monde", "Hallo welt"),
		HintSrcLang: "en-US",
	}))
	result := app.SearchTMEntries(handle, "monde", "", "", 0, 10)
	require.Equal(t, 1, result.TotalCount)
}

func TestTM_GetFacets_LocalesAndSessions(t *testing.T) {
	app := newTestApp(t)
	handle := openTestTM(t, app)

	// Seed a session and an entry tagged with it.
	tm, _ := app.tmHandles.Get(handle)
	require.NoError(t, tm.CreateImportSession(sievepen.ImportSession{
		ID: "s1", FileKey: "seed.tmx", ImportedAt: time.Now(),
	}))
	require.NoError(t, tm.Add(sievepen.TMEntry{
		ID: "e1",
		Variants: map[model.LocaleID]*model.Fragment{
			"en-US": model.NewFragment("hi"),
			"fr-FR": model.NewFragment("salut"),
		},
		Origins: []sievepen.Origin{{Source: "import", SessionID: "s1"}},
	}))

	facets := app.GetTMFacets(handle)
	require.NotNil(t, facets)
	assert.NotEmpty(t, facets.Locales)
	assert.Len(t, facets.ImportSessions, 1)
	assert.Equal(t, "s1", facets.ImportSessions[0].SessionID)
}

func TestTM_ListImportSessions(t *testing.T) {
	app := newTestApp(t)
	handle := openTestTM(t, app)
	tm, _ := app.tmHandles.Get(handle)
	require.NoError(t, tm.CreateImportSession(sievepen.ImportSession{
		ID: "s1", FileKey: "a.tmx", ImportedAt: time.Now(),
	}))
	sessions := app.ListTMImportSessions(handle)
	require.Len(t, sessions, 1)
	assert.Equal(t, "a.tmx", sessions[0].FileKey)
}

func TestTM_GetImportSession_NotFound(t *testing.T) {
	app := newTestApp(t)
	handle := openTestTM(t, app)
	assert.Nil(t, app.GetTMImportSession(handle, "missing"))
}

func TestTM_DeleteImportSession_KeepsEntries(t *testing.T) {
	app := newTestApp(t)
	handle := openTestTM(t, app)
	tm, _ := app.tmHandles.Get(handle)
	require.NoError(t, tm.CreateImportSession(sievepen.ImportSession{
		ID: "s1", FileKey: "a.tmx", ImportedAt: time.Now(),
	}))
	require.NoError(t, tm.Add(sievepen.TMEntry{
		ID: "e1",
		Variants: map[model.LocaleID]*model.Fragment{
			"en-US": model.NewFragment("hi"),
			"fr-FR": model.NewFragment("salut"),
		},
		Origins: []sievepen.Origin{{Source: "import", SessionID: "s1"}},
	}))
	require.NoError(t, app.DeleteTMImportSession(handle, "s1"))
	got := app.GetTMEntry(handle, "e1")
	require.NotNil(t, got)
	require.Len(t, got.Origins, 1)
	assert.Equal(t, "", got.Origins[0].SessionID)
}

func TestTM_DeleteEntry(t *testing.T) {
	app := newTestApp(t)
	handle := openTestTM(t, app)
	require.NoError(t, app.AddTMEntry(handle, AddTMEntryRequest{
		Variants:    multilingualInput("Save", "Enregistrer", "Speichern"),
		HintSrcLang: "en-US",
	}))
	r := app.SearchTMEntries(handle, "", "", "", 0, 10)
	require.Len(t, r.Entries, 1)
	require.NoError(t, app.DeleteTMEntry(handle, r.Entries[0].ID))
	assert.Equal(t, 0, app.GetTMStats(handle).Count)
}
