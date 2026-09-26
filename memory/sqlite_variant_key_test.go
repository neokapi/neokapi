package memory

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func keyedEntry(id, en, fr string) Entry {
	return Entry{
		ID:          id,
		HintSrcLang: "en",
		Variants: map[model.LocaleID][]model.Run{
			"en": {{Text: &model.TextRun{Text: en}}},
			"fr": {{Text: &model.TextRun{Text: fr}}},
		},
	}
}

// assertIndexKeyed asserts that each FTS5 table holds exactly one row per
// variant, under the variant's vid, carrying that variant's text.
func assertIndexKeyed(t *testing.T, db *storage.DB) {
	t.Helper()
	ctx := context.Background()
	var variants int
	require.NoError(t, db.QueryRowContext(ctx, `SELECT COUNT(*) FROM tm_variants`).Scan(&variants))
	for table, textCol := range map[string]string{"tm_variant_search": "text", "tm_variant_trigram": "plain"} {
		var rows, matched int
		require.NoError(t, db.QueryRowContext(ctx, `SELECT COUNT(*) FROM `+table).Scan(&rows))
		require.NoError(t, db.QueryRowContext(ctx, `SELECT COUNT(*) FROM `+table+` s
			JOIN tm_variants v ON v.vid = s.rowid
			WHERE s.`+textCol+` = v.plain AND s.entry_id = v.entry_id AND s.locale = v.locale`).Scan(&matched))
		assert.Equal(t, variants, rows, "%s rows", table)
		assert.Equal(t, variants, matched, "%s rows keyed by their variant", table)
	}
}

// A write replaces a variant's index rows by the variant's key, so a rewrite
// and a delete leave no stale row behind.
func TestVariantIndexFollowsTheVariantKey(t *testing.T) {
	ctx := context.Background()
	tm, err := NewSQLiteStore(":memory:")
	require.NoError(t, err)
	defer tm.Close()

	require.NoError(t, tm.Add(ctx, keyedEntry("e1", "Save the file", "Enregistrer le fichier")))
	require.NoError(t, tm.Add(ctx, keyedEntry("e2", "Open the file", "Ouvrir le fichier")))
	require.NoError(t, tm.Add(ctx, Entry{ID: "e1", HintSrcLang: "en", Variants: map[model.LocaleID][]model.Run{
		"fr": {{Text: &model.TextRun{Text: "Sauvegarder le fichier"}}},
	}}))
	require.NoError(t, tm.Delete(ctx, "e2"))
	assertIndexKeyed(t, tm.DB())

	got, _, err := tm.SearchEntries(ctx, SearchParams{Query: "Sauvegarder", Limit: 10})
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "e1", got[0].ID)
	got, _, err = tm.SearchEntries(ctx, SearchParams{Query: "Enregistrer", Limit: 10})
	require.NoError(t, err)
	assert.Empty(t, got, "the replaced wording is gone from the index")

	require.NoError(t, tm.RebuildSearchIndex(ctx))
	require.NoError(t, tm.RebuildFuzzyIndex(ctx))
	assertIndexKeyed(t, tm.DB())
}

// A store written before the variant key existed keeps its entries, and its
// index is re-keyed by the migration.
func TestMigrationKeysAnExistingIndex(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "memory.db")
	db, err := storage.Open(path)
	require.NoError(t, err)
	require.NoError(t, storage.Migrate(db, migrationsTable, memoryMigrations[:5]))
	for _, stmt := range []string{
		`INSERT INTO tm_entries (id, created_at, updated_at) VALUES ('old', '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')`,
		`INSERT INTO tm_variants (entry_id, locale, coded, plain, struct_key, general_key)
			VALUES ('old', 'en', '[{"text":{"text":"Close the window"}}]', 'close the window', 'close the window', 'close the window')`,
		`INSERT INTO tm_variant_search (text, locale, entry_id) VALUES ('close the window', 'en', 'old')`,
		`INSERT INTO tm_variant_trigram (plain, struct_key, general_key, locale, entry_id)
			VALUES ('close the window', 'close the window', 'close the window', 'en', 'old')`,
	} {
		_, err := db.ExecContext(ctx, stmt)
		require.NoError(t, err)
	}
	require.NoError(t, db.Close())

	tm, err := NewSQLiteStore(path)
	require.NoError(t, err)
	defer tm.Close()
	assertIndexKeyed(t, tm.DB())
	require.NoError(t, tm.Add(ctx, keyedEntry("new", "Close the door", "Fermer la porte")))
	assertIndexKeyed(t, tm.DB())
	n, err := tm.Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 2, n)
}
