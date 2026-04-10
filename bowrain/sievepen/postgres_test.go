//go:build integration

package sievepen_test

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/sievepen"

	pgtm "github.com/neokapi/neokapi/bowrain/sievepen"
	storage "github.com/neokapi/neokapi/bowrain/storage"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func openTestPostgresTM(t *testing.T) *pgtm.PostgresTM {
	t.Helper()
	connStr := os.Getenv("BOWRAIN_TEST_POSTGRES_URL")
	if connStr == "" {
		connStr = "postgres://bowrain:bowrain@localhost:5432/bowrain_test?sslmode=disable"
	}
	db, err := storage.OpenPostgres(connStr)
	if err != nil {
		t.Skipf("PostgreSQL not available: %v", err)
	}
	wsID := fmt.Sprintf("test-%s-%d", t.Name(), time.Now().UnixNano())
	tm, err := pgtm.NewPostgresTMFromDB(db, wsID)
	require.NoError(t, err)
	t.Cleanup(func() {
		db.Exec("DELETE FROM tm_entries WHERE workspace_id = $1", wsID)
		db.Exec("DELETE FROM tm_import_sessions WHERE workspace_id = $1", wsID)
		db.Close()
	})
	return tm
}

func trilingual(id, en, fr, de string) sievepen.TMEntry {
	return sievepen.TMEntry{
		ID: id,
		Variants: map[model.LocaleID]*model.Fragment{
			"en": model.NewFragment(en),
			"fr": model.NewFragment(fr),
			"de": model.NewFragment(de),
		},
		HintSrcLang: "en",
	}
}

func TestPostgresTM_MultilingualAddAndLookup(t *testing.T) {
	tm := openTestPostgresTM(t)
	require.NoError(t, tm.Add(trilingual("e1", "Hello", "Bonjour", "Hallo")))
	assert.Equal(t, 1, tm.Count())

	matches, err := tm.LookupText("Hello", "en", "fr", sievepen.DefaultLookupOptions())
	require.NoError(t, err)
	require.Len(t, matches, 1)
	assert.Equal(t, "Bonjour", matches[0].Entry.VariantText("fr"))
	assert.Equal(t, 1.0, matches[0].Score)
	assert.Equal(t, sievepen.MatchExact, matches[0].MatchType)
}

func TestPostgresTM_LookupCrossDirection(t *testing.T) {
	tm := openTestPostgresTM(t)
	require.NoError(t, tm.Add(trilingual("e1", "Save", "Enregistrer", "Speichern")))
	matches, err := tm.LookupText("Enregistrer", "fr", "de", sievepen.DefaultLookupOptions())
	require.NoError(t, err)
	require.Len(t, matches, 1)
	assert.Equal(t, "Speichern", matches[0].Entry.VariantText("de"))
}

func TestPostgresTM_SearchRequireLocale(t *testing.T) {
	tm := openTestPostgresTM(t)
	require.NoError(t, tm.Add(sievepen.TMEntry{
		ID: "e1",
		Variants: map[model.LocaleID]*model.Fragment{
			"en": model.NewFragment("hello"),
			"fr": model.NewFragment("bonjour"),
		},
	}))
	require.NoError(t, tm.Add(sievepen.TMEntry{
		ID: "e2",
		Variants: map[model.LocaleID]*model.Fragment{
			"en": model.NewFragment("hello"),
		},
	}))
	entries, total := tm.SearchEntries("hello", "en", "fr", 0, 10)
	assert.Equal(t, 1, total)
	require.Len(t, entries, 1)
	assert.Equal(t, "e1", entries[0].ID)
}

func TestPostgresTM_FacetLocales(t *testing.T) {
	tm := openTestPostgresTM(t)
	require.NoError(t, tm.Add(trilingual("e1", "a", "b", "c")))
	require.NoError(t, tm.Add(sievepen.TMEntry{
		ID: "e2",
		Variants: map[model.LocaleID]*model.Fragment{
			"en": model.NewFragment("d"),
			"fr": model.NewFragment("e"),
		},
	}))
	f := tm.FacetStats()
	counts := map[string]int{}
	for _, lf := range f.Locales {
		counts[lf.Locale] = lf.Count
	}
	assert.Equal(t, 2, counts["en"])
	assert.Equal(t, 2, counts["fr"])
	assert.Equal(t, 1, counts["de"])
}

func TestPostgresTM_ImportSessionCRUD(t *testing.T) {
	tm := openTestPostgresTM(t)
	require.NoError(t, tm.CreateImportSession(sievepen.ImportSession{
		ID: "s1", FileKey: "a.tmx", FileHash: "deadbeef",
	}))
	s, ok := tm.GetImportSession("s1")
	require.True(t, ok)
	assert.Equal(t, "a.tmx", s.FileKey)

	require.NoError(t, tm.UpdateImportSessionCount("s1", 42))
	s, _ = tm.GetImportSession("s1")
	assert.Equal(t, 42, s.EntryCount)

	hit, ok := tm.FindImportSessionByHash("deadbeef")
	require.True(t, ok)
	assert.Equal(t, "s1", hit.ID)

	require.NoError(t, tm.DeleteImportSession("s1"))
	_, ok = tm.GetImportSession("s1")
	assert.False(t, ok)
}

func TestPostgresTM_DeleteSessionKeepsOrigins(t *testing.T) {
	tm := openTestPostgresTM(t)
	require.NoError(t, tm.CreateImportSession(sievepen.ImportSession{ID: "s1", FileKey: "a.tmx"}))
	e := trilingual("e1", "a", "b", "c")
	e.Origins = []sievepen.Origin{{Source: "import", SessionID: "s1"}}
	require.NoError(t, tm.Add(e))
	require.NoError(t, tm.DeleteImportSession("s1"))
	got, ok := tm.GetEntry("e1")
	require.True(t, ok)
	require.Len(t, got.Origins, 1)
	assert.Equal(t, "", got.Origins[0].SessionID)
}

func TestPostgresTM_EntityRoundtrip(t *testing.T) {
	tm := openTestPostgresTM(t)
	e := trilingual("e1", "John works here", "Jean travaille ici", "Johann arbeitet hier")
	e.Entities = []sievepen.EntityMapping{
		{
			PlaceholderID: "e1",
			Type:          "entity:person",
			Values: map[model.LocaleID]sievepen.EntityValue{
				"en": {Text: "John", Start: 0, End: 4},
				"fr": {Text: "Jean", Start: 0, End: 4},
				"de": {Text: "Johann", Start: 0, End: 6},
			},
		},
	}
	require.NoError(t, tm.Add(e))
	got, ok := tm.GetEntry("e1")
	require.True(t, ok)
	require.Len(t, got.Entities, 1)
	assert.Equal(t, "Jean", got.Entities[0].Values["fr"].Text)
}
