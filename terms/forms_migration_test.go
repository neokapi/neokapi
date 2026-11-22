package terms

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A terms database written before terms carried forms has no forms column.
// Opening it adds the column, and its existing terms read back with no forms.
func TestSQLiteStore_MigratesADatabaseWithoutForms(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "terms.db")

	db, err := storage.Open(path)
	require.NoError(t, err)
	require.NoError(t, storage.Migrate(db, migrationsTable, tbMigrations[:3]))
	_, err = db.ExecContext(ctx, `INSERT INTO tb_concepts (id, created_at, updated_at) VALUES ('c1', '2026-09-01T00:00:00Z', '2026-09-01T00:00:00Z')`)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `INSERT INTO tb_terms (concept_id, text, text_lower, locale, status) VALUES ('c1', 'varsel', 'varsel', 'nb', 'preferred')`)
	require.NoError(t, err)
	require.NoError(t, db.Close())

	tb, err := NewSQLiteStore(path)
	require.NoError(t, err)
	defer tb.Close()

	c, ok, err := tb.GetConcept(ctx, "c1")
	require.NoError(t, err)
	require.True(t, ok)
	require.Len(t, c.Terms, 1)
	assert.Nil(t, c.Terms[0].Forms)

	c.Terms[0].Forms = []string{"varsler"}
	require.NoError(t, tb.AddConcept(ctx, c))
	matches, err := tb.LookupAll(ctx, "Lese varsler", LookupOptions{SourceLocale: model.LocaleID("nb")})
	require.NoError(t, err)
	require.Len(t, matches, 1)
	assert.Equal(t, "varsel", matches[0].Term.Text)
}

func TestSQLiteStore_NormalizedLookupAcrossMigration(t *testing.T) {
	ctx := t.Context()
	path := filepath.Join(t.TempDir(), "terms.db")
	db, err := storage.Open(path)
	require.NoError(t, err)
	require.NoError(t, storage.Migrate(db, migrationsTable, tbMigrations[:6]))
	const legacy = "  Cafe\u0301\t\tAu  Lait "
	for _, row := range []struct{ id, text string }{{"old", legacy}, {"other", "milk"}} {
		_, err = db.ExecContext(ctx, `INSERT INTO tb_concepts (id, created_at, updated_at) VALUES (?, '2026-09-01T00:00:00Z', '2026-09-01T00:00:00Z')`, row.id)
		require.NoError(t, err)
		_, err = db.ExecContext(ctx, `INSERT INTO tb_terms (concept_id, text, text_lower, locale) VALUES (?, ?, ?, 'en')`, row.id, row.text, strings.ToLower(row.text))
		require.NoError(t, err)
	}
	require.NoError(t, db.Close())

	tb, err := NewSQLiteStore(path)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, tb.Close()) })
	require.NoError(t, tb.AddConcept(ctx, Concept{ID: "new", Terms: []Term{{Text: "Café Au Lait", Locale: "en"}}}))
	var key string
	require.NoError(t, tb.DB().QueryRowContext(ctx, `SELECT text_normalized FROM tb_terms WHERE concept_id = 'new'`).Scan(&key))
	require.Equal(t, "café au lait", key)
	opts := LookupOptions{SourceLocale: "en", MatchModes: []model.MatchStrategy{model.MatchStrategyNormalized}}
	matches, err := tb.Lookup(ctx, "CAFÉ AU LAIT", opts)
	require.NoError(t, err)
	assert.Equal(t, []string{"new", "old"}, MatchedConceptIDs(matches))
}

// A database whose terms were written before the normalized key existed holds
// an empty key on every row. Opening it fills the key in, across more than
// one backfill batch, and the normalized and fuzzy tiers find the rows by it.
func TestSQLiteStore_BackfillsNormalizedText(t *testing.T) {
	ctx := t.Context()
	for _, tc := range []struct {
		name    string
		applied int
	}{
		{"before the column", 6},
		{"column left empty", 7},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "terms.db")
			db, err := storage.Open(path)
			require.NoError(t, err)
			require.NoError(t, storage.Migrate(db, migrationsTable, tbMigrations[:tc.applied]))
			rows := map[string]string{"latte": "  Café\t\tAu  Lait ", "blank": "   "}
			for i := range 2*normalizedBackfillBatch + 3 {
				rows[fmt.Sprintf("filler-%04d", i)] = fmt.Sprintf("Filler Term %04d", i)
			}
			for id, text := range rows {
				_, err = db.ExecContext(ctx, `INSERT INTO tb_concepts (id, created_at, updated_at) VALUES (?, '2026-09-01T00:00:00Z', '2026-09-01T00:00:00Z')`, id)
				require.NoError(t, err)
				_, err = db.ExecContext(ctx, `INSERT INTO tb_terms (concept_id, text, text_lower, locale) VALUES (?, ?, ?, 'en')`, id, text, strings.ToLower(text))
				require.NoError(t, err)
			}
			require.NoError(t, db.Close())

			for range 2 {
				tb, err := NewSQLiteStore(path)
				require.NoError(t, err)
				var empty int
				require.NoError(t, tb.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM tb_terms WHERE text_normalized = ''`).Scan(&empty))
				assert.Equal(t, 1, empty, "only the whitespace term normalizes to the empty string")
				var key string
				require.NoError(t, tb.DB().QueryRowContext(ctx, `SELECT text_normalized FROM tb_terms WHERE concept_id = 'filler-1002'`).Scan(&key))
				assert.Equal(t, "filler term 1002", key)

				matches, err := tb.Lookup(ctx, "CAFÉ AU LAIT", LookupOptions{SourceLocale: "en", MatchModes: []model.MatchStrategy{model.MatchStrategyNormalized}})
				require.NoError(t, err)
				assert.Equal(t, []string{"latte"}, MatchedConceptIDs(matches))
				matches, err = tb.Lookup(ctx, "cafe au lai", LookupOptions{SourceLocale: "en", MatchModes: []model.MatchStrategy{model.MatchStrategyFuzzy}})
				require.NoError(t, err)
				assert.Equal(t, []string{"latte"}, MatchedConceptIDs(matches))
				require.NoError(t, tb.Close())
			}
		})
	}
}
