//go:build integration

package terms_test

import (
	"fmt"
	"os"
	"sort"
	"testing"
	"time"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/terms"

	storage "github.com/neokapi/neokapi/bowrain/storage"
	pgterms "github.com/neokapi/neokapi/bowrain/terms"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func openTestPostgresTerms(t *testing.T) *pgterms.PostgresStore {
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
	tb, err := pgterms.NewPostgresStoreFromDB(db, wsID)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = db.Exec("DELETE FROM tb_terms WHERE workspace_id = $1", wsID)
		_, _ = db.Exec("DELETE FROM tb_concepts WHERE workspace_id = $1", wsID)
		_ = db.Close()
	})
	return tb
}

func populatePgTB(t *testing.T, tb terms.Terminology) {
	t.Helper()
	for _, c := range softwareConcepts() {
		require.NoError(t, tb.AddConcept(t.Context(), c))
	}
}

func TestPostgresTerms_IntegrationAddAndGet(t *testing.T) {
	tb := openTestPostgresTerms(t)

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

func TestPostgresTerms_IntegrationDelete(t *testing.T) {
	tb := openTestPostgresTerms(t)

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

func TestPostgresTerms_IntegrationUpdate(t *testing.T) {
	tb := openTestPostgresTerms(t)

	populatePgTB(t, tb)

	err := tb.AddConcept(t.Context(), terms.Concept{
		ID:         "c1",
		Domain:     "software-ui",
		Definition: "Updated",
		Terms: []terms.Term{
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

func TestPostgresTerms_IntegrationLookup(t *testing.T) {
	tb := openTestPostgresTerms(t)

	populatePgTB(t, tb)

	matches, err := tb.Lookup(t.Context(), "Save", terms.LookupOptions{
		SourceLocale: model.LocaleEnglish,
	})
	require.NoError(t, err)
	require.Len(t, matches, 1)
	assert.Equal(t, "Save", matches[0].Term.Text)
	assert.Equal(t, 1.0, matches[0].Score)
}

func TestPostgresTerms_IntegrationLookupAll(t *testing.T) {
	tb := openTestPostgresTerms(t)

	populatePgTB(t, tb)

	text := "Click Save or Cancel"
	matches, err := tb.LookupAll(t.Context(), text, terms.LookupOptions{
		SourceLocale: model.LocaleEnglish,
	})
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(matches), 2)
}

func TestPostgresTerms_IntegrationSearch(t *testing.T) {
	tb := openTestPostgresTerms(t)

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
func TestPostgresTerms_IntegrationSearchListAll(t *testing.T) {
	tb := openTestPostgresTerms(t)

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

func TestPostgresTerms_IntegrationInterfaceCompliance(t *testing.T) {
	tb := openTestPostgresTerms(t)

	var _ terms.Terminology = tb
	assert.NoError(t, tb.Close())
}

func TestPostgresTerms_IntegrationAddConceptWithStream(t *testing.T) {
	tb := openTestPostgresTerms(t)

	mainConcept := terms.Concept{
		ID:     "c-main",
		Domain: "software",
		Terms: []terms.Term{
			{Text: "File", Locale: "en-US", Status: model.TermApproved},
			{Text: "Datei", Locale: "de-DE", Status: model.TermApproved},
		},
	}
	require.NoError(t, tb.AddConceptWithStream(t.Context(), mainConcept, "main"))

	featureConcept := terms.Concept{
		ID:     "c-feat",
		Domain: "software",
		Terms: []terms.Term{
			{Text: "Document", Locale: "en-US", Status: model.TermApproved},
			{Text: "Dokument", Locale: "de-DE", Status: model.TermApproved},
		},
	}
	require.NoError(t, tb.AddConceptWithStream(t.Context(), featureConcept, "feature/rebrand"))

	wsConcept := terms.Concept{
		ID:     "c-ws",
		Domain: "software",
		Terms: []terms.Term{
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

// seedTargetTermsFixture loads a concept with source, target and third-locale
// wording, a concept with source wording only, a concept whose source term is
// forbidden, a competitor's name, and a shadow copy of the first concept carrying a target term of
// its own.
func seedTargetTermsFixture(t *testing.T, tb terms.Terminology) {
	t.Helper()
	for _, c := range []terms.Concept{
		{ID: "c-enc", Domain: "security", Terms: []terms.Term{
			{Text: "encryption", Locale: "en", Status: model.TermApproved},
			{Text: "chiffrement", Locale: "fr", Status: model.TermApproved},
			{Text: "cryptage", Locale: "fr", Status: model.TermForbidden},
			{Text: "Verschlüsselung", Locale: "de", Status: model.TermApproved},
		}},
		{ID: "c-dash", Domain: "ui", Terms: []terms.Term{
			{Text: "dashboard", Locale: "en", Status: model.TermApproved},
		}},
		{ID: "c-rival", Domain: "market", Terms: []terms.Term{
			{Text: "RivalCrypt", Locale: "en", Status: model.TermForbidden, CompetitorTerm: true},
		}},
		{ID: "c-crypto", Domain: "security", Terms: []terms.Term{
			{Text: "crypto", Locale: "en", Status: model.TermForbidden},
			{Text: "crypto", Locale: "fr", Status: model.TermForbidden},
		}},
		{ID: terms.ShadowIDPrefix + "c-enc", Domain: "security", Terms: []terms.Term{
			{Text: "encryption", Locale: "en", Status: model.TermApproved},
			{Text: "codage", Locale: "fr", Status: model.TermApproved},
		}},
	} {
		require.NoError(t, tb.AddConcept(t.Context(), c))
	}
}

// termsIn returns the texts of a concept's terms in one locale, sorted.
func termsIn(c terms.Concept, loc model.LocaleID) []string {
	var out []string
	for _, tm := range c.Terms {
		if tm.Locale == loc {
			out = append(out, tm.Text)
		}
	}
	sort.Strings(out)
	return out
}

// matchFor returns the match of the named concept, failing when there is none.
func matchFor(t *testing.T, matches []terms.TermMatch, conceptID string) terms.TermMatch {
	t.Helper()
	for _, m := range matches {
		if m.Concept.ID == conceptID {
			return m
		}
	}
	require.Failf(t, "no match", "concept %s did not match", conceptID)
	return terms.TermMatch{}
}

// assertLookupAllTargetTerms checks that LookupAll gives each match its whole
// concept, with the wording in the target locale, while the status filter and
// the shadow exclusion still decide which concepts match.
func assertLookupAllTargetTerms(t *testing.T, tb terms.Terminology) {
	t.Helper()
	const text = "Turn on encryption from the dashboard"

	t.Run("target wording rides the match", func(t *testing.T) {
		matches, err := tb.LookupAll(t.Context(), text, terms.LookupOptions{SourceLocale: "en", TargetLocale: "fr"})
		require.NoError(t, err)
		require.Len(t, matches, 2)

		enc := matchFor(t, matches, "c-enc")
		assert.Equal(t, "encryption", enc.Term.Text)
		assert.Equal(t, model.LocaleID("en"), enc.Term.Locale)
		assert.Equal(t, "security", enc.Concept.Domain)
		assert.Equal(t, []string{"encryption"}, termsIn(enc.Concept, "en"))
		assert.Equal(t, []string{"chiffrement", "cryptage"}, termsIn(enc.Concept, "fr"))
		assert.Equal(t, []string{"Verschlüsselung"}, termsIn(enc.Concept, "de"))

		dash := matchFor(t, matches, "c-dash")
		assert.Equal(t, []string{"dashboard"}, termsIn(dash.Concept, "en"))
		assert.Empty(t, termsIn(dash.Concept, "fr"))
	})

	t.Run("no target locale still carries the whole concept", func(t *testing.T) {
		matches, err := tb.LookupAll(t.Context(), text, terms.LookupOptions{SourceLocale: "en"})
		require.NoError(t, err)
		assert.Equal(t, []string{"chiffrement", "cryptage"}, termsIn(matchFor(t, matches, "c-enc").Concept, "fr"))
	})

	t.Run("status filter decides which terms match", func(t *testing.T) {
		matches, err := tb.LookupAll(t.Context(), "Avoid crypto in the dashboard", terms.LookupOptions{
			SourceLocale: "en", TargetLocale: "fr", StatusFilter: []model.TermStatus{model.TermApproved},
		})
		require.NoError(t, err)
		require.Len(t, matches, 1, "the forbidden source term does not match")
		assert.Equal(t, "c-dash", matches[0].Concept.ID)
	})

	t.Run("the matched term keeps its competitor flag", func(t *testing.T) {
		matches, err := tb.LookupAll(t.Context(), "Unlike RivalCrypt, we encrypt", terms.LookupOptions{SourceLocale: "en"})
		require.NoError(t, err)
		rival := matchFor(t, matches, "c-rival")
		assert.True(t, rival.Term.CompetitorTerm)
	})

	t.Run("shadow concepts stay out", func(t *testing.T) {
		matches, err := tb.LookupAll(t.Context(), text, terms.LookupOptions{SourceLocale: "en", TargetLocale: "fr"})
		require.NoError(t, err)
		for _, m := range matches {
			assert.False(t, terms.IsShadowID(m.Concept.ID), "matched shadow concept %s", m.Concept.ID)
			assert.NotContains(t, termsIn(m.Concept, "fr"), "codage")
		}
	})
}

func TestPostgresTerms_IntegrationLookupAllTargetTerms(t *testing.T) {
	tb := openTestPostgresTerms(t)
	seedTargetTermsFixture(t, tb)
	assertLookupAllTargetTerms(t, tb)
}
