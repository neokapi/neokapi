package terms

import (
	"context"
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
