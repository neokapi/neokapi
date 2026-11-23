package terms_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sqlterms "github.com/neokapi/neokapi/bowrain/terms"
	"github.com/neokapi/neokapi/bowrain/terms/termstest"
	"github.com/neokapi/neokapi/core/model"
	fw "github.com/neokapi/neokapi/terms"
)

func importCSV(t *testing.T, st sqlterms.Store, csv string) sqlterms.ImportReport {
	t.Helper()
	rows, err := fw.ParseCSV(strings.NewReader(csv), fw.CSVImportOptions{
		SourceLocale: model.LocaleEnglish, TargetLocale: "nb", HasHeader: strings.HasPrefix(csv, "source,"),
	})
	require.NoError(t, err)
	report, err := sqlterms.ImportRows(t.Context(), st, rows)
	require.NoError(t, err)
	return report
}

func revisionsOf(t *testing.T, st sqlterms.Store) map[string]string {
	t.Helper()
	all, err := st.Concepts(t.Context())
	require.NoError(t, err)
	ids := make([]string, len(all))
	for i, c := range all {
		ids[i] = c.ID
	}
	revs, err := st.ConceptRevisions(t.Context(), ids)
	require.NoError(t, err)
	return revs
}

func concept(t *testing.T, st sqlterms.Store, id string) fw.Concept {
	t.Helper()
	c, _, ok, err := st.GetConceptAt(t.Context(), id)
	require.NoError(t, err)
	require.True(t, ok, "concept %s is stored", id)
	return c
}

// importScenarios are the import's rules, run against each store.
var importScenarios = []struct {
	name string
	run  func(t *testing.T, st sqlterms.Store)
}{
	{"two files name two concepts", func(t *testing.T, st sqlterms.Store) {
		a := importCSV(t, st, "Save,Lagre\n")
		b := importCSV(t, st, "Cancel,Avbryt\n")
		assert.Equal(t, 1, a.Created)
		assert.Equal(t, 1, b.Created, "a second file's first row is a concept of its own")
		assert.Len(t, revisionsOf(t, st), 2)
		assert.Equal(t, "Lagre", concept(t, st, "term:en:save").Terms[1].Text)
		assert.Equal(t, "Avbryt", concept(t, st, "term:en:cancel").Terms[1].Text)
	}},
	{"the same file twice changes nothing", func(t *testing.T, st sqlterms.Store) {
		const file = "Save,Lagre\nCancel,Avbryt\n"
		first := importCSV(t, st, file)
		assert.Equal(t, 2, first.Created)
		before := revisionsOf(t, st)
		again := importCSV(t, st, file)
		assert.Equal(t, sqlterms.ImportReport{Unchanged: 2}, again)
		assert.Equal(t, before, revisionsOf(t, st), "nothing was written")
	}},
	{"a changed row updates at the concept's revision", func(t *testing.T, st sqlterms.Store) {
		importCSV(t, st, "Save,Lagre\n")
		before := revisionsOf(t, st)["term:en:save"]
		report := importCSV(t, st, "Save,Lagre data\n")
		assert.Equal(t, 1, report.Updated)
		assert.Equal(t, "Lagre data", concept(t, st, "term:en:save").Terms[1].Text)
		assert.NotEqual(t, before, revisionsOf(t, st)["term:en:save"])
	}},
	{"a concept a person edited is reported, not overwritten", func(t *testing.T, st sqlterms.Store) {
		importCSV(t, st, "Save,Lagre\n")
		edited := concept(t, st, "term:en:save")
		edited.Definition = "a person's edit"
		_, rev, _, err := st.GetConceptAt(t.Context(), "term:en:save")
		require.NoError(t, err)
		_, err = st.UpdateConcept(t.Context(), edited, "", rev)
		require.NoError(t, err)

		report := importCSV(t, st, "Save,Lagre data\n")
		assert.Zero(t, report.Imported())
		require.Len(t, report.Conflicts, 1)
		assert.Equal(t, 1, report.Conflicts[0].Row)
		assert.Contains(t, report.Conflicts[0].Reason, "was edited after it was last imported")
		got := concept(t, st, "term:en:save")
		assert.Equal(t, "a person's edit", got.Definition, "the person's edit stays")
		assert.Equal(t, "Lagre", got.Terms[1].Text, "and the import's change was not applied")
	}},
	{"a homonym is reported with its remedy", func(t *testing.T, st sqlterms.Store) {
		// A concept stored before imports named their concepts by term.
		_, err := st.CreateConcept(t.Context(), fw.Concept{ID: "csv-3", Terms: []fw.Term{
			{Text: "Save", Locale: "en", Status: model.TermApproved},
			{Text: "Spare", Locale: "nb", Status: model.TermApproved},
		}}, "")
		require.NoError(t, err)

		report := importCSV(t, st, "Save,Lagre\n")
		require.Len(t, report.Conflicts, 1)
		assert.Contains(t, report.Conflicts[0].Reason, `"Save" is already a term of concept csv-3`)
		assert.Contains(t, report.Conflicts[0].Reason, "concept_id column")
		assert.Equal(t, "Spare", concept(t, st, "csv-3").Terms[1].Text, "the stored concept is left alone")

		// Naming it is the remedy.
		report = importCSV(t, st, "source,target,domain,definition,status,term_source,competitor_term,advisory,concept_id\nSave,Lagre,,,approved,,,,csv-3\n")
		assert.Equal(t, 1, report.Updated)
		assert.Equal(t, "Lagre", concept(t, st, "csv-3").Terms[1].Text)
	}},
	{"a term twice in one file is reported", func(t *testing.T, st sqlterms.Store) {
		report := importCSV(t, st, "Save,Lagre\nSave,Spare\n")
		assert.Equal(t, 1, report.Created)
		require.Len(t, report.Conflicts, 1)
		assert.Equal(t, 2, report.Conflicts[0].Row)
		assert.Contains(t, report.Conflicts[0].Reason, "appears in more than one row")
	}},
}

func TestImportRows(t *testing.T) {
	for _, sc := range importScenarios {
		t.Run(sc.name, func(t *testing.T) { sc.run(t, termstest.Memory()) })
	}
}

// An import that loses a race with a person's save decides again, and finds
// the concept edited.
func TestImportRows_APersonsSaveDuringTheImportStays(t *testing.T) {
	st := termstest.Memory()
	importCSV(t, st, "Save,Lagre\n")
	raced := termstest.Interleaved(st, termstest.SetDefinition(t, "a person's edit"))
	report := importCSV(t, raced, "Save,Lagre data\n")
	require.Len(t, report.Conflicts, 1)
	assert.Equal(t, "a person's edit", concept(t, st, "term:en:save").Definition)
}
