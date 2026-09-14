//go:build integration

package terms_test

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/terms"

	storage "github.com/neokapi/neokapi/bowrain/storage"
	pgterms "github.com/neokapi/neokapi/bowrain/terms"
)

// TestPostgresTerms_KeepsDoNotTranslate mirrors the framework store: a
// do-not-translate concept reads back with its flag from every read path, and
// the rule derived from it governs a language the concept has no term in.
func TestPostgresTerms_KeepsDoNotTranslate(t *testing.T) {
	tb := openTestPostgresTerms(t)
	ctx := context.Background()

	require.NoError(t, tb.AddConcept(ctx, terms.Concept{
		ID:             "kapi",
		DoNotTranslate: true,
		Terms:          []terms.Term{{Text: "kapi", Locale: model.LocaleEnglish, Status: model.TermPreferred}},
	}))
	require.NoError(t, tb.AddConcept(ctx, terms.Concept{
		ID: "save",
		Terms: []terms.Term{
			{Text: "Save", Locale: model.LocaleEnglish, Status: model.TermPreferred},
			{Text: "Enregistrer", Locale: model.LocaleFrench, Status: model.TermPreferred},
		},
	}))

	c, ok, err := tb.GetConcept(ctx, "kapi")
	require.NoError(t, err)
	require.True(t, ok)
	assert.True(t, c.DoNotTranslate, "GetConcept keeps the flag")

	all, err := tb.Concepts(ctx)
	require.NoError(t, err)
	flags := map[string]bool{}
	for _, concept := range all {
		flags[concept.ID] = concept.DoNotTranslate
	}
	assert.Equal(t, map[string]bool{"kapi": true, "save": false}, flags, "Concepts keeps the flag")

	matches, err := tb.LookupAll(ctx, "Open kapi", terms.LookupOptions{SourceLocale: model.LocaleEnglish})
	require.NoError(t, err)
	require.NotEmpty(t, matches)
	assert.True(t, matches[0].Concept.DoNotTranslate, "a lookup hydrates the flag")

	rule, ok := terms.RuleForConcept(c, model.LocaleEnglish, model.LocaleGerman)
	require.True(t, ok, "a stored do-not-translate concept yields a rule in a language it has no term in")
	assert.True(t, rule.DoNotTranslate)
}

// TestPostgresTerms_MigratesASchemaWithoutDoNotTranslate builds the schema a
// database carried before concepts stored the flag, with a concept in it, and
// opens a store over it. The concept survives and reads back without the flag,
// and the flag persists once set.
func TestPostgresTerms_MigratesASchemaWithoutDoNotTranslate(t *testing.T) {
	db := scratchTermsDatabase(t)
	ctx := context.Background()

	require.NoError(t, storage.MigratePostgresNS(db, "tb_schema_migrations", pgterms.Migrations[:2]))
	_, err := db.ExecContext(ctx, `ALTER TABLE tb_concepts DROP COLUMN IF EXISTS do_not_translate`)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `INSERT INTO tb_concepts (id, workspace_id) VALUES ('kapi', 'ws')`)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `INSERT INTO tb_terms (workspace_id, concept_id, text, text_lower, locale, status)
		VALUES ('ws', 'kapi', 'kapi', 'kapi', 'en', 'preferred')`)
	require.NoError(t, err)

	tb, err := pgterms.NewPostgresStoreFromDB(db, "ws")
	require.NoError(t, err)

	c, ok, err := tb.GetConcept(ctx, "kapi")
	require.NoError(t, err)
	require.True(t, ok, "the existing concept survives the migration")
	require.Len(t, c.Terms, 1)
	assert.False(t, c.DoNotTranslate, "an existing concept reads back without the flag")

	c.DoNotTranslate = true
	require.NoError(t, tb.AddConcept(ctx, c))
	got, _, err := tb.GetConcept(ctx, "kapi")
	require.NoError(t, err)
	assert.True(t, got.DoNotTranslate, "the flag persists once the column exists")
}

// scratchTermsDatabase creates a database of its own on the test server, so a
// test can build an older schema without altering the one other tests share.
func scratchTermsDatabase(t *testing.T) *storage.PgDB {
	t.Helper()
	connStr := os.Getenv("BOWRAIN_TEST_POSTGRES_URL")
	if connStr == "" {
		connStr = "postgres://bowrain:bowrain@localhost:5432/bowrain_test?sslmode=disable"
	}
	admin, err := storage.OpenPostgres(connStr)
	if err != nil {
		t.Skipf("PostgreSQL not available: %v", err)
	}
	t.Cleanup(func() { _ = admin.Close() })

	name := fmt.Sprintf("terms_dnt_%d_%d", os.Getpid(), time.Now().UnixNano())
	_, err = admin.Exec(`CREATE DATABASE "` + name + `"`)
	require.NoError(t, err)
	t.Cleanup(func() { _, _ = admin.Exec(`DROP DATABASE IF EXISTS "` + name + `" WITH (FORCE)`) })

	u, err := url.Parse(connStr)
	require.NoError(t, err)
	u.Path = "/" + name
	db, err := storage.OpenPostgres(u.String())
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	return db
}
