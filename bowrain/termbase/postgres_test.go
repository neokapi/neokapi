//go:build integration

package termbase_test

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/termbase"

	storage "github.com/neokapi/neokapi/bowrain/storage"
	pgtb "github.com/neokapi/neokapi/bowrain/termbase"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func openTestPostgresTermBase(t *testing.T) *pgtb.PostgresTermBase {
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
	tb, err := pgtb.NewPostgresTermBaseFromDB(db, wsID)
	require.NoError(t, err)
	t.Cleanup(func() {
		db.Exec("DELETE FROM tb_terms WHERE workspace_id = $1", wsID)
		db.Exec("DELETE FROM tb_concepts WHERE workspace_id = $1", wsID)
		db.Close()
	})
	return tb
}

func populatePgTB(t *testing.T, tb termbase.TermBase) {
	t.Helper()
	for _, c := range softwareConcepts() {
		require.NoError(t, tb.AddConcept(t.Context(), c))
	}
}

func TestPostgresTermBase_IntegrationAddAndGet(t *testing.T) {
	tb := openTestPostgresTermBase(t)

	populatePgTB(t, tb)
	count, err := tb.Count(t.Context())
	require.NoError(t, err)
	assert.Equal(t, 3, count)

	c, ok, err := tb.GetConcept(t.Context(), "c1")
	require.NoError(t, err)
	assert.True(t, ok)
	assert.Equal(t, "software", c.Domain)
	assert.Len(t, c.Terms, 3)
}

func TestPostgresTermBase_IntegrationDelete(t *testing.T) {
	tb := openTestPostgresTermBase(t)

	populatePgTB(t, tb)

	err := tb.DeleteConcept(t.Context(), "c2")
	require.NoError(t, err)
	count, err := tb.Count(t.Context())
	require.NoError(t, err)
	assert.Equal(t, 2, count)

	_, ok, err := tb.GetConcept(t.Context(), "c2")
	require.NoError(t, err)
	assert.False(t, ok)
}

func TestPostgresTermBase_IntegrationUpdate(t *testing.T) {
	tb := openTestPostgresTermBase(t)

	populatePgTB(t, tb)

	err := tb.AddConcept(t.Context(), termbase.Concept{
		ID:         "c1",
		Domain:     "software-ui",
		Definition: "Updated",
		Terms: []termbase.Term{
			{Text: "Save", Locale: model.LocaleEnglish, Status: model.TermPreferred},
		},
	})
	require.NoError(t, err)
	count, err := tb.Count(t.Context())
	require.NoError(t, err)
	assert.Equal(t, 3, count)

	c, ok, err := tb.GetConcept(t.Context(), "c1")
	require.NoError(t, err)
	assert.True(t, ok)
	assert.Equal(t, "software-ui", c.Domain)
	assert.Len(t, c.Terms, 1)
}

func TestPostgresTermBase_IntegrationLookup(t *testing.T) {
	tb := openTestPostgresTermBase(t)

	populatePgTB(t, tb)

	matches, err := tb.Lookup(t.Context(), "Save", termbase.LookupOptions{
		SourceLocale: model.LocaleEnglish,
	})
	require.NoError(t, err)
	require.Len(t, matches, 1)
	assert.Equal(t, "Save", matches[0].Term.Text)
	assert.Equal(t, 1.0, matches[0].Score)
}

func TestPostgresTermBase_IntegrationLookupAll(t *testing.T) {
	tb := openTestPostgresTermBase(t)

	populatePgTB(t, tb)

	text := "Click Save or Cancel"
	matches, err := tb.LookupAll(t.Context(), text, termbase.LookupOptions{
		SourceLocale: model.LocaleEnglish,
	})
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(matches), 2)
}

func TestPostgresTermBase_IntegrationSearch(t *testing.T) {
	tb := openTestPostgresTermBase(t)

	populatePgTB(t, tb)

	results, total, err := tb.Search(t.Context(), "save", "", "", 0, 100)
	require.NoError(t, err)
	assert.Equal(t, 1, total)
	assert.Len(t, results, 1)
}

// Regression for #747: the empty-query "list all" path built
// `SELECT DISTINCT c.id ... ORDER BY c.updated_at DESC`, which Postgres rejects
// ("for SELECT DISTINCT, ORDER BY expressions must appear in select list"). The
// query errored and Search returned (nil, total) — so the term explorer showed
// empty despite a non-zero count. List-all must return every concept, and the
// trgm search path (which had the same DISTINCT+ORDER-BY issue) must return matches.
func TestPostgresTermBase_IntegrationSearchListAll(t *testing.T) {
	tb := openTestPostgresTermBase(t)

	populatePgTB(t, tb)

	// Empty query = list all (the term-explorer default). Before the fix this
	// returned zero rows with a non-zero total.
	all, total, err := tb.Search(t.Context(), "", "", "", 0, 100)
	require.NoError(t, err)
	assert.Equal(t, 3, total)
	assert.Len(t, all, 3)
	// Pagination still applies.
	page, total, err := tb.Search(t.Context(), "", "", "", 0, 2)
	require.NoError(t, err)
	assert.Equal(t, 3, total)
	assert.Len(t, page, 2)

	// Trgm search path returns matches (not an empty slice on a query error).
	hits, htotal, err := tb.Search(t.Context(), "save", "", "", 0, 100)
	require.NoError(t, err)
	assert.Equal(t, 1, htotal)
	assert.Len(t, hits, 1)
}

func TestPostgresTermBase_IntegrationInterfaceCompliance(t *testing.T) {
	tb := openTestPostgresTermBase(t)

	var _ termbase.TermBase = tb
	assert.NoError(t, tb.Close())
}

func TestPostgresTermBase_IntegrationAddConceptWithStream(t *testing.T) {
	tb := openTestPostgresTermBase(t)

	mainConcept := termbase.Concept{
		ID:     "c-main",
		Domain: "software",
		Terms: []termbase.Term{
			{Text: "File", Locale: "en-US", Status: model.TermApproved},
			{Text: "Datei", Locale: "de-DE", Status: model.TermApproved},
		},
	}
	require.NoError(t, tb.AddConceptWithStream(t.Context(), mainConcept, "main"))

	featureConcept := termbase.Concept{
		ID:     "c-feat",
		Domain: "software",
		Terms: []termbase.Term{
			{Text: "Document", Locale: "en-US", Status: model.TermApproved},
			{Text: "Dokument", Locale: "de-DE", Status: model.TermApproved},
		},
	}
	require.NoError(t, tb.AddConceptWithStream(t.Context(), featureConcept, "feature/rebrand"))

	wsConcept := termbase.Concept{
		ID:     "c-ws",
		Domain: "software",
		Terms: []termbase.Term{
			{Text: "Save", Locale: "en-US", Status: model.TermApproved},
			{Text: "Speichern", Locale: "de-DE", Status: model.TermApproved},
		},
	}
	require.NoError(t, tb.AddConcept(t.Context(), wsConcept))

	concepts, total, err := tb.SearchForStream(t.Context(), "", "", "",
		"feature/rebrand", []string{"main", ""}, 0, 100)
	require.NoError(t, err)
	assert.Equal(t, 3, total)
	assert.Len(t, concepts, 3)
	assert.Equal(t, "c-feat", concepts[0].ID)

	concepts, total, err = tb.SearchForStream(t.Context(), "", "", "", "", nil, 0, 100)
	require.NoError(t, err)
	assert.Equal(t, 1, total)
	assert.Len(t, concepts, 1)
	assert.Equal(t, "c-ws", concepts[0].ID)

	concepts, total, err = tb.SearchForStream(t.Context(), "save", "", "",
		"feature/rebrand", []string{"main", ""}, 0, 100)
	require.NoError(t, err)
	assert.Equal(t, 1, total)
	assert.Len(t, concepts, 1)
	assert.Equal(t, "c-ws", concepts[0].ID)
}
