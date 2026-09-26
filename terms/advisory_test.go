package terms

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/core/storage"
)

func advisoryUtilizeConcept() Concept {
	return Concept{
		ID:       "utilize",
		Advisory: true,
		Terms: []Term{
			{Text: "utilize", Locale: model.LocaleEnglish, Status: model.TermForbidden},
			{Text: "use", Locale: model.LocaleEnglish, Status: model.TermPreferred},
			{Text: "utiliser", Locale: model.LocaleFrench, Status: model.TermForbidden},
		},
	}
}

func advisoryLeverageConcept() Concept {
	return Concept{
		ID: "leverage",
		Terms: []Term{
			{Text: "leverage", Locale: model.LocaleEnglish, Status: model.TermForbidden},
			{Text: "levier", Locale: model.LocaleFrench, Status: model.TermForbidden},
		},
	}
}

func advisoryFlags(concepts []Concept) map[string]bool {
	flags := map[string]bool{}
	for _, c := range concepts {
		flags[c.ID] = c.Advisory
	}
	return flags
}

// TestSQLiteStore_KeepsAdvisory stores an advisory concept beside a failing
// one and reads both back through every read path, the lookups included.
func TestSQLiteStore_KeepsAdvisory(t *testing.T) {
	ctx := context.Background()
	tb := dntStore(t)
	require.NoError(t, tb.AddConcept(ctx, advisoryUtilizeConcept()))
	require.NoError(t, tb.AddConcept(ctx, advisoryLeverageConcept()))
	want := map[string]bool{"utilize": true, "leverage": false}

	c, ok, err := tb.GetConcept(ctx, "utilize")
	require.NoError(t, err)
	require.True(t, ok)
	assert.True(t, c.Advisory, "GetConcept keeps the flag")

	all, err := tb.Concepts(ctx)
	require.NoError(t, err)
	assert.Equal(t, want, advisoryFlags(all), "Concepts keeps the flag")

	matches, err := tb.LookupAll(ctx, "We utilize and leverage it", LookupOptions{SourceLocale: model.LocaleEnglish})
	require.NoError(t, err)
	matched := map[string]bool{}
	for _, m := range matches {
		matched[m.Concept.ID] = m.Concept.Advisory
	}
	assert.Equal(t, want, matched, "LookupAll hydrates the flag on the matched concept")

	one, err := tb.Lookup(ctx, "utilize", LookupOptions{SourceLocale: model.LocaleEnglish})
	require.NoError(t, err)
	require.NotEmpty(t, one)
	assert.True(t, one[0].Concept.Advisory, "Lookup hydrates the flag")

	c.Advisory = false
	require.NoError(t, tb.AddConcept(ctx, c))
	got, _, err := tb.GetConcept(ctx, "utilize")
	require.NoError(t, err)
	assert.False(t, got.Advisory, "clearing the flag persists")
}

// A terms database written before concepts carried the advisory flag has no
// column for it. Opening it adds the column, and its concepts read back as
// failing until one is marked.
func TestSQLiteStore_MigratesADatabaseWithoutAdvisory(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "terms.db")

	db, err := storage.Open(path)
	require.NoError(t, err)
	require.NoError(t, storage.Migrate(db, migrationsTable, tbMigrations[:5]))
	_, err = db.ExecContext(ctx, `INSERT INTO tb_concepts (id, created_at, updated_at) VALUES ('utilize', '2026-09-01T00:00:00Z', '2026-09-01T00:00:00Z')`)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `INSERT INTO tb_terms (concept_id, text, text_lower, locale, status) VALUES ('utilize', 'utilize', 'utilize', 'en', 'forbidden')`)
	require.NoError(t, err)
	require.NoError(t, db.Close())

	tb, err := NewSQLiteStore(path)
	require.NoError(t, err)
	defer tb.Close()

	var columns int
	require.NoError(t, tb.DB().QueryRowContext(ctx,
		`SELECT COUNT(*) FROM pragma_table_info('tb_concepts') WHERE name = 'advisory'`).Scan(&columns))
	assert.Equal(t, 1, columns, "opening the database adds the column")

	c, ok, err := tb.GetConcept(ctx, "utilize")
	require.NoError(t, err)
	require.True(t, ok, "the existing concept survives the migration")
	assert.False(t, c.Advisory, "an existing concept reads back as failing")

	c.Advisory = true
	require.NoError(t, tb.AddConcept(ctx, c))
	got, _, err := tb.GetConcept(ctx, "utilize")
	require.NoError(t, err)
	assert.True(t, got.Advisory, "the flag persists once set")
}

// TestAdvisorySurvivesTBXAndJSON exports an advisory concept from a SQLite
// store as TBX and as JSON, imports each into a fresh store, and reads the
// flag back.
func TestAdvisorySurvivesTBXAndJSON(t *testing.T) {
	ctx := context.Background()
	src := dntStore(t)
	require.NoError(t, src.AddConcept(ctx, advisoryUtilizeConcept()))
	require.NoError(t, src.AddConcept(ctx, advisoryLeverageConcept()))
	want := map[string]bool{"utilize": true, "leverage": false}

	var tbx bytes.Buffer
	require.NoError(t, ExportTBX(ctx, src, &tbx, TBXExportOptions{}))
	assert.Contains(t, tbx.String(), `<descrip type="x-advisory">true</descrip>`)
	assert.Equal(t, 1, bytes.Count(tbx.Bytes(), []byte("x-advisory")), "only the advisory concept carries it")
	fromTBX := dntStore(t)
	_, err := ImportTBX(ctx, fromTBX, bytes.NewReader(tbx.Bytes()), TBXImportOptions{})
	require.NoError(t, err)
	got, err := fromTBX.Concepts(ctx)
	require.NoError(t, err)
	assert.Equal(t, want, advisoryFlags(got), "a TBX round trip keeps the flag")

	var js bytes.Buffer
	require.NoError(t, ExportJSON(ctx, src, &js, "vocabulary"))
	fromJSON := dntStore(t)
	_, err = ImportJSON(ctx, fromJSON, bytes.NewReader(js.Bytes()))
	require.NoError(t, err)
	got, err = fromJSON.Concepts(ctx)
	require.NoError(t, err)
	assert.Equal(t, want, advisoryFlags(got), "a JSON round trip keeps the flag")
}

func TestImportCSVReadsTheAdvisoryColumn(t *testing.T) {
	ctx := context.Background()

	t.Run("bilingual", func(t *testing.T) {
		tb := NewInMemoryStore()
		_, err := ImportCSV(ctx, tb, strings.NewReader(
			"utilize,utiliser,,,forbidden,brand_vocabulary,,true\n"+
				"leverage,levier,,,forbidden,,,\n"+
				"Globex,Globex,,,forbidden,,true,false\n"),
			CSVImportOptions{SourceLocale: model.LocaleEnglish, TargetLocale: model.LocaleFrench})
		require.NoError(t, err)
		got, err := tb.Concepts(ctx)
		require.NoError(t, err)
		assert.Equal(t, map[string]bool{"csv-1": true, "csv-2": false, "csv-3": false}, advisoryFlags(got))
		for _, c := range got {
			if c.ID == "csv-3" {
				assert.True(t, c.Terms[0].CompetitorTerm, "the competitor column sits before the advisory one")
			}
		}
	})

	t.Run("monolingual", func(t *testing.T) {
		tb := NewInMemoryStore()
		_, err := ImportCSV(ctx, tb, strings.NewReader(
			"utilize,,,forbidden,,,TRUE\n"+
				"leverage,,,forbidden\n"),
			CSVImportOptions{SourceLocale: model.LocaleEnglish, Monolingual: true})
		require.NoError(t, err)
		got, err := tb.Concepts(ctx)
		require.NoError(t, err)
		assert.Equal(t, map[string]bool{"csv-1": true, "csv-2": false}, advisoryFlags(got))
	})
}

// TestExportCSVImportsBack exports concepts as CSV and imports the file into
// a fresh store: the advisory, competitor and term source markings survive,
// because the export writes the layout the import reads.
func TestExportCSVImportsBack(t *testing.T) {
	ctx := context.Background()
	src := NewInMemoryStore()
	advisory := advisoryUtilizeConcept()
	advisory.Source = TermSourceBrandVocabulary
	require.NoError(t, src.AddConcept(ctx, advisory))
	require.NoError(t, src.AddConcept(ctx, Concept{ID: "globex", Terms: []Term{
		{Text: "Globex", Locale: model.LocaleEnglish, Status: model.TermForbidden, CompetitorTerm: true},
		{Text: "Globex", Locale: model.LocaleFrench, Status: model.TermForbidden, CompetitorTerm: true},
	}}))

	var buf bytes.Buffer
	require.NoError(t, ExportCSV(ctx, src, &buf, model.LocaleEnglish, model.LocaleFrench, true))
	header, _, _ := strings.Cut(buf.String(), "\n")
	assert.Equal(t, "source,target,domain,definition,status,term_source,competitor_term,advisory,concept_id", header)

	dst := NewInMemoryStore()
	n, err := ImportCSV(ctx, dst, bytes.NewReader(buf.Bytes()), CSVImportOptions{
		SourceLocale: model.LocaleEnglish, TargetLocale: model.LocaleFrench, HasHeader: true,
	})
	require.NoError(t, err)
	require.Equal(t, 2, n)
	got, err := dst.Concepts(ctx)
	require.NoError(t, err)
	byText := map[string]Concept{}
	for _, c := range got {
		byText[c.Terms[0].Text] = c
	}
	assert.True(t, byText["utilize"].Advisory, "an advisory concept imports back advisory")
	assert.Equal(t, TermSourceBrandVocabulary, byText["utilize"].Source)
	assert.False(t, byText["utilize"].Terms[0].CompetitorTerm)
	assert.False(t, byText["Globex"].Advisory, "a competitor term imports back failing")
	assert.True(t, byText["Globex"].Terms[0].CompetitorTerm, "a competitor term imports back a competitor")
}

func TestMarkingAdvisory(t *testing.T) {
	ctx := context.Background()
	store := NewInMemoryStore()
	marked := MarkingAdvisory(store)
	require.NoError(t, marked.AddConcept(ctx, advisoryLeverageConcept()))

	c, ok, err := store.GetConcept(ctx, "leverage")
	require.NoError(t, err)
	require.True(t, ok)
	assert.True(t, c.Advisory, "a concept written through the wrapper is advisory")

	_, err = ImportCSV(ctx, marked, strings.NewReader("synergy,,,forbidden\n"),
		CSVImportOptions{SourceLocale: model.LocaleEnglish, Monolingual: true})
	require.NoError(t, err)
	imported, ok, err := marked.GetConcept(ctx, "csv-1")
	require.NoError(t, err)
	require.True(t, ok, "reads pass through the wrapper")
	assert.True(t, imported.Advisory, "an import through the wrapper marks every concept")

	require.NoError(t, store.AddConcept(ctx, advisoryLeverageConcept()))
	c, _, err = store.GetConcept(ctx, "leverage")
	require.NoError(t, err)
	assert.False(t, c.Advisory, "the store itself marks nothing")
}

func TestSourceWordRules(t *testing.T) {
	concepts := []Concept{
		{ID: "utilize", Terms: []Term{
			{Text: "utilize", Locale: "en", Status: model.TermForbidden, Forms: []string{"utilizes"}},
			{Text: "use", Locale: "en", Status: model.TermPreferred},
			{Text: "utiliser", Locale: "fr", Status: model.TermForbidden},
		}},
		{ID: "advisory", Advisory: true, Terms: []Term{
			{Text: "leverage", Locale: "en", Status: model.TermForbidden, Note: ReplacementNote("build on")},
		}},
		{ID: "retired", Terms: []Term{
			{Text: "whitelist", Locale: "en", Status: model.TermDeprecated},
			{Text: "allowlist", Locale: "en", Status: model.TermPreferred},
		}},
		{ID: "globex", Terms: []Term{
			{Text: "Globex", Locale: "en", Status: model.TermApproved, CompetitorTerm: true, Note: "a rival"},
		}},
		{ID: "preferred-only", Terms: []Term{
			{Text: "sign in", Locale: "en", Status: model.TermPreferred},
		}},
	}

	rules := SourceWordRules(concepts, "en-US")
	assert.Equal(t, []profile.TermRule{
		{Term: "utilize", Replacement: "use", Forms: NormalizeForms("utilize", []string{"utilizes"}), ConceptID: "utilize"},
		{Term: "leverage", Replacement: "build on", Advisory: true, ConceptID: "advisory"},
		{Term: "whitelist", Replacement: "allowlist", Advisory: true, ConceptID: "retired"},
		{Term: "Globex", Note: "a rival", Competitor: true, ConceptID: "globex"},
	}, rules, "the language beneath the locale applies; a retired term and an advisory concept report; a preferred term imposes nothing")

	fr := SourceWordRules(concepts, "fr")
	require.Len(t, fr, 1)
	assert.Equal(t, "utiliser", fr[0].Term)
	assert.Empty(t, fr[0].Replacement, "no preferred term in the language, no replacement")

	assert.Empty(t, SourceWordRules(concepts, "de"))
}

func TestPromoteAndDemoteRule(t *testing.T) {
	ctx := context.Background()

	t.Run("a promotion opens a concept, and demoting it removes the concept", func(t *testing.T) {
		store := NewInMemoryStore()
		changed, err := PromoteRule(ctx, store, "en", profile.SuggestedRule{Term: "utilize", Replacement: "use", CorrectionCount: 3})
		require.NoError(t, err)
		require.True(t, changed)

		c, ok, err := store.GetConcept(ctx, DecisionConceptID("utilize", "en"))
		require.NoError(t, err)
		require.True(t, ok)
		assert.Equal(t, "3", c.Properties[PropPromotedFrom])
		assert.Equal(t, []profile.TermRule{{Term: "utilize", Replacement: "use", ConceptID: c.ID}},
			SourceWordRules([]Concept{c}, "en"), "the promoted term is forbidden with the correction as its replacement")

		changed, err = PromoteRule(ctx, store, "en", profile.SuggestedRule{Term: "utilize", Replacement: "use", CorrectionCount: 3})
		require.NoError(t, err)
		assert.False(t, changed, "promoting the same rule again changes nothing")

		changed, err = PromoteRule(ctx, store, "en", profile.SuggestedRule{Term: "utilize", Replacement: "use", CorrectionCount: 5})
		require.NoError(t, err)
		assert.True(t, changed, "a new correction count is recorded")

		changed, err = DemoteRule(ctx, store, "en", "Utilize")
		require.NoError(t, err)
		require.True(t, changed)
		n, err := store.Count(ctx)
		require.NoError(t, err)
		assert.Zero(t, n, "a concept that existed for the promotion alone goes with it")

		changed, err = DemoteRule(ctx, store, "en", "utilize")
		require.NoError(t, err)
		assert.False(t, changed, "demoting a term the store does not hold changes nothing")
	})

	t.Run("a promotion joining an existing concept leaves the concept when demoted", func(t *testing.T) {
		store := NewInMemoryStore()
		require.NoError(t, store.AddConcept(ctx, Concept{ID: "use", Terms: []Term{
			{Text: "use", Locale: "en", Status: model.TermPreferred},
			{Text: "utiliser", Locale: "fr", Status: model.TermPreferred},
		}}))
		changed, err := PromoteRule(ctx, store, "en", profile.SuggestedRule{Term: "utilize", Replacement: "use", CorrectionCount: 2})
		require.NoError(t, err)
		require.True(t, changed)
		c, _, err := store.GetConcept(ctx, "use")
		require.NoError(t, err)
		require.Len(t, c.Terms, 3, "the promoted term joins the concept declaring its replacement")

		changed, err = DemoteRule(ctx, store, "en", "utilize")
		require.NoError(t, err)
		require.True(t, changed)
		c, ok, err := store.GetConcept(ctx, "use")
		require.NoError(t, err)
		require.True(t, ok, "the concept existed before the promotion and stays")
		assert.Len(t, c.Terms, 2, "only the demoted term goes")
	})

	t.Run("demoting one of several discouraged terms keeps the concept", func(t *testing.T) {
		store := NewInMemoryStore()
		_, err := PromoteRule(ctx, store, "en", profile.SuggestedRule{Term: "utilize", Replacement: "use", CorrectionCount: 2})
		require.NoError(t, err)
		_, err = PromoteRule(ctx, store, "en", profile.SuggestedRule{Term: "utilise", Replacement: "use", CorrectionCount: 2})
		require.NoError(t, err)
		n, err := store.Count(ctx)
		require.NoError(t, err)
		require.Equal(t, 1, n, "both promotions join one concept")

		changed, err := DemoteRule(ctx, store, "en", "utilise")
		require.NoError(t, err)
		require.True(t, changed)
		all, err := store.Concepts(ctx)
		require.NoError(t, err)
		require.Len(t, all, 1)
		assert.Len(t, SourceWordRules(all, "en"), 1, "the other promoted term is still enforced")
	})

	t.Run("an empty term promotes nothing", func(t *testing.T) {
		store := NewInMemoryStore()
		changed, err := PromoteRule(ctx, store, "en", profile.SuggestedRule{Term: "  ", Replacement: "use"})
		require.NoError(t, err)
		assert.False(t, changed)
	})
}
