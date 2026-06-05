package sievepen_test

import (
	"context"
	"database/sql"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/sievepen"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// trilingual is a convenience helper for building 3-variant entries.
func trilingual(id, en, fr, de string) sievepen.TMEntry {
	return sievepen.TMEntry{
		ID: id,
		Variants: map[model.LocaleID][]model.Run{
			"en": {{Text: &model.TextRun{Text: en}}},
			"fr": {{Text: &model.TextRun{Text: fr}}},
			"de": {{Text: &model.TextRun{Text: de}}},
		},
		HintSrcLang: "en",
	}
}

// --- Multilingual CRUD ---

func TestSQLiteTM_MultilingualAdd(t *testing.T) {
	tm, err := sievepen.NewSQLiteTM(":memory:")
	require.NoError(t, err)
	defer tm.Close()

	require.NoError(t, tm.Add(trilingual("e1", "Hello", "Bonjour", "Hallo")))
	assert.Equal(t, 1, tm.Count())

	got, ok := tm.GetEntry("e1")
	require.True(t, ok)
	assert.Equal(t, "Hello", got.VariantText("en"))
	assert.Equal(t, "Bonjour", got.VariantText("fr"))
	assert.Equal(t, "Hallo", got.VariantText("de"))
	assert.Equal(t, []model.LocaleID{"de", "en", "fr"}, got.Locales())
}

func TestSQLiteTM_MultilingualUpdate(t *testing.T) {
	tm, err := sievepen.NewSQLiteTM(":memory:")
	require.NoError(t, err)
	defer tm.Close()

	require.NoError(t, tm.Add(trilingual("e1", "Hello", "Bonjour", "Hallo")))
	// Replace: add Italian, drop German.
	entry := sievepen.TMEntry{
		ID: "e1",
		Variants: map[model.LocaleID][]model.Run{
			"en": {{Text: &model.TextRun{Text: "Hello"}}},
			"fr": {{Text: &model.TextRun{Text: "Bonjour"}}},
			"it": {{Text: &model.TextRun{Text: "Ciao"}}},
		},
	}
	require.NoError(t, tm.Add(entry))
	got, _ := tm.GetEntry("e1")
	assert.Equal(t, []model.LocaleID{"en", "fr", "it"}, got.Locales())
	assert.Equal(t, "Ciao", got.VariantText("it"))
}

func TestSQLiteTM_DeleteCascades(t *testing.T) {
	tm, err := sievepen.NewSQLiteTM(":memory:")
	require.NoError(t, err)
	defer tm.Close()
	require.NoError(t, tm.Add(trilingual("e1", "a", "b", "c")))
	require.NoError(t, tm.Delete("e1"))
	_, ok := tm.GetEntry("e1")
	assert.False(t, ok)
	assert.Equal(t, 0, tm.Count())
}

// fullEntry builds an entry that touches every child table that Delete must
// clean up: variants (→ tm_variants + the two FTS5 side-tables), entities
// (→ tm_entry_entities), entity values (→ tm_entry_entity_values), and
// origins (→ tm_entry_origins).
func fullEntry(id string) sievepen.TMEntry {
	e := trilingual(id, "John works at Acme", "Jean travaille chez Acme", "Johann arbeitet bei Acme")
	e.Entities = []sievepen.EntityMapping{
		{
			PlaceholderID: "p1",
			Type:          "entity:person",
			ConceptID:     "concept-john",
			Values: map[model.LocaleID]sievepen.EntityValue{
				"en": {Text: "John", Start: 0, End: 4},
				"fr": {Text: "Jean", Start: 0, End: 4},
			},
		},
	}
	e.Origins = []sievepen.Origin{
		{Source: "import", Key: "a.tmx"},
		{Source: "manual"},
	}
	return e
}

// childTableCount returns the number of rows in tableName whose entry_id equals
// id, querying the underlying connection directly so the assertion is
// independent of any Go-side caching.
func childTableCount(t *testing.T, db interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, tableName, id string,
) int {
	t.Helper()
	var n int
	q := "SELECT COUNT(*) FROM " + tableName + " WHERE entry_id = ?"
	require.NoError(t, db.QueryRowContext(context.Background(), q, id).Scan(&n))
	return n
}

// TestSQLiteTM_DeleteRemovesAllChildRows asserts that Delete removes every
// child row across all owned tables — even when foreign_keys is OFF, proving
// the delete no longer depends on ON DELETE CASCADE. Regression test for the
// audit finding that Delete was non-transactional and leaned on cascade for
// tm_variants / tm_entry_entities / tm_entry_origins, leaving orphan rows.
func TestSQLiteTM_DeleteRemovesAllChildRows(t *testing.T) {
	dir := t.TempDir()
	tm, err := sievepen.NewSQLiteTM(dir + "/tm.db")
	require.NoError(t, err)
	defer tm.Close()

	// Pin the pool to a single connection and force foreign_keys OFF on it,
	// so any reliance on ON DELETE CASCADE would leave orphan child rows.
	db := tm.DB()
	db.SetMaxOpenConns(1)
	_, err = db.ExecContext(context.Background(), "PRAGMA foreign_keys=OFF")
	require.NoError(t, err)
	var fk int
	require.NoError(t, db.QueryRowContext(context.Background(), "PRAGMA foreign_keys").Scan(&fk))
	require.Equal(t, 0, fk, "foreign_keys must be OFF to prove independence from cascade")

	require.NoError(t, tm.Add(fullEntry("e1")))
	// A second entry that must remain untouched by the delete.
	require.NoError(t, tm.Add(fullEntry("e2")))

	childTables := []string{
		"tm_variants",
		"tm_variant_search",
		"tm_variant_trigram",
		"tm_entry_entities",
		"tm_entry_entity_values",
		"tm_entry_origins",
	}

	// Sanity: every child table has rows for e1 before the delete.
	for _, tbl := range childTables {
		require.Positivef(t, childTableCount(t, db, tbl, "e1"), "%s should have rows for e1 before delete", tbl)
	}

	require.NoError(t, tm.Delete("e1"))

	// Main row gone.
	_, ok := tm.GetEntry("e1")
	assert.False(t, ok)

	// Every child table is empty for e1, despite foreign_keys=OFF.
	for _, tbl := range childTables {
		assert.Zerof(t, childTableCount(t, db, tbl, "e1"), "%s still has orphan rows for e1 after delete", tbl)
	}

	// The untouched entry e2 keeps all of its child rows.
	for _, tbl := range childTables {
		assert.Positivef(t, childTableCount(t, db, tbl, "e2"), "%s lost rows for the unrelated entry e2", tbl)
	}
	got, ok := tm.GetEntry("e2")
	require.True(t, ok)
	assert.Equal(t, "John works at Acme", got.VariantText("en"))
}

// TestSQLiteTM_DeleteMissingEntry asserts that deleting an unknown ID reports
// not-found and leaves unrelated data intact (the transaction rolls back its
// no-op child deletes without disturbing other entries).
func TestSQLiteTM_DeleteMissingEntry(t *testing.T) {
	tm, err := sievepen.NewSQLiteTM(":memory:")
	require.NoError(t, err)
	defer tm.Close()

	require.NoError(t, tm.Add(fullEntry("keep")))

	err = tm.Delete("nope")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")

	_, ok := tm.GetEntry("keep")
	assert.True(t, ok)
	assert.Equal(t, 1, tm.Count())
}

func TestSQLiteTM_NoVariantsError(t *testing.T) {
	tm, err := sievepen.NewSQLiteTM(":memory:")
	require.NoError(t, err)
	defer tm.Close()
	err = tm.Add(sievepen.TMEntry{ID: "e1"})
	assert.ErrorIs(t, err, sievepen.ErrEntryNoVariants)
}

// --- Lookup ---

func TestSQLiteTM_LookupPlain(t *testing.T) {
	tm, err := sievepen.NewSQLiteTM(":memory:")
	require.NoError(t, err)
	defer tm.Close()
	require.NoError(t, tm.Add(trilingual("e1", "Save", "Enregistrer", "Speichern")))

	matches, err := tm.LookupText("Save", "en", "fr", sievepen.DefaultLookupOptions())
	require.NoError(t, err)
	require.Len(t, matches, 1)
	assert.Equal(t, "Enregistrer", matches[0].Entry.VariantText("fr"))
	assert.Equal(t, sievepen.MatchExact, matches[0].MatchType)
}

func TestSQLiteTM_LookupCrossDirection(t *testing.T) {
	tm, err := sievepen.NewSQLiteTM(":memory:")
	require.NoError(t, err)
	defer tm.Close()
	require.NoError(t, tm.Add(trilingual("e1", "Save", "Enregistrer", "Speichern")))

	// Lookup from fr → de, using the fr variant as the source.
	matches, err := tm.LookupText("Enregistrer", "fr", "de", sievepen.LookupOptions{MinScore: 1.0, MaxResults: 5})
	require.NoError(t, err)
	require.Len(t, matches, 1)
	assert.Equal(t, "Speichern", matches[0].Entry.VariantText("de"))
}

func TestSQLiteTM_LookupMissingTarget(t *testing.T) {
	tm, err := sievepen.NewSQLiteTM(":memory:")
	require.NoError(t, err)
	defer tm.Close()
	require.NoError(t, tm.Add(sievepen.TMEntry{
		ID: "e1",
		Variants: map[model.LocaleID][]model.Run{
			"en": {{Text: &model.TextRun{Text: "Save"}}},
			"fr": {{Text: &model.TextRun{Text: "Enregistrer"}}},
		},
	}))

	// Target locale "de" not present.
	matches, err := tm.LookupText("Save", "en", "de", sievepen.DefaultLookupOptions())
	require.NoError(t, err)
	assert.Empty(t, matches)
}

func TestSQLiteTM_LookupFuzzy(t *testing.T) {
	tm, err := sievepen.NewSQLiteTM(":memory:")
	require.NoError(t, err)
	defer tm.Close()
	require.NoError(t, tm.Add(trilingual("e1",
		"The file was saved successfully",
		"Le fichier a été sauvegardé",
		"Die Datei wurde gespeichert")))

	matches, err := tm.LookupText("The file was saved", "en", "fr",
		sievepen.LookupOptions{MinScore: 0.5, MaxResults: 5})
	require.NoError(t, err)
	require.NotEmpty(t, matches)
	assert.Equal(t, sievepen.MatchFuzzy, matches[0].MatchType)
	assert.Greater(t, matches[0].Score, 0.5)
	assert.Less(t, matches[0].Score, 1.0)
}

// --- Search ---

func TestSQLiteTM_SearchAnyLocale(t *testing.T) {
	tm, err := sievepen.NewSQLiteTM(":memory:")
	require.NoError(t, err)
	defer tm.Close()
	require.NoError(t, tm.Add(trilingual("e1", "hello world", "bonjour monde", "hallo welt")))
	require.NoError(t, tm.Add(trilingual("e2", "goodbye", "au revoir", "auf wiedersehen")))

	entries, total := tm.SearchEntries("hello", "", "", 0, 10)
	assert.Equal(t, 1, total)
	require.Len(t, entries, 1)
	assert.Equal(t, "e1", entries[0].ID)

	// "monde" is only in the fr variant.
	entries, total = tm.SearchEntries("monde", "", "", 0, 10)
	assert.Equal(t, 1, total)
	assert.Equal(t, "e1", entries[0].ID)
}

func TestSQLiteTM_SearchRequireLocale(t *testing.T) {
	tm, err := sievepen.NewSQLiteTM(":memory:")
	require.NoError(t, err)
	defer tm.Close()

	require.NoError(t, tm.Add(sievepen.TMEntry{
		ID: "e1",
		Variants: map[model.LocaleID][]model.Run{
			"en": {{Text: &model.TextRun{Text: "hello"}}},
			"fr": {{Text: &model.TextRun{Text: "bonjour"}}},
		},
	}))
	require.NoError(t, tm.Add(sievepen.TMEntry{
		ID: "e2",
		Variants: map[model.LocaleID][]model.Run{
			"en": {{Text: &model.TextRun{Text: "hello"}}},
		},
	}))

	// Require fr variant — excludes e2.
	entries, total := tm.SearchEntries("hello", "en", "fr", 0, 10)
	assert.Equal(t, 1, total)
	require.Len(t, entries, 1)
	assert.Equal(t, "e1", entries[0].ID)
}

func TestSQLiteTM_SearchFilterSession(t *testing.T) {
	tm, err := sievepen.NewSQLiteTM(":memory:")
	require.NoError(t, err)
	defer tm.Close()

	// Create sessions and tag entries with them.
	require.NoError(t, tm.CreateImportSession(sievepen.ImportSession{ID: "s1", FileKey: "a.tmx"}))
	require.NoError(t, tm.CreateImportSession(sievepen.ImportSession{ID: "s2", FileKey: "b.tmx"}))

	e1 := trilingual("e1", "one", "un", "eins")
	e1.Origins = []sievepen.Origin{{Source: "import", Key: "a.tmx", SessionID: "s1"}}
	e2 := trilingual("e2", "two", "deux", "zwei")
	e2.Origins = []sievepen.Origin{{Source: "import", Key: "b.tmx", SessionID: "s2"}}

	require.NoError(t, tm.Add(e1))
	require.NoError(t, tm.Add(e2))

	entries, total := tm.SearchEntriesFiltered("", "", "", sievepen.SearchFilter{
		SessionIDs: []string{"s1"},
	}, 0, 10)
	assert.Equal(t, 1, total)
	require.Len(t, entries, 1)
	assert.Equal(t, "e1", entries[0].ID)
}

// --- Facets ---

func TestSQLiteTM_FacetLocales(t *testing.T) {
	tm, err := sievepen.NewSQLiteTM(":memory:")
	require.NoError(t, err)
	defer tm.Close()
	require.NoError(t, tm.Add(trilingual("e1", "a", "b", "c")))
	require.NoError(t, tm.Add(sievepen.TMEntry{
		ID: "e2",
		Variants: map[model.LocaleID][]model.Run{
			"en": {{Text: &model.TextRun{Text: "d"}}},
			"fr": {{Text: &model.TextRun{Text: "e"}}},
		},
	}))

	facets := tm.FacetStats()
	counts := map[string]int{}
	for _, lf := range facets.Locales {
		counts[lf.Locale] = lf.Count
	}
	assert.Equal(t, 2, counts["en"])
	assert.Equal(t, 2, counts["fr"])
	assert.Equal(t, 1, counts["de"])
}

func TestSQLiteTM_FacetImportSessions(t *testing.T) {
	tm, err := sievepen.NewSQLiteTM(":memory:")
	require.NoError(t, err)
	defer tm.Close()

	require.NoError(t, tm.CreateImportSession(sievepen.ImportSession{ID: "s1", FileKey: "norwegian.tmx", ToolName: "bitextor"}))
	e := trilingual("e1", "one", "un", "eins")
	e.Origins = []sievepen.Origin{{Source: "import", SessionID: "s1"}}
	require.NoError(t, tm.Add(e))

	facets := tm.FacetStats()
	require.Len(t, facets.ImportSessions, 1)
	assert.Equal(t, "s1", facets.ImportSessions[0].SessionID)
	assert.Equal(t, 1, facets.ImportSessions[0].Count)
	assert.Equal(t, "norwegian.tmx", facets.ImportSessions[0].FileKey)
}

// --- Import sessions CRUD ---

func TestSQLiteTM_ImportSessionCRUD(t *testing.T) {
	tm, err := sievepen.NewSQLiteTM(":memory:")
	require.NoError(t, err)
	defer tm.Close()

	s := sievepen.ImportSession{
		ID: "s1", FileKey: "a.tmx", FileHash: "deadbeef",
		ToolName: "tmx-import", EntryCount: 0,
		Properties: map[string]string{"foo": "bar"},
	}
	require.NoError(t, tm.CreateImportSession(s))

	got, ok := tm.GetImportSession("s1")
	require.True(t, ok)
	assert.Equal(t, "a.tmx", got.FileKey)
	assert.Equal(t, "bar", got.Properties["foo"])

	_, ok = tm.GetImportSession("missing")
	assert.False(t, ok)

	found, ok := tm.FindImportSessionByHash("deadbeef")
	require.True(t, ok)
	assert.Equal(t, "s1", found.ID)

	require.NoError(t, tm.UpdateImportSessionCount("s1", 42))
	got, _ = tm.GetImportSession("s1")
	assert.Equal(t, 42, got.EntryCount)

	list := tm.ListImportSessions()
	require.Len(t, list, 1)

	require.NoError(t, tm.DeleteImportSession("s1"))
	_, ok = tm.GetImportSession("s1")
	assert.False(t, ok)
}

func TestSQLiteTM_DeleteSessionKeepsOrigins(t *testing.T) {
	tm, err := sievepen.NewSQLiteTM(":memory:")
	require.NoError(t, err)
	defer tm.Close()

	require.NoError(t, tm.CreateImportSession(sievepen.ImportSession{ID: "s1", FileKey: "a.tmx"}))
	e := trilingual("e1", "one", "un", "eins")
	e.Origins = []sievepen.Origin{{Source: "import", SessionID: "s1"}}
	require.NoError(t, tm.Add(e))

	require.NoError(t, tm.DeleteImportSession("s1"))

	got, ok := tm.GetEntry("e1")
	require.True(t, ok)
	require.Len(t, got.Origins, 1)
	assert.Empty(t, got.Origins[0].SessionID)
}

// --- Entity mapping (multilingual) ---

func TestSQLiteTM_EntityMappingRoundtrip(t *testing.T) {
	tm, err := sievepen.NewSQLiteTM(":memory:")
	require.NoError(t, err)
	defer tm.Close()

	entry := trilingual("e1", "John works at Acme", "Jean travaille chez Acme", "Johann arbeitet bei Acme")
	entry.Entities = []sievepen.EntityMapping{
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
	require.NoError(t, tm.Add(entry))

	got, ok := tm.GetEntry("e1")
	require.True(t, ok)
	require.Len(t, got.Entities, 1)
	assert.Equal(t, "e1", got.Entities[0].PlaceholderID)
	assert.Equal(t, "John", got.Entities[0].Values["en"].Text)
	assert.Equal(t, "Jean", got.Entities[0].Values["fr"].Text)
	assert.Equal(t, "Johann", got.Entities[0].Values["de"].Text)
}

func TestSQLiteTM_EntityConceptIDRoundtrip(t *testing.T) {
	tm, err := sievepen.NewSQLiteTM(":memory:")
	require.NoError(t, err)
	defer tm.Close()

	entry := trilingual("e1", "Acme ships fast", "Acme livre vite", "Acme liefert schnell")
	entry.Entities = []sievepen.EntityMapping{
		{
			PlaceholderID: "e1",
			Type:          "entity:organization",
			ConceptID:     "concept-acme-corp",
			Values: map[model.LocaleID]sievepen.EntityValue{
				"en": {Text: "Acme"},
				"fr": {Text: "Acme"},
				"de": {Text: "Acme"},
			},
		},
	}
	require.NoError(t, tm.Add(entry))

	got, ok := tm.GetEntry("e1")
	require.True(t, ok)
	require.Len(t, got.Entities, 1)
	assert.Equal(t, "concept-acme-corp", got.Entities[0].ConceptID, "concept_id should round-trip")
}

// --- ICU tokenizer — multilingual search ---
//
// Tests that depend on word segmentation of scripts without explicit word
// boundaries (CJK, Thai) require the ICU tokenizer and live in the cgo-only
// file sqlite_icu_test.go. The no-cgo (modernc + unicode61) counterparts that
// assert whole-token search behaviour live in sqlite_nocgo_test.go. The Arabic
// and Korean cases below use space-separated words, so they behave the same
// under both tokenizers and stay shared here.

func TestSQLiteTM_SearchArabic(t *testing.T) {
	tm, err := sievepen.NewSQLiteTM(":memory:")
	require.NoError(t, err)
	defer tm.Close()

	require.NoError(t, tm.Add(sievepen.TMEntry{
		ID: "e1",
		Variants: map[model.LocaleID][]model.Run{
			"ar-SA": {{Text: &model.TextRun{Text: "اختبار البحث باللغة العربية"}}},
			"en":    {{Text: &model.TextRun{Text: "Arabic language search test"}}},
		},
		HintSrcLang: "ar-SA",
	}))

	entries, total := tm.SearchEntries("البحث", "ar-SA", "", 0, 10)
	assert.Equal(t, 1, total, "ICU should handle Arabic search")
	if len(entries) > 0 {
		assert.Equal(t, "e1", entries[0].ID)
	}
}

func TestSQLiteTM_SearchKorean(t *testing.T) {
	tm, err := sievepen.NewSQLiteTM(":memory:")
	require.NoError(t, err)
	defer tm.Close()

	require.NoError(t, tm.Add(sievepen.TMEntry{
		ID: "e1",
		Variants: map[model.LocaleID][]model.Run{
			"ko-KR": {{Text: &model.TextRun{Text: "한국어 번역 테스트입니다"}}},
			"en":    {{Text: &model.TextRun{Text: "Korean translation test"}}},
		},
		HintSrcLang: "ko-KR",
	}))

	entries, total := tm.SearchEntries("번역", "ko-KR", "", 0, 10)
	assert.Equal(t, 1, total, "ICU should handle Korean search")
	if len(entries) > 0 {
		assert.Equal(t, "e1", entries[0].ID)
	}
}

// TestSQLiteTM_SearchLatinRegression ensures Latin-script search still
// works correctly after switching from unicode61 to ICU.
func TestSQLiteTM_SearchLatinRegression(t *testing.T) {
	tm, err := sievepen.NewSQLiteTM(":memory:")
	require.NoError(t, err)
	defer tm.Close()

	require.NoError(t, tm.Add(trilingual("e1", "hello world", "bonjour monde", "hallo welt")))
	require.NoError(t, tm.Add(trilingual("e2", "goodbye", "au revoir", "auf wiedersehen")))

	entries, _ := tm.SearchEntries("hello", "", "", 0, 10)
	require.Len(t, entries, 1)
	assert.Equal(t, "e1", entries[0].ID)

	entries, _ = tm.SearchEntries("revoir", "", "", 0, 10)
	require.Len(t, entries, 1)
	assert.Equal(t, "e2", entries[0].ID)
}

// --- Locale stats & activity ---

func TestSQLiteTM_LocaleStats(t *testing.T) {
	tm, err := sievepen.NewSQLiteTM(":memory:")
	require.NoError(t, err)
	defer tm.Close()
	require.NoError(t, tm.Add(trilingual("e1", "a", "b", "c")))
	stats := tm.LocaleStats()
	require.Len(t, stats, 3)
	for _, s := range stats {
		assert.Equal(t, 1, s.Count)
	}
}
