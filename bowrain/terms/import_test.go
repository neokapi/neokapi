package terms_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sqlterms "github.com/neokapi/neokapi/bowrain/terms"
	"github.com/neokapi/neokapi/bowrain/terms/termstest"
	"github.com/neokapi/neokapi/core/model"
	fw "github.com/neokapi/neokapi/terms"
)

const importWorkspace = "ws-import"

func importRows(t *testing.T, st sqlterms.Store, name string, content []byte, rows []fw.ImportRow) sqlterms.ImportReport {
	t.Helper()
	declared := ""
	if len(rows) > 0 {
		declared = rows[0].Document
	}
	report, err := sqlterms.ImportRows(t.Context(), st, rows, sqlterms.ImportSource{
		Key:    sqlterms.ImportSourceKey(declared, name, content),
		Origin: sqlterms.ExportOrigin(importWorkspace),
	})
	require.NoError(t, err)
	return report
}

// importCSV imports csv as the file named name.
func importCSV(t *testing.T, st sqlterms.Store, name, csv string) sqlterms.ImportReport {
	t.Helper()
	rows, err := fw.ParseCSV(strings.NewReader(csv), fw.CSVImportOptions{
		SourceLocale: model.LocaleEnglish, TargetLocale: "nb", HasHeader: strings.HasPrefix(csv, "source,"),
	})
	require.NoError(t, err)
	return importRows(t, st, name, []byte(csv), rows)
}

func importTBX(t *testing.T, st sqlterms.Store, name, doc string) sqlterms.ImportReport {
	t.Helper()
	rows, err := fw.ParseTBX(strings.NewReader(doc), fw.TBXImportOptions{})
	require.NoError(t, err)
	return importRows(t, st, name, []byte(doc), rows)
}

// exportJSON is the workspace's terms as its export route writes them.
func exportJSON(t *testing.T, st sqlterms.Store) fw.JSONStore {
	t.Helper()
	var buf bytes.Buffer
	require.NoError(t, fw.ExportJSON(t.Context(), st, &buf, "workspace terms"))
	var doc fw.JSONStore
	require.NoError(t, json.Unmarshal(buf.Bytes(), &doc))
	ids := make([]string, len(doc.Concepts))
	for i, c := range doc.Concepts {
		ids[i] = c.ID
	}
	revs, err := st.ConceptRevisions(t.Context(), ids)
	require.NoError(t, err)
	doc.Origin, doc.Revisions = sqlterms.ExportOrigin(importWorkspace), revs
	return doc
}

func importJSON(t *testing.T, st sqlterms.Store, name string, doc fw.JSONStore) sqlterms.ImportReport {
	t.Helper()
	raw, err := json.Marshal(doc)
	require.NoError(t, err)
	rows, err := fw.ParseJSON(bytes.NewReader(raw))
	require.NoError(t, err)
	return importRows(t, st, name, raw, rows)
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

func editDefinition(t *testing.T, st sqlterms.Store, id, definition string) {
	t.Helper()
	c, rev, ok, err := st.GetConceptAt(t.Context(), id)
	require.NoError(t, err)
	require.True(t, ok)
	c.Definition = definition
	_, err = st.UpdateConcept(t.Context(), c, "", rev)
	require.NoError(t, err)
}

func tbxEntry(id, term string) string {
	return `<conceptEntry id="` + id + `"><langSec xml:lang="en"><termSec><term>` + term + `</term></termSec></langSec></conceptEntry>`
}

func tbxDoc(entries ...string) string {
	return `<?xml version="1.0"?><tbx><text><body>` + strings.Join(entries, "") + `</body></text></tbx>`
}

// importScenarios are the import's rules, run against each store.
var importScenarios = []struct {
	name string
	run  func(t *testing.T, st sqlterms.Store)
}{
	{"two files name two concepts", func(t *testing.T, st sqlterms.Store) {
		a := importCSV(t, st, "a.csv", "Save,Lagre\n")
		b := importCSV(t, st, "b.csv", "Cancel,Avbryt\n")
		assert.Equal(t, 1, a.Created)
		assert.Equal(t, 1, b.Created, "a second file's first row is a concept of its own")
		assert.Len(t, revisionsOf(t, st), 2)
		assert.Equal(t, "Lagre", concept(t, st, "term:en:save").Terms[1].Text)
		assert.Equal(t, "Avbryt", concept(t, st, "term:en:cancel").Terms[1].Text)
	}},
	{"two TBX files that both say c1 keep two concepts", func(t *testing.T, st sqlterms.Store) {
		a := importTBX(t, st, "a.tbx", tbxDoc(tbxEntry("c1", "text editor")))
		b := importTBX(t, st, "b.tbx", tbxDoc(tbxEntry("c1", "invoice")))
		assert.Equal(t, 1, a.Created)
		assert.Equal(t, sqlterms.ImportReport{Created: 1}, b, "the second file's c1 is a concept of its own")
		assert.Equal(t, "text editor", concept(t, st, "term:en:text-editor:c1").Terms[0].Text, "the first file's c1 is unchanged")
		assert.Equal(t, "invoice", concept(t, st, "term:en:invoice:c1").Terms[0].Text)
	}},
	{"an id that names a concept a person made is reported, not followed", func(t *testing.T, st sqlterms.Store) {
		_, err := st.CreateConcept(t.Context(), fw.Concept{ID: "hm1", Definition: "made in Bowrain", Terms: []fw.Term{
			{Text: "Archive", Locale: "en", Status: model.TermApproved},
		}}, "")
		require.NoError(t, err)
		before := revisionsOf(t, st)

		report := importCSV(t, st, "ids.csv", "source,target,domain,definition,status,term_source,competitor_term,advisory,concept_id\nArchive,Arkiv,,,approved,,,,hm1\n")
		assert.Zero(t, report.Imported(), "nothing is written for the row")
		require.Len(t, report.Conflicts, 1)
		assert.Equal(t, 2, report.Conflicts[0].Row)
		assert.Contains(t, report.Conflicts[0].Reason, "also names concept hm1 in the workspace")
		assert.Equal(t, before, revisionsOf(t, st), "and nothing changed")
		assert.Equal(t, "made in Bowrain", concept(t, st, "hm1").Definition)
	}},
	{"the same file twice changes nothing", func(t *testing.T, st sqlterms.Store) {
		const file = "Save,Lagre\nCancel,Avbryt\n"
		first := importCSV(t, st, "terms.csv", file)
		assert.Equal(t, 2, first.Created)
		before := revisionsOf(t, st)
		again := importCSV(t, st, "terms.csv", file)
		assert.Equal(t, sqlterms.ImportReport{Unchanged: 2}, again)
		assert.Equal(t, before, revisionsOf(t, st), "nothing was written")

		doc := tbxDoc(tbxEntry("c1", "text editor"))
		importTBX(t, st, "a.tbx", doc)
		assert.Equal(t, sqlterms.ImportReport{Unchanged: 1}, importTBX(t, st, "a.tbx", doc))
	}},
	{"a changed row of the same file updates at the concept's revision", func(t *testing.T, st sqlterms.Store) {
		importCSV(t, st, "terms.csv", "Save,Lagre\n")
		before := revisionsOf(t, st)["term:en:save"]
		report := importCSV(t, st, "terms.csv", "Save,Lagre data\n")
		assert.Equal(t, 1, report.Updated)
		assert.Equal(t, "Lagre data", concept(t, st, "term:en:save").Terms[1].Text)
		assert.NotEqual(t, before, revisionsOf(t, st)["term:en:save"])
	}},
	{"another file's row for the same term is reported", func(t *testing.T, st sqlterms.Store) {
		importCSV(t, st, "terms.csv", "Save,Lagre\n")
		report := importCSV(t, st, "other.csv", "Save,Spare\n")
		require.Len(t, report.Conflicts, 1)
		assert.Contains(t, report.Conflicts[0].Reason, "was imported from another file")
		assert.Equal(t, "Lagre", concept(t, st, "term:en:save").Terms[1].Text)
	}},
	{"a concept a person edited is reported, not overwritten", func(t *testing.T, st sqlterms.Store) {
		importCSV(t, st, "terms.csv", "Save,Lagre\n")
		editDefinition(t, st, "term:en:save", "a person's edit")

		report := importCSV(t, st, "terms.csv", "Save,Lagre data\n")
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

		report := importCSV(t, st, "terms.csv", "Save,Lagre\n")
		require.Len(t, report.Conflicts, 1)
		assert.Contains(t, report.Conflicts[0].Reason, `"Save" is already a term of concept csv-3`)
		assert.Contains(t, report.Conflicts[0].Reason, "export the workspace's terms")
		assert.Equal(t, "Spare", concept(t, st, "csv-3").Terms[1].Text, "the stored concept is left alone")
	}},
	{"a workspace export edited and imported back updates its concepts", func(t *testing.T, st sqlterms.Store) {
		_, err := st.CreateConcept(t.Context(), fw.Concept{ID: "csv-3", Terms: []fw.Term{
			{Text: "Save", Locale: "en", Status: model.TermApproved},
			{Text: "Spare", Locale: "nb", Status: model.TermApproved},
		}}, "")
		require.NoError(t, err)

		doc := exportJSON(t, st)
		doc.Concepts[0].Terms[1].Text = "Lagre"
		assert.Equal(t, sqlterms.ImportReport{Updated: 1}, importJSON(t, st, "export.json", doc))
		assert.Equal(t, "Lagre", concept(t, st, "csv-3").Terms[1].Text)

		// The same export again: Bowrain has the edit, so nothing changes.
		assert.Equal(t, sqlterms.ImportReport{Unchanged: 1}, importJSON(t, st, "export.json", doc))

		// An export a person edits while someone else changes the concept in
		// Bowrain does not replace that change.
		stale := exportJSON(t, st)
		editDefinition(t, st, "csv-3", "changed in Bowrain")
		stale.Concepts[0].Terms[1].Text = "Lagre data"
		report := importJSON(t, st, "export.json", stale)
		require.Len(t, report.Conflicts, 1)
		assert.Contains(t, report.Conflicts[0].Reason, "changed in Bowrain after this file was exported")
		assert.Equal(t, "changed in Bowrain", concept(t, st, "csv-3").Definition)
	}},
	{"a term twice in one file is reported", func(t *testing.T, st sqlterms.Store) {
		report := importCSV(t, st, "terms.csv", "Save,Lagre\nSave,Spare\n")
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
	importCSV(t, st, "terms.csv", "Save,Lagre\n")
	raced := termstest.Interleaved(st, termstest.SetDefinition(t, "a person's edit"))
	report := importCSV(t, raced, "terms.csv", "Save,Lagre data\n")
	require.Len(t, report.Conflicts, 1)
	assert.Equal(t, "a person's edit", concept(t, st, "term:en:save").Definition)
}
