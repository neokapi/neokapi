package terms

import (
	"bytes"
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/storage"
)

func dntKapiConcept() Concept {
	return Concept{
		ID:             "kapi",
		Definition:     "The command-line tool.",
		DoNotTranslate: true,
		Terms:          []Term{{Text: "kapi", Locale: model.LocaleEnglish, Status: model.TermPreferred}},
	}
}

func dntSaveConcept() Concept {
	return Concept{
		ID: "save",
		Terms: []Term{
			{Text: "Save", Locale: model.LocaleEnglish, Status: model.TermPreferred},
			{Text: "Enregistrer", Locale: model.LocaleFrench, Status: model.TermPreferred},
		},
	}
}

func dntStore(t *testing.T) *SQLiteStore {
	t.Helper()
	tb, err := NewSQLiteStore(filepath.Join(t.TempDir(), "terms.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = tb.Close() })
	return tb
}

func dntFlags(concepts []Concept) map[string]bool {
	flags := map[string]bool{}
	for _, c := range concepts {
		flags[c.ID] = c.DoNotTranslate
	}
	return flags
}

// TestSQLiteStore_KeepsDoNotTranslate stores a do-not-translate concept beside
// an ordinary one and reads both back through every read path.
func TestSQLiteStore_KeepsDoNotTranslate(t *testing.T) {
	ctx := context.Background()
	tb := dntStore(t)
	require.NoError(t, tb.AddConcept(ctx, dntKapiConcept()))
	require.NoError(t, tb.AddConcept(ctx, dntSaveConcept()))

	c, ok, err := tb.GetConcept(ctx, "kapi")
	require.NoError(t, err)
	require.True(t, ok)
	assert.True(t, c.DoNotTranslate, "GetConcept keeps the flag")

	all, err := tb.Concepts(ctx)
	require.NoError(t, err)
	assert.Equal(t, map[string]bool{"kapi": true, "save": false}, dntFlags(all), "Concepts keeps the flag")

	matches, err := tb.LookupAll(ctx, "Open kapi and Save", LookupOptions{SourceLocale: model.LocaleEnglish})
	require.NoError(t, err)
	matched := map[string]bool{}
	for _, m := range matches {
		matched[m.Concept.ID] = m.Concept.DoNotTranslate
	}
	assert.Equal(t, map[string]bool{"kapi": true, "save": false}, matched, "a lookup carries the flag on the matched concept")
}

// TestRuleForConcept_StoredDoNotTranslate reads a do-not-translate concept back
// from the SQLite store and derives its rule for a language it has no term in.
func TestRuleForConcept_StoredDoNotTranslate(t *testing.T) {
	ctx := context.Background()
	tb := dntStore(t)
	require.NoError(t, tb.AddConcept(ctx, dntKapiConcept()))

	c, ok, err := tb.GetConcept(ctx, "kapi")
	require.NoError(t, err)
	require.True(t, ok)
	rule, ok := RuleForConcept(c, model.LocaleEnglish, model.LocaleGerman)
	require.True(t, ok, "a stored do-not-translate concept yields a rule")
	assert.True(t, rule.DoNotTranslate)
	assert.Equal(t, "kapi", rule.Term)
}

// A terms database written before concepts carried the flag has no column for
// it. Opening it adds the column, and its existing concepts read back without
// the flag until one is set.
func TestSQLiteStore_MigratesADatabaseWithoutDoNotTranslate(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "terms.db")

	db, err := storage.Open(path)
	require.NoError(t, err)
	require.NoError(t, storage.Migrate(db, migrationsTable, tbMigrations[:4]))
	_, err = db.ExecContext(ctx, `INSERT INTO tb_concepts (id, created_at, updated_at) VALUES ('kapi', '2026-09-01T00:00:00Z', '2026-09-01T00:00:00Z')`)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `INSERT INTO tb_terms (concept_id, text, text_lower, locale, status) VALUES ('kapi', 'kapi', 'kapi', 'en', 'preferred')`)
	require.NoError(t, err)
	require.NoError(t, db.Close())

	tb, err := NewSQLiteStore(path)
	require.NoError(t, err)
	defer tb.Close()

	var columns int
	require.NoError(t, tb.DB().QueryRowContext(ctx,
		`SELECT COUNT(*) FROM pragma_table_info('tb_concepts') WHERE name = 'do_not_translate'`).Scan(&columns))
	assert.Equal(t, 1, columns, "opening the database adds the column")

	c, ok, err := tb.GetConcept(ctx, "kapi")
	require.NoError(t, err)
	require.True(t, ok, "the existing concept survives the migration")
	require.Len(t, c.Terms, 1)
	assert.False(t, c.DoNotTranslate, "an existing concept reads back without the flag")

	c.DoNotTranslate = true
	require.NoError(t, tb.AddConcept(ctx, c))
	got, _, err := tb.GetConcept(ctx, "kapi")
	require.NoError(t, err)
	assert.True(t, got.DoNotTranslate, "the flag persists once set")
}

// TestDoNotTranslateSurvivesTBXAndJSON exports a do-not-translate concept from a
// SQLite store as TBX and as JSON, imports each into a fresh SQLite store, and
// reads the flag back.
func TestDoNotTranslateSurvivesTBXAndJSON(t *testing.T) {
	ctx := context.Background()
	src := dntStore(t)
	require.NoError(t, src.AddConcept(ctx, dntKapiConcept()))
	require.NoError(t, src.AddConcept(ctx, dntSaveConcept()))
	want := map[string]bool{"kapi": true, "save": false}

	var tbx bytes.Buffer
	require.NoError(t, ExportTBX(ctx, src, &tbx, TBXExportOptions{}))
	fromTBX := dntStore(t)
	_, err := ImportTBX(ctx, fromTBX, bytes.NewReader(tbx.Bytes()), TBXImportOptions{})
	require.NoError(t, err)
	got, err := fromTBX.Concepts(ctx)
	require.NoError(t, err)
	assert.Equal(t, want, dntFlags(got), "a TBX round trip keeps the flag")

	var js bytes.Buffer
	require.NoError(t, ExportJSON(ctx, src, &js, "vocabulary"))
	fromJSON := dntStore(t)
	_, err = ImportJSON(ctx, fromJSON, bytes.NewReader(js.Bytes()))
	require.NoError(t, err)
	got, err = fromJSON.Concepts(ctx)
	require.NoError(t, err)
	assert.Equal(t, want, dntFlags(got), "a JSON round trip keeps the flag")
}

// TestExportTBXWritesDoNotTranslateAsAPrivateDescrip pins where the flag
// travels in TBX, and that ImportTBX reads it from there.
func TestExportTBXWritesDoNotTranslateAsAPrivateDescrip(t *testing.T) {
	ctx := context.Background()
	src := NewInMemoryStore()
	require.NoError(t, src.AddConcept(ctx, dntKapiConcept()))
	require.NoError(t, src.AddConcept(ctx, dntSaveConcept()))

	var buf bytes.Buffer
	require.NoError(t, ExportTBX(ctx, src, &buf, TBXExportOptions{}))
	assert.Contains(t, buf.String(), `<descrip type="x-doNotTranslate">true</descrip>`)
	assert.Equal(t, 1, bytes.Count(buf.Bytes(), []byte("x-doNotTranslate")), "only the do-not-translate concept carries it")

	dst := NewInMemoryStore()
	_, err := ImportTBX(ctx, dst, bytes.NewReader(buf.Bytes()), TBXImportOptions{})
	require.NoError(t, err)
	got, err := dst.Concepts(ctx)
	require.NoError(t, err)
	assert.Equal(t, map[string]bool{"kapi": true, "save": false}, dntFlags(got))
}
