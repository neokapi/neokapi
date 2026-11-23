package terms_test

import (
	"bytes"
	"encoding/json"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sqlterms "github.com/neokapi/neokapi/bowrain/terms"
	"github.com/neokapi/neokapi/bowrain/terms/termstest"
	"github.com/neokapi/neokapi/core/model"
	fw "github.com/neokapi/neokapi/terms"
)

var importSeq atomic.Int64

// importClock is the time of the next import: a minute later each time.
func importClock() time.Time {
	return time.Date(2026, 11, 23, 10, 0, 0, 0, time.UTC).Add(time.Duration(importSeq.Add(1)) * time.Minute)
}

// newImport records a new import of the file name by who, with an id of its
// own, as the server does before it writes a row.
func newImport(t *testing.T, st sqlterms.Store, name, who string) sqlterms.ImportSource {
	t.Helper()
	id := "import-" + strconv.FormatInt(importSeq.Add(1), 10)
	require.NoError(t, st.CreateImport(t.Context(), sqlterms.ImportRecord{ID: id, Name: name, CreatedBy: who, CreatedAt: importClock()}))
	return sqlterms.ImportSource{ID: id}
}

// updateOf records that who runs the earlier import again with the file
// name, as the server does when a person names it in update_of.
func updateOf(t *testing.T, st sqlterms.Store, earlier sqlterms.ImportSource, name, who string) sqlterms.ImportSource {
	t.Helper()
	_, err := st.TouchImport(t.Context(), earlier.ID, name, who, importClock())
	require.NoError(t, err)
	return earlier
}

func runImport(t *testing.T, st sqlterms.Store, src sqlterms.ImportSource, rows []fw.ImportRow) sqlterms.ImportReport {
	t.Helper()
	report, err := sqlterms.ImportRows(t.Context(), st, rows, src)
	require.NoError(t, err)
	return report
}

func csvRows(t *testing.T, csv string) []fw.ImportRow {
	t.Helper()
	rows, err := fw.ParseCSV(strings.NewReader(csv), fw.CSVImportOptions{
		SourceLocale: model.LocaleEnglish, TargetLocale: "nb", HasHeader: strings.HasPrefix(csv, "source,"),
	})
	require.NoError(t, err)
	return rows
}

func tbxRows(t *testing.T, doc string) []fw.ImportRow {
	t.Helper()
	rows, err := fw.ParseTBX(strings.NewReader(doc), fw.TBXImportOptions{})
	require.NoError(t, err)
	return rows
}

func jsonRows(t *testing.T, doc fw.JSONStore) []fw.ImportRow {
	t.Helper()
	raw, err := json.Marshal(doc)
	require.NoError(t, err)
	rows, err := fw.ParseJSON(bytes.NewReader(raw))
	require.NoError(t, err)
	return rows
}

// exportJSON is the workspace's terms as its export route writes them:
// revisions read first, then the concepts.
func exportJSON(t *testing.T, st sqlterms.Store) fw.JSONStore {
	t.Helper()
	all, err := st.Concepts(t.Context())
	require.NoError(t, err)
	ids := make([]string, len(all))
	for i, c := range all {
		ids[i] = c.ID
	}
	revs, err := st.ConceptRevisions(t.Context(), ids)
	require.NoError(t, err)
	var buf bytes.Buffer
	require.NoError(t, fw.ExportJSON(t.Context(), st, &buf, "workspace terms"))
	var doc fw.JSONStore
	require.NoError(t, json.Unmarshal(buf.Bytes(), &doc))
	doc.Revisions = revs
	return doc
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

func handMade(t *testing.T, st sqlterms.Store, id, source, target string) {
	t.Helper()
	_, err := st.CreateConcept(t.Context(), fw.Concept{ID: id, Definition: "made in Bowrain", Terms: []fw.Term{
		{Text: source, Locale: "en", Status: model.TermApproved},
		{Text: target, Locale: "nb", Status: model.TermApproved},
	}}, "")
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
		a := runImport(t, st, newImport(t, st, "a.csv", "ann"), csvRows(t, "Save,Lagre\n"))
		b := runImport(t, st, newImport(t, st, "b.csv", "ann"), csvRows(t, "Cancel,Avbryt\n"))
		assert.Equal(t, 1, a.Created)
		assert.Equal(t, 1, b.Created)
		assert.Equal(t, "Lagre", concept(t, st, "term:en:save").Terms[1].Text)
		assert.Equal(t, "Avbryt", concept(t, st, "term:en:cancel").Terms[1].Text)
	}},
	{"two TBX files that both say c1 keep two concepts", func(t *testing.T, st sqlterms.Store) {
		a := runImport(t, st, newImport(t, st, "a.tbx", "ann"), tbxRows(t, tbxDoc(tbxEntry("c1", "text editor"))))
		b := runImport(t, st, newImport(t, st, "b.tbx", "bob"), tbxRows(t, tbxDoc(tbxEntry("c1", "invoice"))))
		assert.Equal(t, 1, a.Created)
		assert.Equal(t, sqlterms.ImportReport{Created: 1}, b, "the second file's c1 is a concept of its own")
		assert.Equal(t, "text editor", concept(t, st, "term:en:text-editor:c1").Terms[0].Text, "the first file's c1 is unchanged")
		assert.Equal(t, "invoice", concept(t, st, "term:en:invoice:c1").Terms[0].Text)
	}},
	{"two people importing files with the same name", func(t *testing.T, st sqlterms.Store) {
		runImport(t, st, newImport(t, st, "terms.csv", "ann"), csvRows(t, "Save,Lagre\n"))
		before := revisionsOf(t, st)
		report := runImport(t, st, newImport(t, st, "terms.csv", "bob"), csvRows(t, "Save,Spare\n"))
		assert.Zero(t, report.Imported())
		require.Len(t, report.Conflicts, 1)
		assert.Equal(t, 1, report.Conflicts[0].Row)
		assert.Contains(t, report.Conflicts[0].Reason, "belongs to the import of terms.csv by ann on 2026-11-23")
		assert.Contains(t, report.Conflicts[0].Reason, "import this file as an update of that import")
		assert.Equal(t, before, revisionsOf(t, st), "nothing was written")
		assert.Equal(t, "Lagre", concept(t, st, "term:en:save").Terms[1].Text)
	}},
	{"a person updates their own earlier import by naming it", func(t *testing.T, st sqlterms.Store) {
		first := newImport(t, st, "terms.csv", "ann")
		runImport(t, st, first, csvRows(t, "Save,Lagre\n"))
		report := runImport(t, st, updateOf(t, st, first, "terms-v2.csv", "ann"), csvRows(t, "Save,Lagre data\nCancel,Avbryt\n"))
		assert.Equal(t, sqlterms.ImportReport{Created: 1, Updated: 1}, report)
		assert.Equal(t, "Lagre data", concept(t, st, "term:en:save").Terms[1].Text)

		imports, err := st.Imports(t.Context(), 10)
		require.NoError(t, err)
		require.Len(t, imports, 1, "the update stays the one import")
		assert.Equal(t, first.ID, imports[0].ID)
		assert.Equal(t, "terms-v2.csv", imports[0].Name, "listed by its latest file")
		assert.Equal(t, "ann", imports[0].CreatedBy)
		assert.Equal(t, "ann", imports[0].UpdatedBy)
	}},
	{"someone else updating an import keeps who created it", func(t *testing.T, st sqlterms.Store) {
		first := newImport(t, st, "terms.csv", "ann")
		runImport(t, st, first, csvRows(t, "Save,Lagre\n"))
		runImport(t, st, updateOf(t, st, first, "terms.csv", "bob"), csvRows(t, "Save,Lagre data\n"))
		r, ok, err := st.GetImport(t.Context(), first.ID)
		require.NoError(t, err)
		require.True(t, ok)
		assert.Equal(t, "ann", r.CreatedBy, "the creator stays")
		assert.Equal(t, "bob", r.UpdatedBy)
		assert.True(t, r.UpdatedAt.After(r.CreatedAt))
	}},
	{"an import that writes nothing is still listed", func(t *testing.T, st sqlterms.Store) {
		handMade(t, st, "csv-3", "Save", "Spare")
		src := newImport(t, st, "homonyms.csv", "ann")
		report := runImport(t, st, src, csvRows(t, "Save,Lagre\n"))
		assert.Zero(t, report.Imported())
		imports, err := st.Imports(t.Context(), 10)
		require.NoError(t, err)
		require.Len(t, imports, 1)
		assert.Equal(t, src.ID, imports[0].ID)
	}},
	{"an update of an import whose concept a person edited leaves the edit", func(t *testing.T, st sqlterms.Store) {
		first := newImport(t, st, "terms.csv", "ann")
		runImport(t, st, first, csvRows(t, "Save,Lagre\n"))
		editDefinition(t, st, "term:en:save", "a person's edit")

		report := runImport(t, st, updateOf(t, st, first, "terms.csv", "ann"), csvRows(t, "Save,Lagre data\n"))
		assert.Zero(t, report.Imported())
		require.Len(t, report.Conflicts, 1)
		assert.Contains(t, report.Conflicts[0].Reason, "was edited after it was last imported")
		got := concept(t, st, "term:en:save")
		assert.Equal(t, "a person's edit", got.Definition)
		assert.Equal(t, "Lagre", got.Terms[1].Text)
	}},
	{"an id that names a concept a person made is reported, not followed", func(t *testing.T, st sqlterms.Store) {
		handMade(t, st, "hm1", "Archive", "Arkiv")
		before := revisionsOf(t, st)
		report := runImport(t, st, newImport(t, st, "ids.csv", "ann"), csvRows(t,
			"source,target,domain,definition,status,term_source,competitor_term,advisory,concept_id\nArchive,Arkivet,,,approved,,,,hm1\n"))
		assert.Zero(t, report.Imported())
		require.Len(t, report.Conflicts, 1)
		assert.Equal(t, 2, report.Conflicts[0].Row)
		assert.Contains(t, report.Conflicts[0].Reason, "also names concept hm1 in the workspace")
		assert.Equal(t, before, revisionsOf(t, st))
	}},
	{"the same file twice changes nothing", func(t *testing.T, st sqlterms.Store) {
		rows := csvRows(t, "Save,Lagre\nCancel,Avbryt\n")
		assert.Equal(t, 2, runImport(t, st, newImport(t, st, "terms.csv", "ann"), rows).Created)
		before := revisionsOf(t, st)
		assert.Equal(t, sqlterms.ImportReport{Unchanged: 2}, runImport(t, st, newImport(t, st, "terms.csv", "ann"), rows))
		assert.Equal(t, before, revisionsOf(t, st), "nothing was written")

		doc := tbxRows(t, tbxDoc(tbxEntry("c1", "text editor")))
		runImport(t, st, newImport(t, st, "a.tbx", "ann"), doc)
		assert.Equal(t, sqlterms.ImportReport{Unchanged: 1}, runImport(t, st, newImport(t, st, "a.tbx", "ann"), doc))
	}},
	{"a homonym is reported with its remedy", func(t *testing.T, st sqlterms.Store) {
		handMade(t, st, "csv-3", "Save", "Spare")
		report := runImport(t, st, newImport(t, st, "terms.csv", "ann"), csvRows(t, "Save,Lagre\n"))
		require.Len(t, report.Conflicts, 1)
		assert.Contains(t, report.Conflicts[0].Reason, `"Save" is already a term of concept csv-3`)
		assert.Equal(t, "Spare", concept(t, st, "csv-3").Terms[1].Text)
	}},
	{"a workspace export edited and imported back updates at its revisions", func(t *testing.T, st sqlterms.Store) {
		handMade(t, st, "csv-3", "Save", "Spare")
		doc := exportJSON(t, st)
		doc.Concepts[0].Terms[1].Text = "Lagre"
		assert.Equal(t, sqlterms.ImportReport{Updated: 1}, runImport(t, st, newImport(t, st, "export.json", "ann"), jsonRows(t, doc)))
		assert.Equal(t, "Lagre", concept(t, st, "csv-3").Terms[1].Text)
		assert.Equal(t, sqlterms.ImportReport{Unchanged: 1}, runImport(t, st, newImport(t, st, "export.json", "ann"), jsonRows(t, doc)))

		// An export edited while someone changes the concept in Bowrain.
		stale := exportJSON(t, st)
		editDefinition(t, st, "csv-3", "changed in Bowrain")
		stale.Concepts[0].Terms[1].Text = "Lagre data"
		report := runImport(t, st, newImport(t, st, "export.json", "ann"), jsonRows(t, stale))
		require.Len(t, report.Conflicts, 1)
		assert.Contains(t, report.Conflicts[0].Reason, "changed in Bowrain after this file was exported")
		assert.Equal(t, "changed in Bowrain", concept(t, st, "csv-3").Definition)
	}},
	{"a file with workspace ids and no or stale revisions updates nothing", func(t *testing.T, st sqlterms.Store) {
		handMade(t, st, "c-a", "Save", "Spare")
		handMade(t, st, "c-b", "Cancel", "Avbryt")
		doc := exportJSON(t, st)
		for i := range doc.Concepts {
			doc.Concepts[i].Definition = "rewritten by the file"
		}
		before := revisionsOf(t, st)

		bare := doc
		bare.Revisions = nil
		report := runImport(t, st, newImport(t, st, "bare.json", "mallory"), jsonRows(t, bare))
		assert.Zero(t, report.Imported())
		assert.Len(t, report.Conflicts, 2, "every row is reported")

		stale := doc
		stale.Revisions = map[string]string{"c-a": "not-a-revision", "c-b": "not-a-revision"}
		report = runImport(t, st, newImport(t, st, "stale.json", "mallory"), jsonRows(t, stale))
		assert.Zero(t, report.Imported())
		assert.Len(t, report.Conflicts, 2, "every row is reported")
		assert.Equal(t, before, revisionsOf(t, st), "nothing was written")
		assert.Equal(t, "made in Bowrain", concept(t, st, "c-a").Definition)
	}},
	{"a term twice in one file is reported", func(t *testing.T, st sqlterms.Store) {
		report := runImport(t, st, newImport(t, st, "terms.csv", "ann"), csvRows(t, "Save,Lagre\nSave,Spare\n"))
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

// An update that loses a race with a person's save decides again, and finds
// the concept edited.
func TestImportRows_APersonsSaveDuringTheImportStays(t *testing.T) {
	st := termstest.Memory()
	first := newImport(t, st, "terms.csv", "ann")
	runImport(t, st, first, csvRows(t, "Save,Lagre\n"))
	raced := termstest.Interleaved(st, termstest.SetDefinition(t, "a person's edit"))
	report := runImport(t, raced, updateOf(t, st, first, "terms.csv", "ann"), csvRows(t, "Save,Lagre data\n"))
	require.Len(t, report.Conflicts, 1)
	assert.Equal(t, "a person's edit", concept(t, st, "term:en:save").Definition)
}
